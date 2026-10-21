package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const hotJournalChildEnv = "MASKRIVER_HOT_JOURNAL_CHILD"

// TestSQLiteDSNCarriesBusyTimeout guards the 5 s wait that keeps a concurrent
// read-only status from turning a writer's transient lock into CommitUnknown.
func TestSQLiteDSNCarriesBusyTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	for _, readOnly := range []bool{false, true} {
		dsn, err := sqliteDSN(path, readOnly)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(dsn, "busy_timeout") {
			t.Fatalf("dsn missing busy_timeout: %s", dsn)
		}
	}
}

// TestHotJournalChild leaves an uncommitted write and exits without closing the
// connection, producing a real hot rollback journal.
func TestHotJournalChild(t *testing.T) {
	if os.Getenv(hotJournalChildEnv) != "1" {
		t.Skip("helper process only")
	}
	path := os.Args[len(os.Args)-1]
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		os.Exit(2)
	}
	if _, err := db.Exec(`PRAGMA cache_size=1`); err != nil {
		os.Exit(3)
	}
	if _, err := db.Exec(`BEGIN IMMEDIATE`); err != nil {
		os.Exit(4)
	}
	if _, err := db.Exec(`UPDATE people SET email='uncommitted@example.invalid'`); err != nil {
		os.Exit(5)
	}
	os.Exit(9)
}

func TestHotJournalRecoveryAndReadOnlyStatus(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the streaming outbox/recovery capability is Linux-only")
	}
	opts, _ := recoveryTestOptions(t)
	o, err := OpenOutbox(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.InitializeRun(context.Background(), OutboxRun{
		RunID: "abcdefabcdefabcdefabcdefabcdefab", PlanID: strings.Repeat("a", 64),
		ApprovalBinding: contracts.ApprovalBindingRevision,
		TargetDigest:    strings.Repeat("e", 64), ScopeDigest: strings.Repeat("c", 64),
		Protocol: contracts.StreamingProtocolRevision,
	}); err != nil {
		t.Fatal(err)
	}
	path := opts.DSN.Payload()
	recovery, err := o.OpenRecovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := recovery.CreateManifest(context.Background(), recoveryTestManifest("abcdefabcdefabcdefabcdefabcdefab")); err != nil {
		t.Fatal(err)
	}
	// Make the table large enough that the uncommitted update spills DB pages.
	seed, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`WITH RECURSIVE cnt(x) AS (SELECT 4 UNION ALL SELECT x+1 FROM cnt WHERE x < 5000) INSERT INTO people(id,email) SELECT x,'seed'||x||'@example.invalid' FROM cnt`); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	// Create a real hot journal in a separate process.
	command := exec.Command(os.Args[0], "-test.run=^TestHotJournalChild$", "--", path)
	command.Env = append(os.Environ(), hotJournalChildEnv+"=1")
	_ = command.Run() // exit 9 is the expected child outcome
	if !journalHot(path) {
		t.Fatal("fixture did not leave a hot journal")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := OpenRecoveryReader(context.Background(), opts.DatabaseOptions)
	if err == nil || !errors.Is(err, ErrHotJournal) {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatalf("read-only status on hot journal: got %v, want ErrHotJournal", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || !journalHot(path) {
		t.Fatal("read-only status changed the database or removed the journal")
	}
	if err := RecoverSQLiteJournal(context.Background(), opts.DatabaseOptions); err != nil {
		t.Fatalf("recover hot journal: %v", err)
	}
	if journalHot(path) {
		t.Fatal("journal still present after recovery")
	}
	reader, err = OpenRecoveryReader(context.Background(), opts.DatabaseOptions)
	if err != nil {
		t.Fatalf("read-only status after recovery: %v", err)
	}
	_ = reader.Close()
}
