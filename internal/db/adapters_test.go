package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// buildSQLiteDB writes a fresh SQLite DB at a temporary path with a
// deterministic fixture of 25 rows, 3 columns, and a composite primary
// key (a, b). DSN returned has the raw file path.
func buildSQLiteDB(t *testing.T, rows int) (dsn contracts.Value, path string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "fixture.db")
	dsnVal, _ := contracts.NewValue(contracts.Text, p)

	// Open RW and create schema + rows.
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatalf("sqlite open err=%v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE users (
		a INTEGER NOT NULL,
		b TEXT NOT NULL,
		c TEXT,
		d INTEGER,
		PRIMARY KEY (a,b)
	)`); err != nil {
		t.Fatalf("sqlite create err=%v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("tx err=%v", err)
	}
	stmt, err := tx.Prepare("INSERT INTO users(a,b,c,d) VALUES(?,?,?,?)")
	if err != nil {
		t.Fatalf("prepare err=%v", err)
	}
	for i := 0; i < rows; i++ {
		a := i / 100
		b := fmt.Sprintf("b%03d", i)
		c := fmt.Sprintf("email%03d@example.com", i%5)
		var d *int
		if i%3 == 0 {
			n := i * 10
			d = &n
		}
		var dArg any
		if d == nil {
			dArg = nil
		} else {
			dArg = *d
		}
		if _, err2 := stmt.Exec(a, b, c, dArg); err2 != nil {
			t.Fatalf("insert %d err=%v", i, err2)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatalf("stmt close err=%v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit err=%v", err)
	}
	return dsnVal, p
}

func requireNoDangling(t *testing.T, opts contracts.DatabaseOptions) {
	t.Helper()
	// Close should be idempotent.
}

func optsFromSQLiteDSN(dsn contracts.Value) contracts.DatabaseOptions {
	return contracts.DatabaseOptions{
		Dialect:   contracts.SQLite,
		DSN:       dsn,
		DatasetID: "unit",
		Schemas:   []string{"main"},
	}
}

func Test_SQLite_ReaderMissingFile_NotFound(t *testing.T) {
	dsn, _ := contracts.NewValue(contracts.Text, filepath.Join(t.TempDir(), "absent.db"))
	_, err := OpenReader(context.Background(), optsFromSQLiteDSN(dsn))
	if err != contracts.ErrNotFound {
		t.Fatalf("missing file want ErrNotFound got %v", err)
	}
	// Ensure the file was never created.
	if _, statErr := os.Stat(dsn.Payload()); !os.IsNotExist(statErr) {
		t.Fatalf("OpenReader created DB file despite missing path (UNSAFE): %v", dsn.Payload())
	}
}

func Test_SQLite_RO_WriteRefused(t *testing.T) {
	dsn, _ := buildSQLiteDB(t, 10)
	r, err := OpenReader(context.Background(), optsFromSQLiteDSN(dsn))
	if err != nil {
		t.Fatalf("open reader err=%v", err)
	}
	defer r.Close()
	// Cast to sqliteHandle to reach underlying db.
	sr := r.(*sqliteHandle)
	if _, err2 := sr.db.Exec("INSERT INTO users(a,b,c) VALUES(999,'x','y')"); err2 == nil {
		t.Fatalf("RO reader allowed INSERT (UNSAFE)")
	}
}

func Test_SQLite_ReaderTablesDescribeCount(t *testing.T) {
	dsn, _ := buildSQLiteDB(t, 3)
	r, err := OpenReader(context.Background(), optsFromSQLiteDSN(dsn))
	if err != nil {
		t.Fatalf("open err=%v", err)
	}
	defer r.Close()
	tables, err := r.Tables(context.Background(), []string{"main"})
	if err != nil {
		t.Fatalf("Tables err=%v", err)
	}
	if len(tables) != 1 || tables[0].Table != "users" {
		t.Fatalf("Tables want [users] got %+v", tables)
	}
	sch, err := r.Describe(context.Background(), tables[0])
	if err != nil {
		t.Fatalf("Describe err=%v", err)
	}
	if len(sch.Columns) != 4 {
		t.Fatalf("Describe columns want 4 got %d", len(sch.Columns))
	}
	if strings.Join(sch.PrimaryKey, ",") != "a,b" {
		t.Fatalf("Describe PK want [a b] got %+v", sch.PrimaryKey)
	}
	n, err := r.Count(context.Background(), tables[0])
	if err != nil {
		t.Fatalf("Count err=%v", err)
	}
	if n != 3 {
		t.Fatalf("Count want 3 got %d", n)
	}
}

func Test_SQLite_Sample(t *testing.T) {
	dsn, _ := buildSQLiteDB(t, 25)
	r, err := OpenReader(context.Background(), optsFromSQLiteDSN(dsn))
	if err != nil {
		t.Fatalf("open err=%v", err)
	}
	defer r.Close()
	col := contracts.ColumnRef{
		TableRef: contracts.TableRef{Database: "unit", Schema: "main", Table: "users"},
		Column:   "c",
	}
	s, err := r.Sample(context.Background(), contracts.SampleRequest{Column: col, Limit: 3})
	if err != nil {
		t.Fatalf("Sample err=%v", err)
	}
	if s.Basis != contracts.DistinctNonNull {
		t.Fatalf("Sample.Basis want DistinctNonNull got %v", s.Basis)
	}
	if s.Truncated != true {
		t.Fatalf("Sample.Truncated want true got false (limit=3 distinct emails >=4)")
	}
	if len(s.Values) > 3 {
		t.Fatalf("Sample len want <=3 got %d", len(s.Values))
	}
	// Truncated=false when limit is large enough.
	s, err = r.Sample(context.Background(), contracts.SampleRequest{Column: col, Limit: 1000})
	if err != nil {
		t.Fatalf("Sample(err) big limit err=%v", err)
	}
	if s.Truncated {
		t.Fatalf("Sample want !truncated when limit >= distinct count")
	}
	if len(s.Values) != 5 {
		t.Fatalf("Sample distinct email count want 5 got %d", len(s.Values))
	}
}

func Test_SQLite_Page_CompositeKey(t *testing.T) {
	rows := 25
	dsn, _ := buildSQLiteDB(t, rows)
	r, err := OpenReader(context.Background(), optsFromSQLiteDSN(dsn))
	if err != nil {
		t.Fatalf("open err=%v", err)
	}
	defer r.Close()
	table := contracts.TableRef{Database: "unit", Schema: "main", Table: "users"}
	cols := []string{"a", "b", "c", "d"}
	keys := []string{"a", "b"}
	limit := 10
	var pages int
	var after []contracts.Value
	total := 0
	for {
		p, err := r.Page(context.Background(), contracts.PageRequest{
			Table: table, Columns: cols, KeyColumns: keys, After: after, Limit: limit,
		})
		if err != nil {
			t.Fatalf("Page err=%v after=%+v", err, after)
		}
		pages++
		total += len(p.Rows)
		if p.Done {
			break
		}
		if len(p.Next) != len(keys) {
			t.Fatalf("Next want %d got %d", len(keys), len(p.Next))
		}
		after = p.Next
		if pages > 10 {
			t.Fatalf("too many pages")
		}
	}
	if total != rows {
		t.Fatalf("paged total want %d got %d (pages=%d)", rows, total, pages)
	}
	if pages != 3 {
		t.Fatalf("page count want 3 (25 rows, limit 10) got %d", pages)
	}
	// Ensure key columns in first row match [a=0, b=b000] deterministically.
	p, _ := r.Page(context.Background(), contracts.PageRequest{
		Table: table, Columns: cols, KeyColumns: keys, Limit: 2,
	})
	if len(p.Rows) == 0 {
		t.Fatalf("empty first page")
	}
	r0 := p.Rows[0]
	if r0["a"].Payload() != "0" || r0["b"].Payload() != "b000" {
		t.Fatalf("page 0 row 0 key want (0,b000) got (%s,%s)", r0["a"].Payload(), r0["b"].Payload())
	}
}

func Test_SQLite_Page_MissingKeys_Unsupported(t *testing.T) {
	dsn, _ := buildSQLiteDB(t, 2)
	r, _ := OpenReader(context.Background(), optsFromSQLiteDSN(dsn))
	defer r.Close()
	if _, err := r.Page(context.Background(), contracts.PageRequest{
		Table:   contracts.TableRef{Schema: "main", Table: "users"},
		Columns: []string{"a", "b"}, KeyColumns: nil, Limit: 1,
	}); err != contracts.ErrUnsupported {
		t.Fatalf("no keys want ErrUnsupported got %v", err)
	}
}

func Test_SQLite_Page_AfterLengthMismatch_Invalid(t *testing.T) {
	dsn, _ := buildSQLiteDB(t, 2)
	r, _ := OpenReader(context.Background(), optsFromSQLiteDSN(dsn))
	defer r.Close()
	bad := []contracts.Value{{}}
	if _, err := r.Page(context.Background(), contracts.PageRequest{
		Table:   contracts.TableRef{Schema: "main", Table: "users"},
		Columns: []string{"a", "b"}, KeyColumns: []string{"a", "b"}, After: bad, Limit: 1,
	}); err != contracts.ErrInvalid {
		t.Fatalf("After len mismatch want ErrInvalid got %v", err)
	}
}

func Test_SQLite_Writer_Begin_ExactRowsAffected_RollbackOnConflict(t *testing.T) {
	dsn, _ := buildSQLiteDB(t, 5)
	opts := optsFromSQLiteDSN(dsn)
	w, err := OpenWriter(context.Background(), opts)
	if err != nil {
		t.Fatalf("OpenWriter err=%v", err)
	}
	defer w.Close()
	plan := contracts.TablePlan{
		Table:      contracts.TableRef{Database: "unit", Schema: "main", Table: "users"},
		PrimaryKey: []string{"a", "b"},
		Columns: []contracts.ColumnPlan{
			{Column: contracts.ColumnRef{Column: "c"}},
		},
	}
	tx, err := w.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin err=%v", err)
	}
	// Good update: row (a=0,b=b000) exists with c=email000@example.com.
	k0 := mustKeyVals(t, "0", "b000")
	n, err := tx.Update(context.Background(), []contracts.RowChange{{
		Key:         k0,
		Expected:    rowC(t, "email000@example.com"),
		Replacement: rowC(t, "masked0"),
	}})
	if err != nil {
		t.Fatalf("Update good err=%v", err)
	}
	if n != 1 {
		t.Fatalf("Update want 1 got %d", n)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("Commit err=%v", err)
	}
	// Verify via Reader.
	r, err := OpenReader(context.Background(), opts)
	if err != nil {
		t.Fatalf("r open err=%v", err)
	}
	defer r.Close()
	p, _ := r.Page(context.Background(), contracts.PageRequest{
		Table: plan.Table, Columns: []string{"a", "b", "c"}, KeyColumns: []string{"a", "b"}, Limit: 1,
	})
	if len(p.Rows) == 0 || p.Rows[0]["c"].Payload() != "masked0" {
		t.Fatalf("applied mask not visible: %+v", p.Rows)
	}
	// Conflict update: Expected != actual => ErrConflict and tx aborts.
	w2, _ := OpenWriter(context.Background(), opts)
	defer w2.Close()
	tx2, err := w2.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin2 err=%v", err)
	}
	if _, err := tx2.Update(context.Background(), []contracts.RowChange{{
		Key:         k0,
		Expected:    rowC(t, "definitely-not-the-actual-value"),
		Replacement: rowC(t, "other"),
	}}); err != contracts.ErrConflict {
		t.Fatalf("conflict want ErrConflict got %v", err)
	}
	// Subsequent Update after ErrConflict must fail (tx should be closed).
	if _, err2 := tx2.Update(context.Background(), []contracts.RowChange{{Key: k0, Expected: rowC(t, "masked0"), Replacement: rowC(t, "x")}}); err2 == nil {
		t.Fatalf("Update after conflict must fail (tx rolled back)")
	}
	// The failed tx must not have committed.
	p, _ = r.Page(context.Background(), contracts.PageRequest{
		Table: plan.Table, Columns: []string{"a", "b", "c"}, KeyColumns: []string{"a", "b"}, Limit: 1,
	})
	if p.Rows[0]["c"].Payload() != "masked0" {
		t.Fatalf("conflicting tx leaked writes: %+v", p.Rows[0])
	}
}

func Test_SQLite_Writer_Rollback_AfterCancel(t *testing.T) {
	dsn, _ := buildSQLiteDB(t, 5)
	w, _ := OpenWriter(context.Background(), optsFromSQLiteDSN(dsn))
	defer w.Close()
	plan := contracts.TablePlan{
		Table:      contracts.TableRef{Database: "unit", Schema: "main", Table: "users"},
		PrimaryKey: []string{"a", "b"},
		Columns: []contracts.ColumnPlan{
			{Column: contracts.ColumnRef{Column: "c"}},
		},
	}
	tx, _ := w.Begin(context.Background(), plan)
	// No-op: just ensure Rollback on a non-started state is safe, even
	// after callers have cancelled their own context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback after cancel should be idempotent: err=%v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("double Rollback must be safe: err=%v", err)
	}
}

func Test_MySQL_UNVERIFIED_Skip(t *testing.T) {
	// M1-B scope: MySQL UNVERIFIED pending issue #10.
	t.Skip("UNVERIFIED: MySQL isolated DB required (issue #10 blocks this environment)")
}

func requireValue(t *testing.T, kind contracts.Kind, raw string) contracts.Value {
	v, err := contracts.NewValue(kind, raw)
	if err != nil {
		t.Fatalf("NewValue(%v,%q) err=%v", kind, truncForT(raw), err)
	}
	return v
}

func truncForT(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:16] + "…"
}

func mustKeyVals(t *testing.T, a, b string) []contracts.Value {
	return []contracts.Value{requireValue(t, contracts.Int, a), requireValue(t, contracts.Text, b)}
}

func rowC(t *testing.T, c string) contracts.Row {
	return contracts.Row{"c": requireValue(t, contracts.Text, c)}
}
