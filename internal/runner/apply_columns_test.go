package runner

// Regression tests for work package B2a: Preview must describe every planned
// column of a row, including NULL originals, and the writer must accept that
// description and assign only the columns that change. Apply itself still
// refuses multi-scope plans (see TestMultiScopeApplyIsRefusedWithoutWriter), so
// the multi-column cases are exercised through the shared visit/WriteTx seam.
// Every fixture is synthetic and lives in t.TempDir().

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// plannedColumnsAllNull reports whether every planned column of a change is
// NULL, which is the one case where a row needs no write at all.
func plannedColumnsAllNull(change contracts.RowChange) bool {
	for _, value := range change.Expected {
		if !value.IsNull() {
			return false
		}
	}
	return true
}

// mixedNullFixture plans two columns of one table and stores one row per NULL
// shape: only email set, only backup_email set, and both NULL.
func mixedNullFixture(t *testing.T) config.Runtime {
	t.Helper()
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, backup_email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', NULL)`,
		`INSERT INTO people VALUES (2, NULL, 'b@example.invalid')`,
		`INSERT INTO people VALUES (3, NULL, NULL)`,
	)
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "backup_email"}}
	return cfg
}

// visitChanges collects the RowChanges one plan visit produces.
func visitChanges(t *testing.T, e *Engine, plan contracts.Plan) []contracts.RowChange {
	t.Helper()
	var changes []contracts.RowChange
	if _, err := e.visit(context.Background(), plan, 0, func(change contracts.RowChange) error {
		changes = append(changes, change)
		return nil
	}); err != nil {
		t.Fatalf("visit: %v", err)
	}
	return changes
}

// TestVisitCarriesEveryPlannedColumnIncludingNull is the direct regression for
// the interface conflict: a page visit carries every planned column in Expected
// and Replacement, and a NULL original stays NULL instead of being omitted.
func TestVisitCarriesEveryPlannedColumnIncludingNull(t *testing.T) {
	engine := scopeEngine(t, mixedNullFixture(t))
	plan := mustPlan(t, engine)
	if len(plan.Tables) != 1 || len(plan.Tables[0].Columns) != 2 {
		t.Fatalf("plan tables=%d columns=%d, want one table with two columns",
			len(plan.Tables), len(plan.Tables[0].Columns))
	}

	changes := visitChanges(t, engine, plan)
	if len(changes) != 3 {
		t.Fatalf("RowChanges=%d, want one per row (3)", len(changes))
	}
	for i, change := range changes {
		if len(change.Expected) != 2 || len(change.Replacement) != 2 {
			t.Fatalf("row %d Expected=%d Replacement=%d, want every planned column",
				i, len(change.Expected), len(change.Replacement))
		}
		if len(change.Key) != 1 {
			t.Fatalf("row %d key=%v, want the primary key", i, change.Key)
		}
		for name, expected := range change.Expected {
			replacement, ok := change.Replacement[name]
			if !ok {
				t.Fatalf("row %d has no replacement for %s", i, name)
			}
			if expected.IsNull() != replacement.IsNull() {
				t.Fatalf("row %d column %s expected null=%t replacement null=%t",
					i, name, expected.IsNull(), replacement.IsNull())
			}
			if !expected.IsNull() && replacement == expected {
				t.Fatalf("row %d column %s was not masked", i, name)
			}
		}
	}
	// Row 1 keeps a NULL backup_email, row 2 a NULL email, row 3 both.
	if !changes[0].Expected["backup_email"].IsNull() || !changes[0].Replacement["backup_email"].IsNull() {
		t.Fatal("row 1 fabricated a value for the NULL backup_email")
	}
	if !changes[1].Expected["email"].IsNull() || !changes[1].Replacement["email"].IsNull() {
		t.Fatal("row 2 fabricated a value for the NULL email")
	}
	if !plannedColumnsAllNull(changes[2]) {
		t.Fatalf("row 3 Expected=%v, want every planned column NULL", changes[2].Expected)
	}
}

// TestWriterAcceptsPreviewChangesAndAgreesOnValues proves the two halves fit:
// the writer accepts the RowChange list a visit produced, assigns only the
// non-NULL columns, and every durable value equals the masked value the visit
// computed for that row.
func TestWriterAcceptsPreviewChangesAndAgreesOnValues(t *testing.T) {
	cfg := mixedNullFixture(t)
	engine := scopeEngine(t, cfg)
	ctx := context.Background()
	plan := mustPlan(t, engine)
	changes := visitChanges(t, engine, plan)

	// A row whose planned columns are all NULL is excluded from the change list
	// instead of being counted as a written row.
	pending := make([]contracts.RowChange, 0, len(changes))
	for _, change := range changes {
		if plannedColumnsAllNull(change) {
			continue
		}
		pending = append(pending, change)
	}
	if len(pending) != 2 {
		t.Fatalf("pending changes=%d, want the 2 rows with a non-NULL column", len(pending))
	}

	opts, err := cfg.DatabaseOptions(false)
	if err != nil {
		t.Fatalf("target options: %v", err)
	}
	writer, err := db.OpenWriter(ctx, opts)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer writer.Close()
	tx, err := writer.Begin(ctx, plan.Tables[0])
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	written, err := tx.Update(ctx, pending)
	if err != nil || written != int64(len(pending)) {
		_ = tx.Rollback(ctx)
		t.Fatalf("update rows=%d err=%v, want %d written rows", written, err, len(pending))
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	readback, err := sql.Open("sqlite", "file:"+cfg.Target+"?mode=ro")
	if err != nil {
		t.Fatalf("open readback: %v", err)
	}
	defer readback.Close()
	rows, err := readback.Query(`SELECT id, email, backup_email FROM people ORDER BY id`)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id int64
		var email, backup sql.NullString
		if err := rows.Scan(&id, &email, &backup); err != nil {
			t.Fatalf("scan: %v", err)
		}
		change := changes[seen]
		seen++
		key, err := strconv.ParseInt(change.Key[0].Payload(), 10, 64)
		if err != nil || key != id {
			t.Fatalf("row %d has no matching preview change", id)
		}
		for name, got := range map[string]sql.NullString{"email": email, "backup_email": backup} {
			want := change.Replacement[name]
			if got.Valid != !want.IsNull() {
				t.Fatalf("row %d column %s NULL=%t, preview said NULL=%t", id, name, !got.Valid, want.IsNull())
			}
			if got.Valid && got.String != want.Payload() {
				t.Fatalf("row %d column %s durable value differs from the previewed masked value", id, name)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("readback rows: %v", err)
	}
	if seen != len(changes) {
		t.Fatalf("readback rows=%d, want %d", seen, len(changes))
	}
}

// TestApplySkipsAllNullRowAndAssignsOnlyPlannedColumn is the single-column B2a
// case: an all-NULL row of an ordinary plan produces no UPDATE and no receipt
// row, and every committed batch holds one UPDATE that assigns only the planned
// column.
func TestApplySkipsAllNullRowAndAssignsOnlyPlannedColumn(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid')`,
		`INSERT INTO people VALUES (2, NULL)`,
		`INSERT INTO people VALUES (3, 'b@example.invalid')`,
	)
	cfg.Table, cfg.Column = "people", "email"
	ctx := context.Background()
	spy := db.NewSQLSpy()
	engine := spyEngineReaders(t, cfg, scopeKey(t), spy)
	engine.openWriter = spy.OpenWriter
	plan := mustPlan(t, engine)

	report, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !report.Complete || len(report.Committed) != 2 {
		t.Fatalf("report=%+v, want complete with one receipt per batch", report)
	}
	for i, receipt := range report.Committed {
		if receipt.Rows != 1 || receipt.Ordinal != int64(i+1) {
			t.Fatalf("receipt %d=%+v, want one row in batch %d", i, receipt, i+1)
		}
	}
	updates := spyUpdateStatements(spy)
	if len(updates) != 2 {
		t.Fatalf("UPDATE statements=%d, want one per changed row (2): %+v", len(updates), updates)
	}
	for _, statement := range updates {
		if !strings.Contains(statement, `SET "email"=? WHERE`) {
			t.Fatalf("UPDATE assigns more than the planned column: %s", statement)
		}
	}
	got := readTargetEmails(t, cfg.Target)
	if len(got) != 2 {
		t.Fatalf("non-NULL rows=%d, want 2", len(got))
	}
	if got[1] == "a@example.invalid" || got[3] == "b@example.invalid" {
		t.Fatal("a non-NULL row kept its original value")
	}
}

// spyUpdateStatements returns the SQL text of every UPDATE the spy observed.
func spyUpdateStatements(spy *db.SQLSpy) []string {
	updates := make([]string, 0)
	for _, statement := range spy.Writes() {
		if statement.Verb == "UPDATE" {
			updates = append(updates, statement.SQL)
		}
	}
	return updates
}
