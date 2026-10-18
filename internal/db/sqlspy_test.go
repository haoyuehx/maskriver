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

func TestClassifyStatementIsFailClosed(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		verb      string
		write     bool
	}{
		{"select", "SELECT 1", "SELECT", false},
		{"lower case select", "select 1", "SELECT", false},
		{"leading comment", "/* note */ SELECT 1", "SELECT", false},
		{"semicolon inside literal is not a separator", "SELECT 'a;b'", "SELECT", false},
		{"read pragma with schema", `PRAGMA main.table_xinfo("people")`, "PRAGMA", false},
		{"read pragma without schema", "PRAGMA index_list('people')", "PRAGMA", false},
		{"bare journal mode read", "PRAGMA main.journal_mode", "PRAGMA", false},
		{"journal mode assignment", "PRAGMA journal_mode(WAL)", "PRAGMA", true},
		{"journal mode equals assignment", "PRAGMA journal_mode=WAL", "PRAGMA", true},
		{"pragma assignment with equals", "PRAGMA user_version = 5", "PRAGMA", true},
		{"pragma assignment with parens", "PRAGMA user_version(9)", "PRAGMA", true},
		{"pragma wal checkpoint", "PRAGMA wal_checkpoint(FULL)", "PRAGMA", true},
		{"pragma not in allowlist", "PRAGMA writable_schema", "PRAGMA", true},
		{"pragma then write", "PRAGMA main.table_info('people'); UPDATE people SET email='x'", "PRAGMA", true},
		{"multi statement select then drop", "SELECT 1; DROP TABLE main.people", "SELECT", true},
		{"multi statement select then delete", "SELECT 1;DELETE FROM people", "SELECT", true},
		{"cte delete", "WITH x AS (SELECT 1) DELETE FROM people", "WITH", true},
		{"cte select", "WITH x AS (SELECT 1) SELECT * FROM x", "WITH", false},
		{"read pragma encoding is not allowlisted", "PRAGMA encoding", "PRAGMA", true},
		{"explain of a write is planning only", "EXPLAIN DELETE FROM people", "EXPLAIN", false},
		{"values form", "VALUES (1), (2)", "VALUES", false},
		{"attach", "ATTACH DATABASE 'x.db' AS other", "ATTACH", true},
		{"vacuum into", "VACUUM INTO 'copy.db'", "VACUUM", true},
		{"begin immediate", "BEGIN IMMEDIATE", "BEGIN", true},
		{"empty", "   ", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verb, write := classifyStatement(tt.statement)
			if verb != tt.verb || write != tt.write {
				t.Fatalf("classify(%q) = (%q,%t), want (%q,%t)", tt.statement, verb, write, tt.verb, tt.write)
			}
		})
	}
}

// TestSQLSpyCatchesMultiStatementWrite is the P1-1 regression: a Prepare text
// that starts with SELECT but carries a later write must be write intent.
func TestSQLSpyCatchesMultiStatementWrite(t *testing.T) {
	ctx := context.Background()
	opts := sqliteOptions(t)
	spy := NewSQLSpy()
	opened, err := spy.OpenWriter(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	writer := opened.(*writer)
	// Execute raw multi-statement text through the same path a driver statement
	// would take, then confirm it is classified as write intent. A read-only
	// Reader rejects the write outright, which is why the writer adapter is used
	// here: the point is the classifier, not the read-only guard.
	if _, execErr := writer.db.ExecContext(ctx, "SELECT 1; CREATE TABLE spy_probe (id INTEGER)"); execErr != nil {
		t.Fatalf("raw exec: %v", execErr)
	}
	if writes := spy.Writes(); len(writes) == 0 {
		t.Fatalf("multi-statement write escaped classification: %+v", spy.Statements())
	}
	if err = opened.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestSQLSpyCatchesPragmaAssignment is the P1-2 regression: PRAGMA name(value)
// is SQLite's assignment form and must not be classified as a read.
func TestSQLSpyCatchesPragmaAssignment(t *testing.T) {
	ctx := context.Background()
	opts := sqliteOptions(t)
	spy := NewSQLSpy()
	opened, err := spy.OpenWriter(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	writer := opened.(*writer)
	if _, execErr := writer.db.ExecContext(ctx, "PRAGMA user_version(9)"); execErr != nil {
		t.Fatalf("pragma: %v", execErr)
	}
	if writes := spy.Writes(); len(writes) == 0 {
		t.Fatalf("PRAGMA assignment escaped classification: %+v", spy.Statements())
	}
	if err = opened.Close(); err != nil {
		t.Fatal(err)
	}
}
