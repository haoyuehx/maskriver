package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
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

// TestQuiescentJournalModes pins the snapshot-identity probe: a rollback-journal
// source is accepted and a WAL source is refused, because only the former keeps
// every committed row in the main .db file the plan digest hashes.
func TestQuiescentJournalModes(t *testing.T) {
	ctx := context.Background()
	t.Run("rollback journal", func(t *testing.T) {
		path := capabilityFile(t, "rollback.db", "file:%s?mode=rwc",
			`CREATE TABLE t (id INTEGER PRIMARY KEY, email TEXT)`)
		reader := capabilityReaderFor(t, path)
		caps := reader.(capabilityProbes)
		if err := caps.Quiescent(ctx); err != nil {
			t.Fatalf("quiescent rollback-journal source refused: %v", err)
		}
	})

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
