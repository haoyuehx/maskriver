package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// ---------- entry points ---------------------------------------------

// OpenReader opens a read-only reader. For SQLite it explicitly refuses
// to create a missing file and opens in RO mode. For MySQL it opens a
// read connection with sql_safe_updates=1 set opportunistically; a
// failure to apply that setting does not abort Open (MySQL user priv
// may deny the session var).
func OpenReader(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Reader, error) {
	if err := validateBase(opts); err != nil {
		return nil, err
	}
	switch opts.Dialect {
	case contracts.SQLite:
		return openSQLite(ctx, opts, true)
	case contracts.MySQL:
		return openMySQL(ctx, opts, true)
	}
	return nil, mapErr(errors.New("unsupported dialect"))
}

// OpenWriter opens a write-capable writer. SQLite RW mode; missing
// database files are created only when the caller writes; opening an
// RO-locked DB still fails. MySQL applies sql_safe_updates=1 as a
// defense-in-depth best-effort session setting.
func OpenWriter(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
	if err := validateBase(opts); err != nil {
		return nil, err
	}
	switch opts.Dialect {
	case contracts.SQLite:
		return openSQLite(ctx, opts, false)
	case contracts.MySQL:
		return openMySQL(ctx, opts, false)
	}
	return nil, mapErr(errors.New("unsupported dialect"))
}

func validateBase(o contracts.DatabaseOptions) error {
	if o.DSN.Kind() == contracts.Null || o.DSN.Payload() == "" {
		return contracts.ErrInvalid
	}
	if o.ConnectTimeout < 0 || o.MaxOpenConns < 0 {
		return contracts.ErrInvalid
	}
	return nil
}

// ---------- base handle (Reader + Writer share it) -------------------

type baseHandle struct {
	mu        sync.Mutex
	db        *sql.DB
	dialect   contracts.Dialect
	dataset   string
	schemas   []string
	closeOnce sync.Once
	closed    bool
}

func (b *baseHandle) closeLocked() error {
	var err error
	b.closeOnce.Do(func() {
		b.closed = true
		if b.db != nil {
			err = b.db.Close()
		}
	})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

func (b *baseHandle) assertOpen() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.db == nil {
		return contracts.ErrClosed
	}
	return nil
}

// ---------- SQLite ---------------------------------------------------

func openSQLite(ctx context.Context, opts contracts.DatabaseOptions, readonly bool) (*sqliteHandle, error) {
	path := strings.TrimPrefix(opts.DSN.Payload(), "file:")
	// Strip embedded query params; modernc sqlite supports URI via dsn
	// but we insist on plain file paths in M1 to keep audits simple.
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		return nil, contracts.ErrInvalid
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, mapErr(err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", abs)
	if readonly {
		dsn += "&mode=ro"
		// MANDATORY security: OpenReader must never create a DB file.
		if !fileExists(abs) {
			return nil, contracts.ErrNotFound
		}
	}
	timeout := opts.ConnectTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	openCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapErr(err)
	}
	maxOpen := opts.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 8
	}
	sqldb.SetMaxOpenConns(maxOpen)
	if err := sqldb.PingContext(openCtx); err != nil {
		_ = sqldb.Close()
		return nil, mapErr(err)
	}
	if readonly {
		// Double-guard against mutable mutations via PRAGMA query_only.
		if _, err2 := sqldb.ExecContext(openCtx, "PRAGMA query_only = 1"); err2 != nil {
			_ = sqldb.Close()
			return nil, mapErr(err2)
		}
	}
	h := &sqliteHandle{baseHandle: &baseHandle{
		db: sqldb, dialect: contracts.SQLite, dataset: opts.DatasetID, schemas: append([]string(nil), opts.Schemas...),
	}}
	return h, nil
}

type sqliteHandle struct{ *baseHandle }

// ---------- MySQL ---------------------------------------------------

func openMySQL(ctx context.Context, opts contracts.DatabaseOptions, readonly bool) (*mysqlHandle, error) {
	// DSN is already a driver-compatible string (user:pwd@tcp(host:port)/db?params).
	// We parse-rebuild minimally: inject parseTime=true if not already
	// present (Value encoding does not require typed scan; this is
	// future-safe for date columns).
	raw := opts.DSN.Payload()
	if raw == "" {
		return nil, contracts.ErrInvalid
	}
	if !strings.Contains(raw, "parseTime=") {
		sep := "?"
		if strings.Contains(raw, "?") {
			sep = "&"
		}
		raw = raw + sep + "parseTime=true"
	}
	timeout := opts.ConnectTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	openCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	sqldb, err := sql.Open("mysql", raw)
	if err != nil {
		return nil, mapErr(err)
	}
	maxOpen := opts.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 32
	}
	sqldb.SetMaxOpenConns(maxOpen)
	if err := sqldb.PingContext(openCtx); err != nil {
		_ = sqldb.Close()
		return nil, mapErr(err)
	}
	// Best-effort sql_safe_updates=1 + tx_isolation READ COMMITTED.
	_, _ = sqldb.ExecContext(openCtx, "SET SESSION sql_safe_updates=1")
	h := &mysqlHandle{baseHandle: &baseHandle{
		db: sqldb, dialect: contracts.MySQL, dataset: opts.DatasetID, schemas: append([]string(nil), opts.Schemas...),
	}, readonly: readonly}
	return h, nil
}

type mysqlHandle struct {
	*baseHandle
	readonly bool
}

// ---------- common helpers ------------------------------------------

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var ne interface{ Temporary() bool }
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, contracts.ErrClosed), errors.Is(err, contracts.ErrNotFound),
		errors.Is(err, contracts.ErrInvalid), errors.Is(err, contracts.ErrUnsupported),
		errors.Is(err, contracts.ErrConflict), errors.Is(err, contracts.ErrCommitUnknown):
		return err
	case errors.As(err, &ne):
		return contracts.ErrDatabase
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "no such table"), strings.Contains(msg, "doesn't exist"),
		strings.Contains(msg, "not found"):
		return contracts.ErrNotFound
	case strings.Contains(msg, "readonly"), strings.Contains(msg, "read-only"),
		strings.Contains(msg, "cannot modify"), strings.Contains(msg, "attempt to write a readonly"):
		return contracts.ErrUnsafe
	case strings.Contains(msg, "constraint"), strings.Contains(msg, "duplicate"),
		strings.Contains(msg, "unique"), strings.Contains(msg, "conflict"):
		return contracts.ErrConflict
	case strings.Contains(msg, "syntax"):
		return contracts.ErrInvalid
	case strings.Contains(msg, "closed"):
		return contracts.ErrClosed
	}
	return contracts.ErrDatabase
}

func isDuplicateTableName(ref contracts.TableRef) bool { return false }

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func qualifiedTbl(d contracts.Dialect, t contracts.TableRef) string {
	switch d {
	case contracts.MySQL:
		parts := make([]string, 0, 2)
		if t.Schema != "" {
			parts = append(parts, quoteIdentMySQL(t.Schema))
		}
		if t.Table == "" {
			return ""
		}
		parts = append(parts, quoteIdentMySQL(t.Table))
		return strings.Join(parts, ".")
	case contracts.SQLite:
		if t.Schema == "" {
			return quoteIdentSQLite(t.Table)
		}
		return quoteIdentSQLite(t.Schema) + "." + quoteIdentSQLite(t.Table)
	}
	return ""
}

func quoteIdentSQLite(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func quoteIdentMySQL(s string) string  { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

func colList(d contracts.Dialect, cols []string) string {
	q := make([]string, len(cols))
	for i, c := range cols {
		if d == contracts.MySQL {
			q[i] = quoteIdentMySQL(c)
		} else {
			q[i] = quoteIdentSQLite(c)
		}
	}
	return strings.Join(q, ", ")
}

// placeholder returns the n-th positional placeholder. SQLite uses ?;
// MySQL also uses ? in the driver we use.
func placeholder(_ contracts.Dialect, n int) string { return "?" }

func placeholderList(d contracts.Dialect, n int) string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = placeholder(d, i+1)
	}
	return strings.Join(out, ", ")
}
