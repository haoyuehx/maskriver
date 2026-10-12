package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/mask"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// TestCNDateSQLiteRealIntegration exercises the actual SQLite driver, read-only
// snapshot, strategy, optimistic UPDATE, commit and independent post-read.
// It never opens a production database or persists a masking key.
func TestCNDateSQLiteRealIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	targetOpts := sqliteOptions(t) // disposable seeded SQLite, owned by this test
	targetPath := targetOpts.DSN.Payload()
	sourcePath := filepath.Join(t.TempDir(), "synthetic-source.db")
	before, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal("snapshot read failed")
	}
	if err := os.WriteFile(sourcePath, before, 0600); err != nil {
		t.Fatal("snapshot copy failed")
	}
	sourceOpts := targetOpts
	sourceOpts.DSN = testValue(t, contracts.Text, sourcePath)
	source, err := OpenReader(ctx, sourceOpts)
	if err != nil {
		t.Fatal("snapshot reader failed", err)
	}
	defer source.Close()
	target, err := OpenReader(ctx, targetOpts)
	if err != nil {
		t.Fatal("target reader failed", err)
	}
	defer target.Close()

	ref := table("people")
	schema, err := target.Describe(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	var born contracts.Column
	for _, col := range schema.Columns {
		if col.Ref.Column == "born" {
			born = col
		}
	}
	if born.Ref.Column == "" || born.Type.Kind != contracts.Date {
		t.Fatal("expected typed date column")
	}

	// The dry-run phase reads the real driver but opens no Writer.
	page, err := target.Page(ctx, contracts.PageRequest{
		Table: ref, Columns: []string{"id", "born"}, KeyColumns: []string{"id"}, Limit: 1,
	})
	if err != nil || len(page.Rows) != 1 {
		t.Fatal("bounded preview read failed", err)
	}
	src, err := source.Page(ctx, contracts.PageRequest{
		Table: ref, Columns: []string{"id", "born"}, KeyColumns: []string{"id"}, Limit: 1,
	})
	if err != nil || len(src.Rows) != 1 || src.Rows[0]["born"] != page.Rows[0]["born"] {
		t.Fatal("source snapshot mismatch", err)
	}
	strategyRef := contracts.StrategyRef{ID: "cn_date", Version: "1"}
	strategy, err := mask.NewStrategy(strategyRef)
	if err != nil {
		t.Fatal(err)
	}
	key := testValue(t, contracts.Bytes, "synthetic-test-key-material-32bytes-only")
	column := contracts.ColumnPlan{
		Column: born.Ref, Type: born.Type, Strategy: strategyRef,
		Scope: "synthetic/integration", NormalizationVersion: "cn-v1", KeyID: "synthetic-key",
	}
	changed, err := strategy.Mask(ctx, page.Rows[0]["born"], contracts.MaskContext{Plan: column, Key: key})
	if err != nil || changed == page.Rows[0]["born"] || changed.Kind() != contracts.Date {
		t.Fatal("strategy output invalid", err)
	}
	dry, err := os.ReadFile(targetPath)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(dry) {
		t.Fatal("read-only preview modified persistent SQLite snapshot", err)
	}

	writer, err := OpenWriter(ctx, targetOpts)
	if err != nil {
		t.Fatal("writer open failed", err)
	}
	defer writer.Close()
	tx, err := writer.Begin(ctx, contracts.TablePlan{
		Table: ref, PrimaryKey: []string{"id"}, Columns: []contracts.ColumnPlan{column},
	})
	if err != nil {
		t.Fatal("write transaction failed", err)
	}
	original := page.Rows[0]["born"]
	count, err := tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{page.Rows[0]["id"]},
		Expected:    contracts.Row{"born": original},
		Replacement: contracts.Row{"born": changed},
	}})
	if err != nil || count != 1 {
		t.Fatal("optimistic update failed", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("commit failed", err)
	}

	// Validate both exact replacement and unchanged source independently.
	readback, err := OpenReader(ctx, targetOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer readback.Close()
	result, err := readback.Page(ctx, contracts.PageRequest{
		Table: ref, Columns: []string{"id", "born"}, KeyColumns: []string{"id"}, Limit: 1,
	})
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["born"] != changed {
		t.Fatal("post-commit mismatch", err)
	}
	if _, err := time.Parse(time.DateOnly, result.Rows[0]["born"].Payload()); err != nil {
		t.Fatal("date lost SQL validity")
	}
	originalPage, err := source.Page(ctx, contracts.PageRequest{
		Table: ref, Columns: []string{"id", "born"}, KeyColumns: []string{"id"}, Limit: 1,
	})
	if err != nil || originalPage.Rows[0]["born"] != original {
		t.Fatal("source snapshot mutated", err)
	}
	// Query through raw SQLite to confirm the change persisted on disk.
	raw, err := sql.Open("sqlite", "file:"+targetPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var persisted string
	if err := raw.QueryRowContext(ctx, "SELECT born FROM people WHERE id=1").Scan(&persisted); err != nil || persisted != changed.Payload() {
		t.Fatal("durable SQLite value mismatch", err)
	}
}
