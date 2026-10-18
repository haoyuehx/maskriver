package runner

// Regression tests for work package B1: Preview and Validate must cover every
// planned table and every planned column, while Apply keeps refusing a
// multi-scope plan. Every fixture is synthetic and lives in t.TempDir().

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/verify"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// scopeKey returns the synthetic masking key the multi-scope fixtures use. The
// value is never written into a plan or a report.
func scopeKey(t *testing.T) contracts.Value {
	t.Helper()
	key, err := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	return key
}

// textValue builds a Text value for a synthetic fixture literal.
func textValue(t *testing.T, raw string) contracts.Value {
	t.Helper()
	value, err := contracts.NewValue(contracts.Text, raw)
	if err != nil {
		t.Fatalf("text value %q: %v", raw, err)
	}
	return value
}

func mustPlan(t *testing.T, e *Engine) contracts.Plan {
	t.Helper()
	plan, err := e.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

// twoTableScopes declares people.email followed by contacts.email, the same
// declared order the two-table fixture creates.
func twoTableScopes() []config.Scope {
	return []config.Scope{{Table: "people", Column: "email"}, {Table: "contacts", Column: "email"}}
}

// TestPreviewVisitsEveryPlannedTable is the primary B1 regression: a plan with
// two tables must preview both, count one row per physical row, and carry the
// planned column of the table each row came from.
func TestPreviewVisitsEveryPlannedTable(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	engine := scopeEngine(t, cfg)
	ctx := context.Background()
	plan := mustPlan(t, engine)

	var changes []contracts.RowChange
	coverage, err := engine.visit(ctx, plan, 0, func(change contracts.RowChange) error {
		changes = append(changes, change)
		return nil
	})
	if err != nil {
		t.Fatalf("visit: %v", err)
	}
	if coverage.Tables != 2 || coverage.Columns != 2 || coverage.Rows != 4 || !coverage.Complete {
		t.Fatalf("visit coverage=%+v, want complete 2 tables/2 columns/4 rows", coverage)
	}
	// Plan order is people then contacts, so the changes must follow it.
	want := []string{
		"a@example.invalid", "b@example.invalid",
		"c@example.invalid", "d@example.invalid",
	}
	if len(changes) != len(want) {
		t.Fatalf("RowChanges=%d, want %d", len(changes), len(want))
	}
	for i, original := range want {
		got, ok := changes[i].Expected["email"]
		if !ok || got != textValue(t, original) {
			t.Fatalf("change %d Expected[email] present=%t, want the planned email column", i, ok)
		}
		if len(changes[i].Expected) != 1 || len(changes[i].Replacement) != 1 {
			t.Fatalf("change %d Expected/Replacement=%v/%v, want only the email column",
				i, changes[i].Expected, changes[i].Replacement)
		}
		if changes[i].Replacement["email"] == got {
			t.Fatalf("change %d claims an unchanged replacement", i)
		}
		if len(changes[i].Key) != 1 || changes[i].Key[0].IsNull() {
			t.Fatalf("change %d key=%v, want one primary key value", i, changes[i].Key)
		}
	}

	report, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !report.Coverage.Complete || report.Coverage.Tables != 2 ||
		report.Coverage.Columns != 2 || report.Coverage.Rows != 4 || report.RowsRead != 4 {
		t.Fatalf("preview report=%+v rows_read=%d, want complete 2 tables/2 columns/4 rows",
			report.Coverage, report.RowsRead)
	}
}

// TestPreviewCoversEveryColumnOfOneTable pins that a second planned column on
// the same table is masked and reported, and that Rows counts rows rather than
// column instances.
func TestPreviewCoversEveryColumnOfOneTable(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, backup_email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', 'b@example.invalid'), (2, 'c@example.invalid', 'd@example.invalid')`,
	)
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "backup_email"}}
	engine := scopeEngine(t, cfg)
	ctx := context.Background()
	plan := mustPlan(t, engine)

	var changes []contracts.RowChange
	coverage, err := engine.visit(ctx, plan, 0, func(change contracts.RowChange) error {
		changes = append(changes, change)
		return nil
	})
	if err != nil {
		t.Fatalf("visit: %v", err)
	}
	if coverage.Tables != 1 || coverage.Columns != 2 || coverage.Rows != 2 || !coverage.Complete {
		t.Fatalf("coverage=%+v, want complete 1 table/2 columns/2 rows", coverage)
	}
	if len(changes) != 2 {
		t.Fatalf("RowChanges=%d, want one per row (2), not one per column", len(changes))
	}
	parents := []map[string]string{
		{"email": "a@example.invalid", "backup_email": "b@example.invalid"},
		{"email": "c@example.invalid", "backup_email": "d@example.invalid"},
	}
	for i, want := range parents {
		for column, original := range want {
			if got, ok := changes[i].Expected[column]; !ok || got != textValue(t, original) {
				t.Fatalf("change %d Expected[%s] present=%t", i, column, ok)
			}
			masked, ok := changes[i].Replacement[column]
			if !ok {
				t.Fatalf("change %d has no replacement for planned column %s", i, column)
			}
			if masked == changes[i].Expected[column] {
				t.Fatalf("change %d column %s was not masked", i, column)
			}
		}
	}

	report, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !report.Coverage.Complete || report.Coverage.Tables != 1 ||
		report.Coverage.Columns != 2 || report.Coverage.Rows != 2 || report.RowsRead != 2 {
		t.Fatalf("preview report=%+v rows_read=%d, want complete 1 table/2 columns/2 rows",
			report.Coverage, report.RowsRead)
	}
}

// TestPreviewLimitIsPerTable proves LimitPerTable is a per-table row budget: a
// table that overruns it is incomplete, and it never consumes the next table's
// budget.
func TestPreviewLimitIsPerTable(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid'), (3, 'c@example.invalid')`,
		`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO contacts VALUES (1, 'd@example.invalid'), (2, 'e@example.invalid'), (3, 'f@example.invalid')`,
	)
	cfg.Scopes = twoTableScopes()
	engine := scopeEngine(t, cfg)
	ctx := context.Background()
	plan := mustPlan(t, engine)

	report, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan, LimitPerTable: 2})
	if !errors.Is(err, contracts.ErrIncomplete) {
		t.Fatalf("truncated preview error=%v, want ErrIncomplete", err)
	}
	if report.Coverage.Complete {
		t.Fatalf("truncated preview claims complete coverage: %+v", report.Coverage)
	}
	if report.Coverage.Tables != 0 || report.Coverage.Columns != 0 {
		t.Fatalf("coverage=%+v, want no completed table", report.Coverage)
	}
	// 2 rows per table, not 2 rows in total: the budget restarts per table.
	if report.RowsRead != 4 || report.Coverage.Rows != 4 {
		t.Fatalf("rows read=%d coverage rows=%d, want 4 (limit 2 per table, 2 tables)",
			report.RowsRead, report.Coverage.Rows)
	}

	// A budget that covers every table exactly stays complete.
	complete, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan, LimitPerTable: 3})
	if err != nil {
		t.Fatalf("in-budget preview: %v", err)
	}
	if !complete.Coverage.Complete || complete.RowsRead != 6 {
		t.Fatalf("in-budget preview report=%+v rows_read=%d, want complete 6 rows",
			complete.Coverage, complete.RowsRead)
	}
}

// TestPreviewMixedNullColumnsNeverFabricates keeps a NULL original NULL in the
// preview: the column is carried in both rows so the change matches the plan,
// and no value is invented for it.
func TestPreviewMixedNullColumnsNeverFabricates(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, backup_email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', NULL), (2, NULL, 'b@example.invalid'), (3, NULL, NULL)`,
	)
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "backup_email"}}
	engine := scopeEngine(t, cfg)
	ctx := context.Background()
	plan := mustPlan(t, engine)

	var changes []contracts.RowChange
	coverage, err := engine.visit(ctx, plan, 0, func(change contracts.RowChange) error {
		changes = append(changes, change)
		return nil
	})
	if err != nil {
		t.Fatalf("visit with NULL planned values: %v", err)
	}
	if coverage.Tables != 1 || coverage.Columns != 2 || coverage.Rows != 3 || !coverage.Complete {
		t.Fatalf("coverage=%+v, want complete 1 table/2 columns/3 rows", coverage)
	}
	if len(changes) != 3 {
		t.Fatalf("RowChanges=%d, want one per row including the all-NULL row", len(changes))
	}
	// Row 1: email set, backup_email NULL.
	if got, ok := changes[0].Replacement["email"]; !ok || got.IsNull() {
		t.Fatalf("row 1 email replacement present=%t", ok)
	}
	if got, ok := changes[0].Expected["backup_email"]; !ok || !got.IsNull() {
		t.Fatalf("row 1 backup_email expected present=%t, want an explicit NULL", ok)
	}
	if got, ok := changes[0].Replacement["backup_email"]; !ok || !got.IsNull() {
		t.Fatalf("row 1 backup_email replacement present=%t, want an explicit NULL", ok)
	}
	// Row 2: email NULL, backup_email set.
	if got, ok := changes[1].Expected["email"]; !ok || !got.IsNull() {
		t.Fatalf("row 2 email expected present=%t, want an explicit NULL", ok)
	}
	if got, ok := changes[1].Replacement["email"]; !ok || !got.IsNull() {
		t.Fatalf("row 2 email replacement present=%t, want an explicit NULL", ok)
	}
	if got, ok := changes[1].Replacement["backup_email"]; !ok || got.IsNull() {
		t.Fatalf("row 2 backup_email replacement present=%t", ok)
	}
	// Row 3: every planned column is NULL, so the row claims no change; the
	// column set is still complete and the primary key is still carried.
	for _, name := range []string{"email", "backup_email"} {
		if got, ok := changes[2].Expected[name]; !ok || !got.IsNull() {
			t.Fatalf("all-NULL row expected %s present=%t, want an explicit NULL", name, ok)
		}
		if got, ok := changes[2].Replacement[name]; !ok || !got.IsNull() {
			t.Fatalf("all-NULL row replacement %s present=%t, want an explicit NULL", name, ok)
		}
	}
	if len(changes[2].Key) != 1 {
		t.Fatalf("all-NULL row key=%v, want the primary key", changes[2].Key)
	}

	report, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan})
	if err != nil {
		t.Fatalf("preview with NULL planned values: %v", err)
	}
	if !report.Coverage.Complete || report.RowsRead != 3 {
		t.Fatalf("preview report=%+v rows_read=%d", report.Coverage, report.RowsRead)
	}
}

// TestMultiTablePreviewIssuesNoWriteSQLThroughSQLSpy is the driver-level proof
// that a multi-table preview stays read-only: no write or transaction intent,
// at least one SELECT, and an unchanged fixture tree.
func TestMultiTablePreviewIssuesNoWriteSQLThroughSQLSpy(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	ctx := context.Background()
	spy := db.NewSQLSpy()
	// The baseline is taken before any connection is opened, so an open-time
	// side effect can not hide inside the measurement.
	before := acceptanceTreeDigest(t, cfg.Target)
	engine := spyEngineReaders(t, cfg, scopeKey(t), spy)

	plan := mustPlan(t, engine)
	report, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan, LimitPerTable: 10000})
	if err != nil {
		t.Fatalf("multi-table preview: %v", err)
	}
	if !report.Coverage.Complete || report.Coverage.Tables != 2 || report.RowsRead != 4 {
		t.Fatalf("preview report=%+v rows_read=%d", report.Coverage, report.RowsRead)
	}
	if writes := spy.Writes(); len(writes) != 0 {
		t.Fatalf("multi-table preview issued write/transaction intent: %+v", writes)
	}
	selects := 0
	for _, statement := range spy.Statements() {
		if statement.Verb == "SELECT" {
			selects++
		}
	}
	if selects == 0 {
		t.Fatalf("spy observed no SELECT; the read assertion would be vacuous: %+v", spy.Statements())
	}
	if err = engine.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if after := acceptanceTreeDigest(t, cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("multi-table preview changed the fixture tree")
	}
}

// rewriteTargetEmail overwrites the planned column of one table in the target
// copy, so a post-write validation has genuinely changed values to compare.
func rewriteTargetEmail(t *testing.T, path, table string, values []string) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer database.Close()
	for i, value := range values {
		if _, err := database.Exec(`UPDATE "`+table+`" SET email = ? WHERE id = ?`, value, i+1); err != nil {
			t.Fatalf("rewrite %s: %v", table, err)
		}
	}
}

// execTargetStatement runs one synthetic DDL/DML statement against the target.
func execTargetStatement(t *testing.T, path, statement string) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(statement); err != nil {
		t.Fatalf("target statement %q: %v", statement, err)
	}
}

// validateEngine opens the two-table fixture, publishes the plan and only then
// lets the test diverge the target copy, which is the post-write state validate
// is meant to judge.
func validateEngine(t *testing.T, mutate func(cfg config.Runtime)) (*Engine, contracts.Plan) {
	t.Helper()
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	engine := scopeEngine(t, cfg)
	plan := mustPlan(t, engine)
	if mutate != nil {
		mutate(cfg)
	}
	return engine, plan
}

// TestValidateCoversEveryPlannedTable is the B1 validate case: two tables with
// already-masked target values pass, and a divergence in either table keeps the
// whole report from PASS.
func TestValidateCoversEveryPlannedTable(t *testing.T) {
	ctx := context.Background()
	masked := func(cfg config.Runtime) {
		rewriteTargetEmail(t, cfg.Target, "people", []string{"m1@maskriver.local", "m2@maskriver.local"})
		rewriteTargetEmail(t, cfg.Target, "contacts", []string{"m3@maskriver.local", "m4@maskriver.local"})
	}

	t.Run("every table masked", func(t *testing.T) {
		engine, plan := validateEngine(t, masked)
		report, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if !report.Passed(true) {
			t.Fatalf("two-table validate did not pass: issues=%+v coverage=%+v",
				report.Issues, report.Coverage)
		}
		if !report.Coverage.Complete || report.Coverage.Tables != 2 ||
			report.Coverage.Columns != 2 || report.Coverage.Rows != 4 {
			t.Fatalf("coverage=%+v, want complete 2 tables/2 columns/4 rows", report.Coverage)
		}
		if report.PlanID != plan.ID {
			t.Fatalf("report PlanID=%q, want the supplied plan", report.PlanID)
		}
	})

	t.Run("one table unchanged", func(t *testing.T) {
		engine, plan := validateEngine(t, func(cfg config.Runtime) {
			rewriteTargetEmail(t, cfg.Target, "people", []string{"m1@maskriver.local", "m2@maskriver.local"})
		})
		report, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if report.Passed(true) {
			t.Fatalf("validate passed although contacts.email is unchanged: %+v", report.Coverage)
		}
		if !report.Coverage.Complete || report.Coverage.Tables != 2 {
			t.Fatalf("coverage=%+v, want both tables verified but not passing", report.Coverage)
		}
		want := contracts.ColumnRef{TableRef: tableRef("contacts"), Column: "email"}
		found := false
		for _, issue := range report.Issues {
			if issue.Code == contracts.UnchangedValue && issue.Column == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("issues=%+v, want an UnchangedValue issue for contacts.email", report.Issues)
		}
	})

	t.Run("one table row count differs", func(t *testing.T) {
		engine, plan := validateEngine(t, func(cfg config.Runtime) {
			rewriteTargetEmail(t, cfg.Target, "people", []string{"m1@maskriver.local", "m2@maskriver.local"})
			rewriteTargetEmail(t, cfg.Target, "contacts", []string{"m3@maskriver.local", "m4@maskriver.local"})
			execTargetStatement(t, cfg.Target, `INSERT INTO contacts VALUES (99, 'm99@maskriver.local')`)
		})
		report, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if report.Passed(true) {
			t.Fatalf("validate passed with a diverging row count: coverage=%+v", report.Coverage)
		}
		found := false
		for _, issue := range report.Issues {
			if issue.Code == contracts.RowCountMismatch {
				found = true
			}
		}
		if !found {
			t.Fatalf("issues=%+v, want a RowCountMismatch issue", report.Issues)
		}
	})

	t.Run("one table constraint differs", func(t *testing.T) {
		engine, plan := validateEngine(t, func(cfg config.Runtime) {
			rewriteTargetEmail(t, cfg.Target, "people", []string{"m1@maskriver.local", "m2@maskriver.local"})
			rewriteTargetEmail(t, cfg.Target, "contacts", []string{"m3@maskriver.local", "m4@maskriver.local"})
			execTargetStatement(t, cfg.Target, `CREATE UNIQUE INDEX contacts_email_key ON contacts (email)`)
		})
		// The plan re-check describes both snapshots, so a diverging schema is
		// refused before the verifier runs: the engine can not report PASS.
		report, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
		if err == nil {
			t.Fatalf("validate accepted a diverging target schema: coverage=%+v issues=%+v",
				report.Coverage, report.Issues)
		}
		if report.Passed(true) {
			t.Fatal("refused validate still claimed PASS")
		}
		// The verifier's own per-table constraint gate is exercised directly,
		// because the plan re-check above stops before it can run.
		direct, err := verify.New().Validate(ctx, engine.source, engine.target,
			contracts.ValidateRequest{Plan: plan, Strict: true})
		if err != nil {
			t.Fatalf("direct verify: %v", err)
		}
		if direct.Passed(true) || direct.Coverage.Complete {
			t.Fatalf("verify passed with a diverging schema: coverage=%+v", direct.Coverage)
		}
		if direct.Coverage.Tables != 1 {
			t.Fatalf("verify coverage tables=%d, want only people verified", direct.Coverage.Tables)
		}
		found := false
		for _, issue := range direct.Issues {
			if issue.Code == contracts.SchemaMismatch {
				found = true
			}
		}
		if !found {
			t.Fatalf("verify issues=%+v, want a SchemaMismatch issue", direct.Issues)
		}
	})
}
