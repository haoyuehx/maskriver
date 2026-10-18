package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/haoyuehx/maskriver/internal/testenv"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// spyTreeDigest hashes every regular file next to the database, so a test can
// prove that no database file, -wal or -shm appeared or changed.
func spyTreeDigest(t *testing.T, dir string) map[string][32]byte {
	t.Helper()
	paths := make([]string, 0)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	digest := make(map[string][32]byte, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		digest[filepath.Base(path)] = sha256.Sum256(content)
	}
	return digest
}

func spyVerbs(statements []SpyStatement) map[string]int {
	verbs := make(map[string]int)
	for _, statement := range statements {
		verbs[statement.Verb]++
	}
	return verbs
}

// TestSQLSpyReadOnlyOperationsIssueNoWrites runs the operations a Preview
// performs through a spy-backed real SQLite Reader. It asserts at the driver
// level that no writing or transaction statement reaches the database and that
// the SQLite file tree is byte identical afterwards.
func TestSQLSpyReadOnlyOperationsIssueNoWrites(t *testing.T) {
	ctx := context.Background()
	opts := sqliteOptions(t)
	dir := filepath.Dir(opts.DSN.Payload())
	before := spyTreeDigest(t, dir)

	spy := NewSQLSpy()
	opened, err := spy.OpenReader(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opened.Close() }()

	if _, err = opened.Describe(ctx, table("people")); err != nil {
		t.Fatalf("describe: %v", err)
	}
	count, err := opened.Count(ctx, table("people"))
	if err != nil || count != testenv.Rows {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if _, err = opened.Sample(ctx, contracts.SampleRequest{
		Column: contracts.ColumnRef{TableRef: table("people"), Column: "nickname"},
		Limit:  100,
	}); err != nil {
		t.Fatalf("sample: %v", err)
	}
	var after []contracts.Value
	pages := 0
	for {
		page, pageErr := opened.Page(ctx, contracts.PageRequest{
			Table: table("people"), Columns: []string{"id", "email"},
			KeyColumns: []string{"id"}, After: after, Limit: 10,
		})
		if pageErr != nil {
			t.Fatalf("page: %v", pageErr)
		}
		pages++
		if page.Done {
			break
		}
		after = page.Next
	}
	if pages == 0 {
		t.Fatal("no page read")
	}
	if err = opened.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if writes := spy.Writes(); len(writes) != 0 {
		t.Fatalf("read-only operations issued write/transaction intent: %+v", writes)
	}
	statements := spy.Statements()
	verbs := spyVerbs(statements)
	if verbs["SELECT"] == 0 {
		t.Fatalf("spy observed no SELECT statement; the read assertion would be vacuous: %+v", statements)
	}
	if verbs["PRAGMA"] == 0 {
		t.Fatalf("spy observed no read PRAGMA; statement capture is incomplete: %+v", statements)
	}
	if after := spyTreeDigest(t, dir); len(after) != len(before) || !sameDigest(before, after) {
		t.Fatalf("SQLite file tree changed: before=%v after=%v", before, after)
	}
}

func sameDigest(a, b map[string][32]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for name, sum := range a {
		other, ok := b[name]
		if !ok || other != sum {
			return false
		}
	}
	return true
}

// TestSQLSpyWriterRecordsWriteStatements is the negative control: it proves the
// spy wrapper and classifier both observe writes when the code really writes.
func TestSQLSpyWriterRecordsWriteStatements(t *testing.T) {
	ctx := context.Background()
	opts := sqliteOptions(t)
	spy := NewSQLSpy()
	opened, err := spy.OpenWriter(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	plan := contracts.TablePlan{
		Table:      table("people"),
		PrimaryKey: []string{"id"},
		Columns: []contracts.ColumnPlan{{
			Column: contracts.ColumnRef{TableRef: table("people"), Column: "email"},
			Type:   contracts.Type{Kind: contracts.Text, Native: "TEXT"},
		}},
	}
	tx, err := opened.Begin(ctx, plan)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	updated, err := tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{testValue(t, contracts.Int, "1")},
		Expected:    contracts.Row{"email": testValue(t, contracts.Text, "fixture-01@example.invalid")},
		Replacement: contracts.Row{"email": testValue(t, contracts.Text, "synthetic-01@example.invalid")},
	}})
	if err != nil || updated != 1 {
		t.Fatalf("update=%d err=%v", updated, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err = opened.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	verbs := spyVerbs(spy.Writes())
	for _, verb := range []string{"BEGIN", "UPDATE", "COMMIT"} {
		if verbs[verb] == 0 {
			t.Fatalf("spy did not classify %s as write intent: %+v", verb, spy.Statements())
		}
	}

	// The wrapped connection must still forward to the real driver.
	readback, err := sql.Open("sqlite", "file:"+opts.DSN.Payload()+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer readback.Close()
	var email string
	if err = readback.QueryRowContext(ctx, `SELECT email FROM main.people WHERE id=1`).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "synthetic-01@example.invalid" {
		t.Fatalf("write did not reach SQLite: %q", email)
	}
}
