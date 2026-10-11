package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func TestSQLiteTypedValuesAndMetadataLimits(t *testing.T) {
	ctx := context.Background()
	opts := sqliteOptions(t)
	raw, err := sql.Open("sqlite", opts.DSN.Payload())
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		"CREATE TABLE empty_table (id INTEGER PRIMARY KEY, note TEXT)",
		"CREATE TABLE \"odd\"\"table\" (id INTEGER PRIMARY KEY, \"x\"\"y\" TEXT)",
		"INSERT INTO \"odd\"\"table\"(id,\"x\"\"y\") VALUES (1,'synthetic')",
		"CREATE TABLE nullable_key (id TEXT PRIMARY KEY, v TEXT)",
		"INSERT INTO nullable_key(id,v) VALUES (NULL,'synthetic')",
		"CREATE TABLE unsigned_key (id UNSIGNED BIG INT PRIMARY KEY)",
		"CREATE TABLE decimal_declared (id INTEGER PRIMARY KEY, amount DECIMAL(24,4))",
		"CREATE TABLE event_time (id INTEGER PRIMARY KEY, happened DATETIME NOT NULL)",
		"INSERT INTO event_time(id,happened) VALUES (1,'2020-01-02 03:04:05.120000')",
		"CREATE INDEX partial_nick ON people(nickname) WHERE nickname IS NOT NULL",
	}
	for _, s := range statements {
		if _, err = raw.ExecContext(ctx, s); err != nil {
			t.Fatal("synthetic DDL failed", err)
		}
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := OpenReader(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	r := opened.(*reader)
	page, err := r.Page(ctx, contracts.PageRequest{
		Table: table("people"), Columns: []string{"id", "nickname", "amount", "born", "payload"},
		KeyColumns: []string{"id"}, Limit: 3,
	})
	if err != nil || len(page.Rows) != 3 {
		t.Fatal("typed page failed", err)
	}
	if !page.Rows[0]["nickname"].IsNull() ||
		page.Rows[1]["nickname"].IsNull() ||
		page.Rows[1]["nickname"].Payload() != "" ||
		page.Rows[2]["amount"].Payload() != "12345678901234567890.1200" ||
		page.Rows[2]["born"].Kind() != contracts.Date ||
		page.Rows[2]["born"].Payload() != "2000-02-29" ||
		page.Rows[2]["payload"].Kind() != contracts.Bytes ||
		page.Rows[2]["payload"].Payload() != string([]byte{0, 1, 255}) {
		t.Fatal("SQLite value roundtrip changed")
	}
	sample, err := r.Sample(ctx, contracts.SampleRequest{
		Column: contracts.ColumnRef{TableRef: table("people"), Column: "nickname"}, Limit: 100,
	})
	if err != nil || sample.Truncated || len(sample.Values) != 24 {
		t.Fatal("distinct non-null sample mismatch", err)
	}
	foundEmpty := false
	for _, v := range sample.Values {
		if !v.IsNull() && v.Payload() == "" {
			foundEmpty = true
		}
	}
	if !foundEmpty {
		t.Fatal("empty string lost from sample")
	}
	empty, err := r.Page(ctx, contracts.PageRequest{
		Table: table("empty_table"), Columns: []string{"id", "note"}, KeyColumns: []string{"id"}, Limit: 10,
	})
	if err != nil || !empty.Done || len(empty.Rows) != 0 || len(empty.Next) != 0 {
		t.Fatal("empty table page mismatch", err)
	}
	odd := table("odd\"table")
	described, err := r.Describe(ctx, odd)
	if err != nil || len(described.Columns) != 2 {
		t.Fatal("quoted table description failed", err)
	}
	oddPage, err := r.Page(ctx, contracts.PageRequest{
		Table: odd, Columns: []string{"id", "x\"y"}, KeyColumns: []string{"id"}, Limit: 1,
	})
	if err != nil || len(oddPage.Rows) != 1 || oddPage.Rows[0]["x\"y"].Payload() != "synthetic" {
		t.Fatal("quoted column query failed", err)
	}
	nullable, err := r.Describe(ctx, table("nullable_key"))
	if err != nil || !nullable.Columns[0].Type.Nullable {
		t.Fatal("nullable SQLite primary key misreported", err)
	}
	if _, err = r.Page(ctx, contracts.PageRequest{
		Table: table("nullable_key"), Columns: []string{"id", "v"}, KeyColumns: []string{"id"}, Limit: 1,
	}); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatal("nullable cursor key accepted", err)
	}
	if _, err = r.Describe(ctx, table("unsigned_key")); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatal("lossy SQLite unsigned type accepted", err)
	}
	if _, err = r.Describe(ctx, table("decimal_declared")); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatal("lossy SQLite decimal affinity accepted", err)
	}
	event, err := r.Page(ctx, contracts.PageRequest{Table: table("event_time"), Columns: []string{"id", "happened"}, KeyColumns: []string{"id"}, Limit: 1})
	if err != nil || len(event.Rows) != 1 || event.Rows[0]["happened"].Payload() != "2020-01-02T03:04:05.12" {
		t.Fatal("local datetime normalization failed", err)
	}
	described, err = r.Describe(ctx, table("people"))
	if err != nil || described.Coverage.Indexes || described.Coverage.Unique {
		t.Fatal("partial index coverage incorrectly marked complete", err)
	}
}

func TestAdapterRejectsUnsafeOptions(t *testing.T) {
	opts := sqliteOptions(t)
	opts.MaxOpenConns = 0
	if _, err := OpenReader(context.Background(), opts); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("invalid pool accepted")
	}
	opts = sqliteOptions(t)
	opts.Schemas = []string{"other"}
	if _, err := OpenReader(context.Background(), opts); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("wrong schema accepted")
	}
	opts = sqliteOptions(t)
	if _, err := OpenReader(nil, opts); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatal("nil context accepted")
	}
	path := opts.DSN.Payload() + "?secret=synthetic-password"
	opts.DSN = testValue(t, contracts.Text, path)
	if _, err := OpenReader(context.Background(), opts); err == nil || strings.Contains(err.Error(), "synthetic-password") {
		t.Fatal("connection error leaked DSN")
	}
}
