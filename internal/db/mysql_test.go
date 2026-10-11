package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/haoyuehx/maskriver/internal/testenv"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func TestMySQLAdapter(t *testing.T) {
	socket := os.Getenv("MASKRIVER_TEST_MYSQL_SOCKET")
	if socket == "" {
		t.Skip("isolated MySQL socket not provided; adapter UNVERIFIED")
	}
	abs, err := filepath.Abs(socket)
	if err != nil {
		t.Fatal("invalid socket")
	}
	dir := filepath.Dir(abs)
	marker, err := os.ReadFile(filepath.Join(dir, "maskriver-fixture-only"))
	info, statErr := os.Lstat(dir)
	if err != nil || statErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 ||
		string(marker) != "synthetic-only\n" || !strings.HasPrefix(filepath.Base(dir), "mysql-m1a-") {
		t.Fatal("refusing non-harness MySQL instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User = "root"
	cfg.Net = "unix"
	cfg.Addr = abs
	cfg.Timeout = 3 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("admin open failed")
	}
	defer admin.Close()
	if err = admin.PingContext(ctx); err != nil {
		t.Fatal("isolated MySQL ping failed")
	}
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE maskriver_db_adapter CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); err != nil {
		t.Fatal("isolated adapter database create failed")
	}
	cfg.DBName = "maskriver_db_adapter"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("adapter fixture open failed")
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, "SET SESSION time_zone = '+00:00'"); err != nil {
		t.Fatal(err)
	}
	if err = testenv.Seed(ctx, db, "mysql"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "CREATE TABLE unsigneds (id BIGINT UNSIGNED PRIMARY KEY, n BIGINT UNSIGNED NOT NULL) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO unsigneds(id,n) VALUES(?,?)", "18446744073709551615", "18446744073709551615"); err != nil {
		t.Fatal(err)
	}

	opts := contracts.DatabaseOptions{
		Dialect: contracts.MySQL, DSN: testValue(t, contracts.Text, cfg.FormatDSN()),
		DatasetID: "fixture", Schemas: []string{"maskriver_db_adapter"},
		MaxOpenConns: 1, ConnectTimeout: 5 * time.Second,
	}
	opened, err := OpenReader(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	r := opened.(*reader)
	people := contracts.TableRef{Database: "fixture", Schema: "maskriver_db_adapter", Table: "people"}
	schema, err := r.Describe(ctx, people)
	if err != nil {
		t.Fatal(err)
	}
	if !schema.Coverage.Columns || !schema.Coverage.PrimaryKey || !schema.Coverage.Indexes || !schema.Coverage.ForeignKeys || !schema.Coverage.Unique {
		t.Fatal("MySQL schema coverage missing")
	}
	n, err := r.Count(ctx, people)
	if err != nil || n != 25 {
		t.Fatal("MySQL count failed", err)
	}
	after := []contracts.Value(nil)
	seen := 0
	for {
		page, pageErr := r.Page(ctx, contracts.PageRequest{
			Table: people, Columns: []string{"id", "amount", "born", "payload"},
			KeyColumns: []string{"id"}, After: after, Limit: 10,
		})
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		for _, row := range page.Rows {
			if row["amount"].Kind() != contracts.Decimal ||
				row["amount"].Payload() != "12345678901234567890.1200" ||
				row["born"].Kind() != contracts.Date ||
				row["born"].Payload() != "2000-02-29" ||
				row["payload"].Kind() != contracts.Bytes {
				t.Fatal("MySQL typed roundtrip failed")
			}
		}
		seen += len(page.Rows)
		if page.Done {
			break
		}
		after = page.Next
	}
	if seen != 25 {
		t.Fatal("MySQL keyset lost rows")
	}
	unsigned := contracts.TableRef{Database: "fixture", Schema: "maskriver_db_adapter", Table: "unsigneds"}
	page, err := r.Page(ctx, contracts.PageRequest{
		Table: unsigned, Columns: []string{"id", "n"}, KeyColumns: []string{"id"}, Limit: 10,
	})
	if err != nil || len(page.Rows) != 1 || page.Rows[0]["n"].Kind() != contracts.Uint ||
		page.Rows[0]["n"].Payload() != "18446744073709551615" {
		t.Fatal("MySQL unsigned roundtrip failed", err)
	}

	wo, err := OpenWriter(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer wo.Close()
	w := wo.(*writer)
	var nick contracts.Column
	for _, c := range schema.Columns {
		if c.Ref.Column == "nickname" {
			nick = c
		}
	}
	plan := contracts.TablePlan{Table: people, PrimaryKey: []string{"id"},
		Columns: []contracts.ColumnPlan{{Column: nick.Ref, Type: nick.Type}},
	}
	tx, err := w.Begin(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	change := contracts.RowChange{
		Key:         []contracts.Value{testValue(t, contracts.Int, "1")},
		Expected:    contracts.Row{"nickname": {}},
		Replacement: contracts.Row{"nickname": testValue(t, contracts.Text, "mysql-synthetic")},
	}
	if _, err = tx.Update(ctx, []contracts.RowChange{change}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = w.Begin(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Update(ctx, []contracts.RowChange{change}); !errors.Is(err, contracts.ErrConflict) {
		t.Fatal("MySQL optimistic conflict not detected", err)
	}
}
