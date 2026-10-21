package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// boundedReaderFor creates a synthetic SQLite file with the given DDL and
// statements, then opens the real read adapter on it.
func boundedReaderFor(t *testing.T, ddl []string, statements ...string) (context.Context, *reader, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-bounded.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=rwc&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range ddl {
		if _, err := raw.Exec(stmt); err != nil {
			_ = raw.Close()
			t.Fatal("synthetic DDL failed")
		}
	}
	for _, stmt := range statements {
		if _, err := raw.Exec(stmt); err != nil {
			_ = raw.Close()
			t.Fatal("synthetic insert failed")
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	opts := contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: testValue(t, contracts.Text, path),
		DatasetID: "fixture", Schemas: []string{"main"}, MaxOpenConns: 2,
		ConnectTimeout: 5 * time.Second,
	}
	ctx := context.Background()
	opened, err := OpenReader(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, opened.(*reader), func() { _ = opened.Close() }
}

func pageReq(limit int, after []contracts.Value, cols ...string) contracts.PageRequest {
	return contracts.PageRequest{
		Table: table("t"), Columns: cols, KeyColumns: []string{"id"},
		After: after, Limit: limit,
	}
}

func TestPageWithinExactBoundaryAndLegacyPage(t *testing.T) {
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, body TEXT NOT NULL)`,
	}, `INSERT INTO t(id,body) VALUES(1,'0123456789abcdef')`,
		`INSERT INTO t(id,body) VALUES(2,'0123456789abcdefg')`)
	defer closeFn()

	// The projected row includes the primary key. Row 1 is 17 bytes and row 2 is
	// 18 bytes; a 17-byte budget passes row 1 but must reject the 18-byte
	// lookahead before returning anything.
	if _, err := r.PageWithin(ctx, pageReq(1, nil, "id", "body"), 17); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("lookahead over budget: got %v", err)
	}
	// With an 18-byte budget the first row is accepted and Done stays false
	// because the lookahead row exists.
	if page, err := r.PageWithin(ctx, pageReq(1, nil, "id", "body"), 18); err != nil || len(page.Rows) != 1 || page.Done {
		t.Fatalf("exact row rejected: %v %d %v", err, len(page.Rows), page.Done)
	}
	// Over by one byte on the second row fails closed.
	if _, err := r.PageWithin(ctx, pageReq(2, nil, "id", "body"), 17); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("over-by-one: got %v", err)
	}
	// The legacy reader is unaffected by the byte budget.
	page, err := r.Page(ctx, pageReq(10, nil, "id", "body"))
	if err != nil || len(page.Rows) != 2 || !page.Done {
		t.Fatalf("legacy page changed: %v %d", err, len(page.Rows))
	}
}

func TestPageWithinMultiColumnAndUTF8(t *testing.T) {
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, a TEXT, b TEXT)`,
	}, `INSERT INTO t(id,a,b) VALUES(1,'12345678','12345678')`,
		`INSERT INTO t(id,a,b) VALUES(2,'héllo','ü')`)
	defer closeFn()

	// Each column fits its own budget, but the row total (id 1 + a 8 + b 8 = 17)
	// does not fit 16.
	if _, err := r.PageWithin(ctx, contracts.PageRequest{
		Table: table("t"), Columns: []string{"id", "a", "b"}, KeyColumns: []string{"id"}, Limit: 1,
	}, 16); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("multi-column total: got %v", err)
	}
	// UTF-8 bytes, not characters: id 1 + "héllo" (6) + "ü" (2) = 9.
	page, err := r.PageWithin(ctx, contracts.PageRequest{
		Table: table("t"), Columns: []string{"id", "a", "b"}, KeyColumns: []string{"id"}, After: []contracts.Value{testValue(t, contracts.Int, "1")}, Limit: 1,
	}, 9)
	if err != nil || len(page.Rows) != 1 || page.Rows[0]["a"].Payload() != "héllo" {
		t.Fatalf("utf8 row: %v %d", err, len(page.Rows))
	}
	if _, err := r.PageWithin(ctx, contracts.PageRequest{
		Table: table("t"), Columns: []string{"id", "a", "b"}, KeyColumns: []string{"id"}, After: []contracts.Value{testValue(t, contracts.Int, "1")}, Limit: 1,
	}, 8); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("utf8 over: got %v", err)
	}
}

func TestPageWithinBlobNULNullAndScalars(t *testing.T) {
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, b BLOB, s TEXT, n INTEGER, f REAL, flag BOOLEAN, d DATE)`,
	}, `INSERT INTO t(id,b,s,n,f,flag,d) VALUES(1,x'00010203','a'||char(0)||'b',-9223372036854775808,1.7976931348623157e308,1,'2026-10-20')`,
		`INSERT INTO t(id,b,s,n,f,flag,d) VALUES(2,NULL,NULL,NULL,NULL,NULL,NULL)`)
	defer closeFn()
	cols := []string{"id", "b", "s", "n", "f", "flag", "d"}
	// id 1, blob 4, text 3 bytes (embedded NUL), int 20, real 32, bool 5,
	// date 10 = 75. Allow exactly 75; row 2 (NULLs) costs its 1-byte id.
	if page, err := r.PageWithin(ctx, contracts.PageRequest{
		Table: table("t"), Columns: cols, KeyColumns: []string{"id"}, Limit: 2,
	}, 75); err != nil || len(page.Rows) != 2 || !page.Done {
		t.Fatalf("typed rows: %v %d", err, len(page.Rows))
	}
	if _, err := r.PageWithin(ctx, contracts.PageRequest{
		Table: table("t"), Columns: cols, KeyColumns: []string{"id"}, Limit: 1,
	}, 74); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("typed over: got %v", err)
	}
	if !isLinux() {
		return
	}
	if page, err := r.PageWithin(ctx, contracts.PageRequest{
		Table: table("t"), Columns: cols, KeyColumns: []string{"id"}, After: []contracts.Value{testValue(t, contracts.Int, "1")}, Limit: 1,
	}, 1); err != nil || len(page.Rows) != 1 || !page.Rows[0]["b"].IsNull() {
		t.Fatalf("null row: %v", err)
	}
}

func TestPageWithinLookaheadNeverMaterialised(t *testing.T) {
	// The lookahead row is huge; the page must fail before returning the two
	// accepted rows, and it must not return the lookahead values.
	big := strings.Repeat("x", 1<<20)
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, body TEXT)`,
	}, `INSERT INTO t(id,body) VALUES(1,'a')`,
		`INSERT INTO t(id,body) VALUES(2,'b')`)
	defer closeFn()
	// A separate big row is inserted through a second connection so this test
	// does not build a giant SQL literal.
	path := r.dbDSNPath(t)
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO t(id,body) VALUES(3,?)`, big); err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	_ = raw.Close()
	if _, err := r.PageWithin(ctx, pageReq(2, nil, "id", "body"), 64); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("lookahead materialisation: got %v", err)
	}
	// A larger budget admits the lookahead key but still returns only the first
	// two rows, without the giant value.
	if page, err := r.PageWithin(ctx, pageReq(2, nil, "id", "body"), (1<<20)+1); err != nil || len(page.Rows) != 2 || page.Done {
		t.Fatalf("first page: %v %d %v", err, len(page.Rows), page.Done)
	}
}

// dbDSNPath returns the file path of this reader's read-only DSN.
func (r *reader) dbDSNPath(t *testing.T) string {
	t.Helper()
	// The adapter keeps only the *sql.DB; the fixture path is recovered from a
	// PRAGMA that names the main database file.
	var path string
	if err := r.db.QueryRow("PRAGMA database_list").Scan(new(int), new(string), &path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPageWithinPreMaterialisationMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory experiment")
	}
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, body BLOB)`,
	})
	defer closeFn()
	path := r.dbDSNPath(t)
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 8<<20)
	if _, err := raw.Exec(`INSERT INTO t(id,body) VALUES(1,?)`, big); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	req := pageReq(1, nil, "id", "body")

	stats := func() (uint64, uint64) {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc, m.TotalAlloc
	}
	big = nil
	heap0, alloc0 := stats()
	if _, err := r.PageWithin(ctx, req, 1<<20); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("oversized bounded page: %v", err)
	}
	heap1, alloc1 := stats()
	boundedAlloc := alloc1 - alloc0
	if boundedAlloc > 2<<20 {
		t.Fatalf("bounded rejection allocated %d bytes", boundedAlloc)
	}
	_, alloc2 := stats()
	page, err := r.Page(ctx, req)
	heap3, alloc3 := stats()
	legacyAlloc := alloc3 - alloc2
	_ = heap3
	if err != nil || len(page.Rows) != 1 {
		t.Fatalf("legacy page: %v", err)
	}
	if legacyAlloc < 4<<20 {
		t.Fatalf("legacy page did not materialise the field: %d", legacyAlloc)
	}
	t.Logf("bounded_alloc=%d legacy_alloc=%d heap_before=%d heap_after_bounded=%d heap_after_legacy=%d",
		boundedAlloc, legacyAlloc, heap0, heap1, heap3)

}

func TestPageWithinContextCancel(t *testing.T) {
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, body TEXT)`,
	}, `INSERT INTO t(id,body) VALUES(1,'x')`)
	defer closeFn()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.PageWithin(cancelled, pageReq(1, nil, "id", "body"), 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestSampleRejectsOversizedValue(t *testing.T) {
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, body TEXT)`,
	}, `INSERT INTO t(id,body) VALUES(1,'small')`)
	defer closeFn()
	ref := contracts.ColumnRef{TableRef: table("t"), Column: "body"}
	sample, err := r.Sample(ctx, contracts.SampleRequest{Column: ref, Limit: 10})
	if err != nil || len(sample.Values) != 1 || sample.Values[0].Payload() != "small" {
		t.Fatalf("small sample: %v", err)
	}
	path := r.dbDSNPath(t)
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO t(id,body) VALUES(2,?)`, strings.Repeat("y", 2<<20)); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	if _, err := r.Sample(ctx, contracts.SampleRequest{Column: ref, Limit: 10}); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("oversized sample: %v", err)
	}
}

func TestPageWithinInvalidArguments(t *testing.T) {
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, body TEXT)`,
	}, `INSERT INTO t(id,body) VALUES(1,'x')`)
	defer closeFn()
	if _, err := r.PageWithin(ctx, pageReq(1, nil, "id", "body"), 0); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("zero budget: %v", err)
	}
	if _, err := r.PageWithin(nil, pageReq(1, nil, "id", "body"), 10); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("nil ctx: %v", err)
	}
}

func isLinux() bool { return os.PathSeparator == '/' && runtime.GOOS == "linux" }

func TestPageWithinLookaheadValueNotRetained(t *testing.T) {
	if testing.Short() {
		t.Skip("memory experiment")
	}
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id INTEGER PRIMARY KEY, body BLOB)`,
	}, `INSERT INTO t(id,body) VALUES(1,x'0102')`)
	defer closeFn()
	path := r.dbDSNPath(t)
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 8<<20)
	if _, err := raw.Exec(`INSERT INTO t(id,body) VALUES(2,?)`, big); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	big = nil
	stats := func() uint64 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.TotalAlloc
	}
	before := stats()
	page, err := r.PageWithin(ctx, pageReq(1, nil, "id", "body"), (8<<20)+4)
	delta := stats() - before
	if err != nil || len(page.Rows) != 1 || page.Done {
		t.Fatalf("lookahead page: %v %d %v", err, len(page.Rows), page.Done)
	}
	if delta > 2<<20 {
		t.Fatalf("lookahead value retained: %d bytes", delta)
	}
	if page.Rows[0]["id"].Payload() != "1" || page.Rows[0]["body"].Payload() != "\x01\x02" {
		t.Fatal("accepted prefix changed")
	}
	t.Logf("lookahead total_alloc_delta=%d", delta)
}

func TestPageWithinHugeTextPrimaryKey(t *testing.T) {
	ctx, r, closeFn := boundedReaderFor(t, []string{
		`CREATE TABLE t(id TEXT PRIMARY KEY NOT NULL, body TEXT)`,
	}, `INSERT INTO t(id,body) VALUES('k','v')`)
	defer closeFn()
	path := r.dbDSNPath(t)
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	hugeKey := strings.Repeat("k", 2<<20)
	if _, err := raw.Exec(`INSERT INTO t(id,body) VALUES(?, 'small')`, hugeKey); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	stats := func() uint64 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.TotalAlloc
	}
	before := stats()
	_, err = r.PageWithin(ctx, contracts.PageRequest{
		Table: table("t"), Columns: []string{"id", "body"}, KeyColumns: []string{"id"}, Limit: 2,
	}, 1<<20)
	delta := stats() - before
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("huge key: %v", err)
	}
	if delta > 2<<20 {
		t.Fatalf("huge key materialised in preflight: %d bytes", delta)
	}
}
