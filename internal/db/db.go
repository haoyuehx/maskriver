package db

import (
	"context"
	"database/sql"
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

func OpenReader(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Reader, error) {
	a, err := open(ctx, opts, true)
	if err != nil {
		return nil, err
	}
	return &reader{a}, nil
}

func OpenWriter(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
	a, err := open(ctx, opts, false)
	if err != nil {
		return nil, err
	}
	return &writer{adapter: a}, nil
}

func open(ctx context.Context, opts contracts.DatabaseOptions, readOnly bool) (*adapter, error) {
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
	var driver, dsn string
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
		driver = "sqlite"
	case contracts.MySQL:
		cfg, err := mysql.ParseDSN(opts.DSN.Payload())
		if err != nil {
			return nil, contracts.ErrInvalid
		}
		if cfg.DBName == "" || !schemas[cfg.DBName] || len(schemas) != 1 {
			return nil, contracts.ErrInvalid
		}
		if cfg.MultiStatements || cfg.AllowAllFiles || cfg.AllowCleartextPasswords {
			return nil, contracts.ErrUnsafe
		}
		cfg.ParseTime = false
		dsn = cfg.FormatDSN()
		driver = "mysql"
	default:
		return nil, contracts.ErrUnsupported
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, contracts.ErrDatabase
	}
	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxOpenConns)
	pingCtx, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, dbError(err)
	}
	return &adapter{db: db, dialect: opts.Dialect, dataset: opts.DatasetID, schemas: schemas}, nil
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
	return a.quote(ref.Schema) + "." + a.quote(ref.Table)
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
