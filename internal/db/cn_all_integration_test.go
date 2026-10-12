package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/mask"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// All test fixtures below are hand-constructed synthetic shapes, not harvested
// identities, contact details, registration codes or payment credentials.
// A format-valid generated identifier is not necessarily unassigned.
var cnIntegrationInputs = []struct {
	column   string
	strategy string
	value    string
}{
	{"resident", "cn_id_card", "110101200002290018"},
	{"mobile", "cn_mobile", "+8613912345678"},
	{"bank_card", "cn_bank_card", "6222021234567894"},
	{"credit_code", "cn_uscc", "91110000123456789Q"},
	{"postal", "cn_postal_code", "100000"},
	{"city", "cn_city", "北京"},
	{"person", "cn_person_name", "欧阳星河"},
	{"address", "cn_address", "北京市东城区和平路12号"},
	{"birth", "cn_date", "2024-02-29"},
}

// verifyCNAdapterIntegration tests actual SQL metadata, typed keyset Page,
// all nine format replacement strategies, a single optimistic transaction,
// durable readback and the unchanged primary key. It does not test the CLI,
// human approvals, or production uniqueness and FK reference integrity.
func verifyCNAdapterIntegration(t *testing.T, ctx context.Context, sqlDB *sql.DB, opts contracts.DatabaseOptions) {
	t.Helper()
	const ddl = `CREATE TABLE cn_synthetic (
		id INTEGER PRIMARY KEY,
		resident TEXT NOT NULL, mobile TEXT NOT NULL, bank_card TEXT NOT NULL,
		credit_code TEXT NOT NULL, postal TEXT NOT NULL, city TEXT NOT NULL,
		person TEXT NOT NULL, address TEXT NOT NULL, birth DATE NOT NULL
	)`
	if _, err := sqlDB.ExecContext(ctx, ddl); err != nil {
		t.Fatal("synthetic CN schema failed", err)
	}
	columns := []string{"id"}
	values := []any{1}
	for _, field := range cnIntegrationInputs {
		columns = append(columns, field.column)
		values = append(values, field.value)
	}
	const insert = `INSERT INTO cn_synthetic
		(id,resident,mobile,bank_card,credit_code,postal,city,person,address,birth)
		VALUES (?,?,?,?,?,?,?,?,?,?)`
	if _, err := sqlDB.ExecContext(ctx, insert, values...); err != nil {
		t.Fatal("synthetic CN fixture insert failed", err)
	}

	ref := contracts.TableRef{Database: opts.DatasetID, Schema: opts.Schemas[0], Table: "cn_synthetic"}
	reader, err := OpenReader(ctx, opts)
	if err != nil {
		t.Fatal("CN reader open failed", err)
	}
	defer reader.Close()
	schema, err := reader.Describe(ctx, ref)
	if err != nil {
		t.Fatal("CN metadata failed", err)
	}
	if len(schema.PrimaryKey) != 1 || schema.PrimaryKey[0] != "id" {
		t.Fatal("CN stable primary key missing")
	}
	page, err := reader.Page(ctx, contracts.PageRequest{
		Table: ref, Columns: columns, KeyColumns: []string{"id"}, Limit: 5,
	})
	if err != nil || len(page.Rows) != 1 || !page.Done {
		t.Fatal("CN typed keyset page failed", err)
	}
	columnTypes := make(map[string]contracts.Column, len(schema.Columns))
	for _, col := range schema.Columns {
		columnTypes[col.Ref.Column] = col
	}
	plans := make([]contracts.ColumnPlan, 0, len(cnIntegrationInputs))
	expected := make(contracts.Row, len(cnIntegrationInputs))
	replacement := make(contracts.Row, len(cnIntegrationInputs))
	key := testValue(t, contracts.Bytes, "synthetic-integration-key-material-32bytes")
	for _, field := range cnIntegrationInputs {
		original := page.Rows[0][field.column]
		if original.Payload() != field.value {
			t.Fatal("CN source fixture differs from typed adapter value")
		}
		ref := contracts.StrategyRef{ID: field.strategy, Version: "1"}
		strategy, err := mask.NewStrategy(ref)
		if err != nil {
			t.Fatal("CN strategy not registered", err)
		}
		col, exists := columnTypes[field.column]
		if !exists {
			t.Fatal("CN metadata column missing")
		}
		plan := contracts.ColumnPlan{
			Column: col.Ref, Type: col.Type, Strategy: ref,
			Scope: "synthetic/real-db", NormalizationVersion: "cn-v1",
			KeyID: "synthetic-key",
		}
		output, err := strategy.Mask(ctx, original, contracts.MaskContext{Plan: plan, Key: key})
		if err != nil || output.IsNull() || output.Kind() != original.Kind() || output == original {
			t.Fatal("CN format replacement failed on real adapter", err)
		}
		// Verify the output itself is accepted by the same format parser.
		if _, err := strategy.Mask(ctx, output, contracts.MaskContext{Plan: plan, Key: key}); err != nil {
			t.Fatal("CN output does not satisfy declared input format", err)
		}
		expected[field.column] = original
		replacement[field.column] = output
		plans = append(plans, plan)
	}

	writer, err := OpenWriter(ctx, opts)
	if err != nil {
		t.Fatal("CN writer open failed", err)
	}
	defer writer.Close()
	tx, err := writer.Begin(ctx, contracts.TablePlan{
		Table: ref, PrimaryKey: []string{"id"}, Columns: plans,
	})
	if err != nil {
		t.Fatal("CN write transaction begin failed", err)
	}
	changed, err := tx.Update(ctx, []contracts.RowChange{{
		Key:         []contracts.Value{page.Rows[0]["id"]},
		Expected:    expected,
		Replacement: replacement,
	}})
	if err != nil || changed != 1 {
		t.Fatal("CN optimistic update refused", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("CN real database commit failed", err)
	}

	after, err := OpenReader(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	readback, err := after.Page(ctx, contracts.PageRequest{
		Table: ref, Columns: columns, KeyColumns: []string{"id"}, Limit: 5,
	})
	if err != nil || len(readback.Rows) != 1 || readback.Rows[0]["id"] != page.Rows[0]["id"] {
		t.Fatal("CN key-aligned readback failed", err)
	}
	for _, field := range cnIntegrationInputs {
		if readback.Rows[0][field.column] != replacement[field.column] {
			t.Fatal("CN durable replacement differs from preview")
		}
	}
	n, err := after.Count(ctx, ref)
	if err != nil || n != 1 {
		t.Fatal("CN row count changed", err)
	}
}

func TestCNAllFormatsSQLiteIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	opts := sqliteOptions(t)
	dsn := "file:" + opts.DSN.Payload() + "?mode=rw&_pragma=foreign_keys(1)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	verifyCNAdapterIntegration(t, ctx, database, opts)
}
