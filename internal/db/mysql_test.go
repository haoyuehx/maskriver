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
	"github.com/haoyuehx/maskriver/internal/mask"
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
	// Reader sessions must be database-enforced read-only, even when
	// the disposable harness connects with an otherwise writable account.
	if _, err := r.db.ExecContext(ctx, "UPDATE people SET nickname='should-never-write' WHERE id=1"); err == nil {
		t.Fatal("MySQL Reader allowed a direct UPDATE")
	}
	var stillNull sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT nickname FROM people WHERE id=1").Scan(&stillNull); err != nil || stillNull.Valid {
		t.Fatal("MySQL reader modified a persistent value")
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
	// Row 1 has a NULL nickname and row 2 an empty one. A NULL original may only
	// stay NULL, so the optimistic update has to target row 2.
	change := contracts.RowChange{
		Key:         []contracts.Value{testValue(t, contracts.Int, "2")},
		Expected:    contracts.Row{"nickname": testValue(t, contracts.Text, "")},
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

	// Exercise the actual CN date strategy through MySQL typed Page,
	// optimistic WriteTx and independent durable readback. This test is
	// reached only when a harness-owned private Unix socket is provided.
	var born contracts.Column
	for _, c := range schema.Columns {
		if c.Ref.Column == "born" {
			born = c
		}
	}
	if born.Ref.Column == "" || born.Type.Kind != contracts.Date {
		t.Fatal("MySQL DATE metadata unavailable")
	}
	before, err := r.Page(ctx, contracts.PageRequest{
		Table: people, Columns: []string{"id", "born"}, KeyColumns: []string{"id"}, Limit: 1,
	})
	if err != nil || len(before.Rows) != 1 {
		t.Fatal("MySQL CN input read failed", err)
	}
	ref := contracts.StrategyRef{ID: "cn_date", Version: "1"}
	strategy, err := mask.NewStrategy(ref)
	if err != nil {
		t.Fatal(err)
	}
	col := contracts.ColumnPlan{
		Column: born.Ref, Type: born.Type, Strategy: ref,
		Scope: "synthetic/mysql", NormalizationVersion: "cn-v1", KeyID: "synthetic-key",
	}
	masked, err := strategy.Mask(ctx, before.Rows[0]["born"], contracts.MaskContext{
		Plan: col, Key: testValue(t, contracts.Bytes, "synthetic-test-key-material-32bytes-only"),
	})
	if err != nil || masked == before.Rows[0]["born"] || masked.Kind() != contracts.Date {
		t.Fatal("MySQL CN date strategy failed", err)
	}
	tx, err = w.Begin(ctx, contracts.TablePlan{
		Table: people, PrimaryKey: []string{"id"}, Columns: []contracts.ColumnPlan{col},
	})
	if err != nil {
		t.Fatal("MySQL CN write begin failed", err)
	}
	count, err := tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{before.Rows[0]["id"]},
		Expected:    contracts.Row{"born": before.Rows[0]["born"]},
		Replacement: contracts.Row{"born": masked},
	}})
	if err != nil || count != 1 {
		t.Fatal("MySQL CN update failed", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("MySQL CN commit failed", err)
	}
	var persisted string
	if err := db.QueryRowContext(ctx, "SELECT born FROM people WHERE id=1").Scan(&persisted); err != nil ||
		persisted != masked.Payload() {
		t.Fatal("MySQL CN durable readback failed", err)
	}
	verifyCNAdapterIntegration(t, ctx, db, opts)

	// Strict integrity must see logical FK damage, not just CHECK TABLE rows.
	if err := r.IntegrityCheck(ctx, people); err != nil {
		t.Fatal("clean MySQL strict integrity failed", err)
	}
	if _, err = db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO links(tenant,seq,person_id) VALUES(999,999,9999)"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=1"); err != nil {
		t.Fatal(err)
	}
	if err = r.IntegrityCheck(ctx, people); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("MySQL strict integrity missed orphan FK: %v", err)
	}
	if _, err = db.ExecContext(ctx, "DELETE FROM links WHERE tenant=999 AND seq=999"); err != nil {
		t.Fatal(err)
	}
}
