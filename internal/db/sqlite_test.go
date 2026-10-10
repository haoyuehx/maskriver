package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/testenv"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func testValue(t *testing.T, kind contracts.Kind, raw string) contracts.Value {
	t.Helper()
	v, err := contracts.NewValue(kind, raw)
	if err != nil {
		t.Fatal("invalid synthetic value")
	}
	return v
}

func sqliteOptions(t *testing.T) contracts.DatabaseOptions {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	seedDB, err := sql.Open("sqlite", "file:"+path+"?mode=rwc&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if err = testenv.Seed(context.Background(), seedDB, "sqlite"); err != nil {
		t.Fatal(err)
	}
	if err = seedDB.Close(); err != nil {
		t.Fatal(err)
	}
	return contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: testValue(t, contracts.Text, path),
		DatasetID: "fixture", Schemas: []string{"main"}, MaxOpenConns: 1,
		ConnectTimeout: 5 * time.Second,
	}
}

func table(name string) contracts.TableRef {
	return contracts.TableRef{Database: "fixture", Schema: "main", Table: name}
}

func TestSQLiteReadAdapter(t *testing.T) {
	ctx := context.Background()
	opts := sqliteOptions(t)
	opened, err := OpenReader(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	r := opened.(*reader)
	tables, err := r.Tables(ctx, nil)
	if err != nil || len(tables) != 3 {
		t.Fatalf("table discovery: %v, %d", err, len(tables))
	}
	schema, err := r.Describe(ctx, table("people"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema.PrimaryKey, []string{"id"}) ||
		!schema.Coverage.Columns || !schema.Coverage.PrimaryKey ||
		!schema.Coverage.Indexes || !schema.Coverage.ForeignKeys ||
		!schema.Coverage.Unique || schema.Coverage.Checks {
		t.Fatal("schema metadata or coverage mismatch")
	}
	n, err := r.Count(ctx, table("people"))
	if err != nil || n != 25 {
		t.Fatalf("count: %v, %d", err, n)
	}
	sample, err := r.Sample(ctx, contracts.SampleRequest{
		Column: contracts.ColumnRef{TableRef: table("people"), Column: "nickname"}, Limit: 1,
	})
	if err != nil || sample.Basis != contracts.DistinctNonNull ||
		len(sample.Values) != 1 || !sample.Truncated {
		t.Fatal("bounded sample mismatch", err)
	}
	var after []contracts.Value
	total := 0
	pageSizes := []int{}
	for {
		page, err := r.Page(ctx, contracts.PageRequest{
			Table: table("people"), Columns: []string{"id", "email", "nickname"},
			KeyColumns: []string{"id"}, After: after, Limit: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		pageSizes = append(pageSizes, len(page.Rows))
		for _, row := range page.Rows {
			total++
			if row["id"].Payload() != testValue(t, contracts.Int, itoa(total)).Payload() {
				t.Fatal("keyset order changed")
			}
		}
		if page.Done {
			break
		}
		if len(page.Next) != 1 {
			t.Fatal("missing cursor")
		}
		after = page.Next
	}
	if total != 25 || !reflect.DeepEqual(pageSizes, []int{10, 10, 5}) {
		t.Fatal("page boundary mismatch", pageSizes)
	}
	links, err := r.Describe(ctx, table("links"))
	if err != nil || !reflect.DeepEqual(links.PrimaryKey, []string{"tenant", "seq"}) {
		t.Fatal("composite primary key mismatch", err)
	}
	after = nil
	total = 0
	for {
		page, err := r.Page(ctx, contracts.PageRequest{
			Table: table("links"), Columns: []string{"tenant", "seq", "person_id"},
			KeyColumns: []string{"tenant", "seq"}, After: after, Limit: 7,
		})
		if err != nil {
			t.Fatal(err)
		}
		total += len(page.Rows)
		if page.Done {
			break
		}
		after = page.Next
	}
	if total != 25 {
		t.Fatal("composite cursor lost rows")
	}
	if _, err = r.Page(ctx, contracts.PageRequest{Table: table("keyless"), Columns: []string{"note"}, Limit: 10}); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatal("keyless table should refuse paging")
	}
	if _, err = r.Page(ctx, contracts.PageRequest{Table: table("people"), Columns: []string{"id"}, KeyColumns: []string{"id"}, Limit: 0}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("zero limit should be rejected")
	}
	if _, err = r.Page(ctx, contracts.PageRequest{Table: table("people"), Columns: []string{"id", "hostile\" FROM people --"}, KeyColumns: []string{"id"}, Limit: 1}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("unknown identifier should be rejected")
	}
	if _, err = r.db.ExecContext(ctx, "UPDATE people SET nickname='bad' WHERE id=1"); err == nil {
		t.Fatal("reader connection allowed UPDATE")
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Count(ctx, table("people")); !errors.Is(err, contracts.ErrClosed) {
		t.Fatal("closed reader accepted operation")
	}
}

func TestSQLiteMissingDatabaseNeverCreated(t *testing.T) {
	opts := sqliteOptions(t)
	missing := filepath.Join(t.TempDir(), "missing.db")
	opts.DSN = testValue(t, contracts.Text, missing)
	if _, err := OpenReader(context.Background(), opts); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatal("missing read-only database should be not found")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reader created database")
	}
}

func TestSQLiteWriteTransactions(t *testing.T) {
	ctx := context.Background()
	opts := sqliteOptions(t)
	opened, err := OpenWriter(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	w := opened.(*writer)
	schema, err := w.describe(ctx, table("people"))
	if err != nil {
		t.Fatal(err)
	}
	var nick contracts.Column
	for _, c := range schema.Columns {
		if c.Ref.Column == "nickname" {
			nick = c
		}
	}
	plan := contracts.TablePlan{
		Table: table("people"), PrimaryKey: []string{"id"},
		Columns: []contracts.ColumnPlan{{Column: nick.Ref, Type: nick.Type}},
	}
	tx, err := w.Begin(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	updated := contracts.RowChange{
		Key:         []contracts.Value{testValue(t, contracts.Int, "1")},
		Expected:    contracts.Row{"nickname": {}},
		Replacement: contracts.Row{"nickname": testValue(t, contracts.Text, "synthetic-new")},
	}
	n, err := tx.Update(ctx, []contracts.RowChange{updated})
	if err != nil || n != 1 {
		t.Fatal("update failed", err, n)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); !errors.Is(err, contracts.ErrClosed) {
		t.Fatal("repeat commit accepted")
	}

	tx, err = w.Begin(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	change2 := contracts.RowChange{
		Key:         []contracts.Value{testValue(t, contracts.Int, "2")},
		Expected:    contracts.Row{"nickname": testValue(t, contracts.Text, "")},
		Replacement: contracts.Row{"nickname": testValue(t, contracts.Text, "second")},
	}
	conflict := contracts.RowChange{
		Key:         []contracts.Value{testValue(t, contracts.Int, "3")},
		Expected:    contracts.Row{"nickname": testValue(t, contracts.Text, "wrong")},
		Replacement: contracts.Row{"nickname": testValue(t, contracts.Text, "third")},
	}
	if _, err = tx.Update(ctx, []contracts.RowChange{change2, conflict}); !errors.Is(err, contracts.ErrConflict) {
		t.Fatal("optimistic conflict not detected", err)
	}
	if err = tx.Commit(ctx); !errors.Is(err, contracts.ErrClosed) {
		t.Fatal("aborted tx accepted commit")
	}
	var nick2 sql.NullString
	if err = w.db.QueryRowContext(ctx, "SELECT nickname FROM people WHERE id=2").Scan(&nick2); err != nil {
		t.Fatal(err)
	}
	if !nick2.Valid || nick2.String != "" {
		t.Fatal("conflict committed earlier row")
	}

	tx, err = w.Begin(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Update(ctx, []contracts.RowChange{change2}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal("rollback not idempotent", err)
	}
	if err = w.db.QueryRowContext(ctx, "SELECT nickname FROM people WHERE id=2").Scan(&nick2); err != nil || nick2.String != "" {
		t.Fatal("rollback persisted change", err)
	}

	tx, err = w.Begin(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = tx.Update(cancelled, []contracts.RowChange{change2}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not propagated", err)
	}
	if err = tx.Commit(ctx); !errors.Is(err, contracts.ErrClosed) {
		t.Fatal("cancelled tx accepted commit")
	}

	bad := plan
	bad.PrimaryKey = []string{}
	if _, err = w.Begin(ctx, bad); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("missing plan key accepted")
	}
	bad = plan
	bad.Columns = []contracts.ColumnPlan{{Column: contracts.ColumnRef{TableRef: table("people"), Column: "id"}, Type: schema.Columns[0].Type}}
	if _, err = w.Begin(ctx, bad); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatal("primary key modification accepted", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
