package testenv

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

func sqliteURL(path string, readOnly bool) string {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	if readOnly {
		q.Set("mode", "ro")
	} else {
		q.Set("mode", "rwc")
	}
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	return u.String()
}
func TestSQLiteFixture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	db, err := sql.Open("sqlite", sqliteURL(path, false))
	if err != nil {
		t.Fatal("sqlite open failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err = Seed(ctx, db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	checkFixture(t, ctx, db)
	ro, err := sql.Open("sqlite", sqliteURL(path, true))
	if err != nil {
		t.Fatal("readonly open failed")
	}
	defer ro.Close()
	if _, err = ro.ExecContext(ctx, `UPDATE people SET nickname='not allowed' WHERE id=1`); err == nil {
		t.Fatal("readonly write succeeded")
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	absent, err := sql.Open("sqlite", sqliteURL(missing, true))
	if err != nil {
		t.Fatal("open handle failed")
	}
	defer absent.Close()
	if err = absent.PingContext(ctx); err == nil {
		t.Fatal("readonly missing database opened")
	}
	if _, err = os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("readonly created a database")
	}
}

// MYSQL test is opt-in and ONLY accepts a harness-created private Unix socket.
// No TCP, arbitrary DSN, real credentials or system sockets are accepted.
func TestMySQLFixture(t *testing.T) {
	socket := os.Getenv("MASKRIVER_TEST_MYSQL_SOCKET")
	if socket == "" {
		t.Skip("isolated MySQL not started; run python3 scripts/test_mysql.py")
	}
	socket, err := filepath.Abs(socket)
	if err != nil {
		t.Fatal("invalid socket path")
	}
	dir := filepath.Dir(socket)
	marker, err := os.ReadFile(filepath.Join(dir, "maskriver-fixture-only"))
	info, statErr := os.Lstat(dir)
	if err != nil || string(marker) != "synthetic-only\n" || statErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 || !strings.HasPrefix(filepath.Base(dir), "mysql-m1a-") {
		t.Fatal("refusing non-harness database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User = "root"
	cfg.Net = "unix"
	cfg.Addr = socket
	cfg.Timeout = 3 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal("connector config failed")
	}
	admin := sql.OpenDB(connector)
	defer admin.Close()
	if err = admin.PingContext(ctx); err != nil {
		t.Fatal("isolated mysql ping failed")
	}
	// The server is dedicated to this run; no IF NOT EXISTS avoids accidental reuse.
	if _, err = admin.ExecContext(ctx, `CREATE DATABASE maskriver_fixture CHARACTER SET utf8mb4 COLLATE utf8mb4_bin`); err != nil {
		t.Fatal("isolated database create failed")
	}
	cfg.DBName = "maskriver_fixture"
	connector, err = mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal("fixture connector failed")
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, `SET SESSION time_zone = '+00:00'`); err != nil {
		t.Fatal("timezone setup failed")
	}
	if err = Seed(ctx, db, "mysql"); err != nil {
		t.Fatal(err)
	}
	checkFixture(t, ctx, db)
	var version string
	if err = db.QueryRowContext(ctx, `SELECT VERSION()`).Scan(&version); err != nil {
		t.Fatal("version query failed")
	}
	t.Logf("isolated MySQL server %s", version)
}
func checkFixture(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM people`).Scan(&count); err != nil || count != Rows {
		t.Fatal("fixture count mismatch")
	}
	var null sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT nickname FROM people WHERE id=1`).Scan(&null); err != nil || null.Valid {
		t.Fatal("NULL lost")
	}
	if err := db.QueryRowContext(ctx, `SELECT nickname FROM people WHERE id=2`).Scan(&null); err != nil || !null.Valid || null.String != "" {
		t.Fatal("empty text lost")
	}
	var amount string
	var blob []byte
	if err := db.QueryRowContext(ctx, `SELECT amount,payload FROM people WHERE id=3`).Scan(&amount, &blob); err != nil || amount != "12345678901234567890.1200" || string(blob) != string([]byte{0, 1, 255}) {
		t.Fatal("decimal/blob changed")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO links(tenant,seq,person_id) VALUES(99,1,999)`); err == nil {
		t.Fatal("FK enforcement missing")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("begin failed")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE people SET nickname='rollback-only' WHERE id=1`); err != nil {
		_ = tx.Rollback()
		t.Fatal("update failed")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal("rollback failed")
	}
	if err = db.QueryRowContext(ctx, `SELECT nickname FROM people WHERE id=1`).Scan(&null); err != nil || null.Valid {
		t.Fatal("rollback failed to restore")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if err = db.PingContext(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not propagated")
	}
}
