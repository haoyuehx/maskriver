package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-sql-driver/mysql"
	"github.com/haoyuehx/maskriver/pkg/contracts"
	_ "modernc.org/sqlite"
)

type adapter struct {
	db      *sql.DB
	dialect contracts.Dialect
	dataset string
	schema  string
	schemas map[string]bool
	mu      sync.Mutex
	closed  bool
}

type reader struct{ *adapter }
type writer struct {
	*adapter
	active *writeTx
}

var _ contracts.Reader = (*reader)(nil)
var _ contracts.Writer = (*writer)(nil)

// connectorFactory builds the driver connector for an already validated driver
// name and DSN. It is the single seam for connection creation: a nil factory
// takes the sql.Open path that every production caller uses.
type connectorFactory func(driverName, dsn string) (driver.Connector, error)

// ErrHotJournal reports a rollback journal that is still present. A read-only
// SQLite connection cannot safely recover it; an explicit apply/resume must
// open the target read-write once to let SQLite roll it back.
var ErrHotJournal = errors.New("maskriver: hot rollback journal present")

// RecoverSQLiteJournal opens the target read-write once so SQLite can complete
// rollback-journal recovery. It is called only on explicit apply/resume paths;
// read-only preview/status must not write.
func RecoverSQLiteJournal(ctx context.Context, opts contracts.DatabaseOptions) error {
	if ctx == nil || opts.Dialect != contracts.SQLite {
		return contracts.ErrUnsupported
	}
	path := opts.DSN.Payload()
	if journalHot(path) {
		a, err := open(ctx, opts, false, nil)
		if err != nil {
			return err
		}
		defer a.Close()
		if _, err := a.db.ExecContext(ctx, "PRAGMA main.journal_mode"); err != nil {
			return dbError(err)
		}
	}
	return nil
}

// journalHot reports whether a non-empty rollback journal exists next to path.
func journalHot(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path + "-journal")
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func OpenReader(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Reader, error) {
	return openReader(ctx, opts, nil)
}

func OpenWriter(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
	return openWriter(ctx, opts, nil)
}

func openReader(ctx context.Context, opts contracts.DatabaseOptions, connect connectorFactory) (contracts.Reader, error) {
	a, err := open(ctx, opts, true, connect)
	if err != nil {
		return nil, err
	}
	return &reader{a}, nil
}

func openWriter(ctx context.Context, opts contracts.DatabaseOptions, connect connectorFactory) (contracts.Writer, error) {
	a, err := open(ctx, opts, false, connect)
	if err != nil {
		return nil, err
	}
	return &writer{adapter: a}, nil
}

func open(ctx context.Context, opts contracts.DatabaseOptions, readOnly bool, connect connectorFactory) (*adapter, error) {
	if ctx == nil || opts.DSN.Kind() != contracts.Text || opts.DSN.Payload() == "" ||
		opts.DatasetID == "" || opts.MaxOpenConns < 1 || opts.ConnectTimeout <= 0 ||
		len(opts.Schemas) == 0 {
		return nil, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	schemas := make(map[string]bool, len(opts.Schemas))
	for _, s := range opts.Schemas {
		if s == "" || schemas[s] || strings.ContainsRune(s, 0) {
			return nil, contracts.ErrInvalid
		}
		schemas[s] = true
	}
	var driverName, dsn string
	switch opts.Dialect {
	case contracts.SQLite:
		if len(schemas) != 1 || !schemas["main"] {
			return nil, contracts.ErrInvalid
		}
		var err error
		dsn, err = sqliteDSN(opts.DSN.Payload(), readOnly)
		if err != nil {
			return nil, err
		}
		driverName = "sqlite"
	case contracts.MySQL:
		cfg, err := mysql.ParseDSN(opts.DSN.Payload())
		if err != nil {
			return nil, contracts.ErrInvalid
		}
		if cfg.DBName == "" || len(schemas) != 1 {
			return nil, contracts.ErrInvalid
		}
		if cfg.MultiStatements || cfg.AllowAllFiles || cfg.AllowCleartextPasswords {
			return nil, contracts.ErrUnsafe
		}
		cfg.ParseTime = false
		if readOnly {
			// The driver applies arbitrary DSN system-variable parameters
			// to every new pooled connection. Force read-only transactions
			// on reader sessions, not just on the initial ping connection.
			// Database credentials should additionally have SELECT-only grants.
			if cfg.Params == nil {
				cfg.Params = make(map[string]string)
			}
			cfg.Params["transaction_read_only"] = "1"
		}
		dsn = cfg.FormatDSN()
		driverName = "mysql"
	default:
		return nil, contracts.ErrUnsupported
	}
	var db *sql.DB
	if connect == nil {
		var err error
		db, err = sql.Open(driverName, dsn)
		if err != nil {
			return nil, contracts.ErrDatabase
		}
	} else {
		connector, err := connect(driverName, dsn)
		if err != nil || connector == nil {
			return nil, contracts.ErrDatabase
		}
		db = sql.OpenDB(connector)
	}
	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxOpenConns)
	pingCtx, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, dbError(err)
	}
	physical := "main"
	if opts.Dialect == contracts.MySQL {
		cfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			_ = db.Close()
			return nil, contracts.ErrInvalid
		}
		physical = cfg.DBName
	}
	return &adapter{db: db, dialect: opts.Dialect, dataset: opts.DatasetID,
		schema: physical, schemas: schemas}, nil
}

func sqliteDSN(input string, readOnly bool) (string, error) {
	var u *url.URL
	var filePath string
	var err error
	if strings.HasPrefix(input, "file:") {
		u, err = url.Parse(input)
		if err != nil || u.Scheme != "file" || u.Host != "" || u.Fragment != "" || u.Opaque != "" {
			return "", contracts.ErrInvalid
		}
		filePath = filepath.FromSlash(u.Path)
		if strings.HasPrefix(u.Path, "/") {
			candidate := filepath.FromSlash(strings.TrimPrefix(u.Path, "/"))
			if filepath.VolumeName(candidate) != "" {
				filePath = candidate
			}
		}
	} else {
		if !filepath.IsAbs(input) {
			return "", contracts.ErrInvalid
		}
		filePath = input
		uriPath := filepath.ToSlash(input)
		if filepath.VolumeName(input) != "" && !strings.HasPrefix(uriPath, "/") {
			uriPath = "/" + uriPath
		}
		u = &url.URL{Scheme: "file", Path: uriPath}
	}
	if filePath == "" || !filepath.IsAbs(filePath) {
		return "", contracts.ErrInvalid
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", contracts.ErrNotFound
		}
		return "", contracts.ErrDatabase
	}
	if !info.Mode().IsRegular() {
		return "", contracts.ErrInvalid
	}
	q := u.Query()
	if q.Has("vfs") || q.Has("immutable") || q.Has("cache") {
		return "", contracts.ErrUnsafe
	}
	if readOnly {
		q.Set("mode", "ro")
	} else {
		q.Set("mode", "rw")
	}
	q.Set("_pragma", "foreign_keys(1)")
	// Rollback-journal databases can briefly hold a reserved/exclusive lock while
	// a writer commits. Without a busy timeout a concurrent read-only status query
	// would make that commit fail as SQLITE_BUSY (reported as CommitUnknown). The
	// bounded wait keeps the single-writer model but does not turn a transient
	// lock into an ambiguous commit.
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (a *adapter) check(ctx context.Context) error {
	if ctx == nil {
		return contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	closed := a.closed
	a.mu.Unlock()
	if closed {
		return contracts.ErrClosed
	}
	return nil
}

func (a *adapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()
	if err := a.db.Close(); err != nil {
		return contracts.ErrDatabase
	}
	return nil
}

func (w *writer) Close() error {
	w.mu.Lock()
	tx := w.active
	w.mu.Unlock()
	if tx != nil {
		_ = tx.Rollback(context.Background())
	}
	return w.adapter.Close()
}

func (a *adapter) table(ref contracts.TableRef) error {
	if ref.Database != a.dataset || !a.schemas[ref.Schema] || ref.Table == "" ||
		strings.ContainsRune(ref.Table, 0) {
		return contracts.ErrInvalid
	}
	return nil
}

func (a *adapter) quote(s string) string {
	if a.dialect == contracts.MySQL {
		return "`" + strings.ReplaceAll(s, "`", "``") + "`"
	}
	return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
}

func (a *adapter) qualified(ref contracts.TableRef) string {
	return a.quote(a.schema) + "." + a.quote(ref.Table)
}

func dbError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ErrNotFound
	}
	return contracts.ErrDatabase
}
