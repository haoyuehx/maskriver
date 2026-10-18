package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// capabilityProbes is the internal capability set this file exercises. It
// mirrors the interface the runner asserts, without importing the runner.
type capabilityProbes interface {
	Quiescent(context.Context) error
	HasTriggers(context.Context, contracts.TableRef) (bool, error)
}

// capabilityReaderFor opens the real SQLite adapter on a synthetic file, the
// same way production does, so the probes run against real DDL.
func capabilityReaderFor(t *testing.T, path string) contracts.Reader {
	t.Helper()
	dsn, err := contracts.NewValue(contracts.Text, path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReader(context.Background(), contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: dsn, DatasetID: "capability",
		Schemas: []string{"main"}, MaxOpenConns: 1, ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

// capabilityFile seeds one SQLite file with the supplied statements. template
// is a DSN with one %s placeholder for the temporary path.
func capabilityFile(t *testing.T, name, template string, statements ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	handle, err := sql.Open("sqlite", fmt.Sprintf(template, path))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range statements {
		if _, err = handle.Exec(statement); err != nil {
			_ = handle.Close()
			t.Fatalf("statement %q: %v", statement, err)
		}
	}
	if err = handle.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// capabilityJournalMode puts the adapter's pooled connection into mode, the way
// a session pragma would. journal_mode is a per-connection property (only WAL is
// stored in the file header), so a file fixture can not carry MEMORY, OFF,
// TRUNCATE or PERSIST into a later session: the mode has to be set on the
// connection the probe reads. These fixtures pin MaxOpenConns to 1, so this
// statement and the following probe share one connection.
func capabilityJournalMode(t *testing.T, ctx context.Context, opened contracts.Reader, mode string) {
	t.Helper()
	adapter, ok := opened.(*reader)
	if !ok {
		t.Fatal("fixture is not the SQLite adapter")
	}
	var applied string
	if err := adapter.db.QueryRowContext(ctx, "PRAGMA journal_mode="+mode).Scan(&applied); err != nil {
		t.Fatalf("set journal_mode=%s: %v", mode, err)
	}
	if !strings.EqualFold(strings.TrimSpace(applied), mode) {
		t.Fatalf("journal_mode=%s was applied as %q", mode, applied)
	}
}

// TestQuiescentJournalModes pins the snapshot-identity probe. DELETE, TRUNCATE
// and PERSIST are accepted: each keeps a rollback journal on disk for the length
// of a write transaction, so the main .db file alone is a transaction-atomic
// image that the plan digest can describe. WAL is refused because committed rows
// stay in the -wal sidecar until a checkpoint. MEMORY and OFF are refused too:
// neither keeps a disk rollback journal, MEMORY holds it in process memory that
// dies with the process, and OFF gives up SQLite's atomic commit/rollback
// guarantee, so an interrupted write can leave a torn main file that no journal
// restores. This probe used to accept both modes; those two subtests fail
// against that earlier behaviour.
//
// What this probe can NOT prove: a rollback journal says nothing about other
// connections. An external writer can commit between the probe and the read that
// follows, so the source must still be an isolated, quiescent copy.
func TestQuiescentJournalModes(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"delete", "truncate", "persist"} {
		t.Run("rollback journal "+mode, func(t *testing.T) {
			path := capabilityFile(t, mode+".db", "file:%s?mode=rwc",
				`CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT)`)
			opened := capabilityReaderFor(t, path)
			caps := opened.(capabilityProbes)
			capabilityJournalMode(t, ctx, opened, mode)
			if err := caps.Quiescent(ctx); err != nil {
				t.Fatalf("%s rollback-journal source refused: %v", mode, err)
			}
		})
	}

	for _, mode := range []string{"memory", "off"} {
		t.Run("no rollback journal "+mode, func(t *testing.T) {
			path := capabilityFile(t, mode+".db", "file:%s?mode=rwc",
				`CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT)`)
			opened := capabilityReaderFor(t, path)
			caps := opened.(capabilityProbes)
			capabilityJournalMode(t, ctx, opened, mode)
			err := caps.Quiescent(ctx)
			if !errors.Is(err, contracts.ErrIncomplete) {
				t.Fatalf("%s source error=%v, want ErrIncomplete", mode, err)
			}
			// The refusal is a category error: no path, table or value reaches it.
			if err.Error() != contracts.ErrIncomplete.Error() {
				t.Fatalf("%s refusal leaked detail: %q", mode, err.Error())
			}
		})
	}

	t.Run("wal", func(t *testing.T) {
		path := capabilityFile(t, "wal.db", "file:%s?mode=rwc&_pragma=journal_mode(wal)",
			`CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT)`)
		reader := capabilityReaderFor(t, path)
		caps := reader.(capabilityProbes)
		if err := caps.Quiescent(ctx); !errors.Is(err, contracts.ErrIncomplete) {
			t.Fatalf("WAL source error=%v, want ErrIncomplete", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		path := capabilityFile(t, "cancel.db", "file:%s?mode=rwc",
			`CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT)`)
		caps := capabilityReaderFor(t, path).(capabilityProbes)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := caps.Quiescent(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled probe error=%v, want context.Canceled", err)
		}
	})
}

// TestHasTriggers pins the trigger probe: the frozen TableSchema has no trigger
// metadata, so this is the only way to learn that a table can fire a trigger.
func TestHasTriggers(t *testing.T) {
	ctx := context.Background()
	path := capabilityFile(t, "trigger.db", "file:%s?mode=rwc",
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT)`,
		`CREATE TABLE audit (id INTEGER PRIMARY KEY, note TEXT)`,
		`CREATE TRIGGER people_email_audit AFTER UPDATE OF email ON people
		 BEGIN INSERT INTO audit(note) VALUES ('changed'); END`,
	)
	reader := capabilityReaderFor(t, path)
	caps := reader.(capabilityProbes)

	for _, tc := range []struct {
		table string
		want  bool
	}{
		{"people", true},
		{"audit", false},
	} {
		has, err := caps.HasTriggers(ctx, contracts.TableRef{Database: "capability", Schema: "main", Table: tc.table})
		if err != nil {
			t.Fatalf("HasTriggers(%s): %v", tc.table, err)
		}
		if has != tc.want {
			t.Fatalf("HasTriggers(%s)=%t, want %t", tc.table, has, tc.want)
		}
	}
	if _, err := caps.HasTriggers(ctx, contracts.TableRef{Database: "other", Schema: "main", Table: "people"}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("foreign dataset ref error=%v, want ErrInvalid", err)
	}
}

// TestCheckMentionsColumn pins the conservative CHECK scanner: a reference to
// the column is found in every quoting form, and an unrelated name is not.
func TestCheckMentionsColumn(t *testing.T) {
	for _, tc := range []struct {
		expression string
		column     string
		want       bool
	}{
		{"( email <> 'x' )", "email", true},
		{"( upper ( email ) <> 'X' )", "email", true},
		{"( \"email\" IS NOT NULL )", "email", true},
		{"( [email] IS NOT NULL )", "email", true},
		{"( `email` IS NOT NULL )", "email", true},
		{"( email_suffix <> '' )", "email", false},
		{"( length ( nickname ) > 3 )", "email", false},
		{"( quote ( email ) <> '' )", "quote", true},
		{"", "email", false},
		{"( unterminated", "email", true},
	} {
		if got := CheckMentionsColumn(tc.expression, tc.column); got != tc.want {
			t.Fatalf("CheckMentionsColumn(%q,%q)=%t, want %t", tc.expression, tc.column, got, tc.want)
		}
	}
	if CheckMentionsColumn("( email IS NOT NULL )", "") {
		t.Fatal("an empty column name must never match")
	}
}
