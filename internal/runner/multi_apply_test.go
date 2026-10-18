package runner

// Regression tests for work package B2b-2: Apply writes every planned table and
// every planned column, one transaction per batch, one receipt per committed
// batch, and stops at the first failing table or batch without losing the
// receipts of already committed work. Every fixture is synthetic, lives in
// t.TempDir(), and the assertions read the real SQLite file back.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// readEmailColumn reads id -> email of one table, skipping NULL emails, so an
// assertion can compare durable values without knowing the masked output.
func readEmailColumn(t *testing.T, path, table string) map[int]string {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open readback: %v", err)
	}
	defer func() { _ = database.Close() }()
	rows, err := database.Query(fmt.Sprintf(`SELECT id, "email" FROM "%s" ORDER BY id`, table))
	if err != nil {
		t.Fatalf("readback %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	values := make(map[int]string)
	for rows.Next() {
		var id int
		var email sql.NullString
		if err := rows.Scan(&id, &email); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		if email.Valid {
			values[id] = email.String
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("readback rows %s: %v", table, err)
	}
	return values
}

// assertOriginalsChanged requires every non-NULL email of one table to differ
// from its synthetic original.
func assertOriginalsChanged(t *testing.T, values map[int]string, originals map[int]string) {
	t.Helper()
	for id, original := range originals {
		got, ok := values[id]
		if !ok {
			t.Fatalf("row %d lost its email", id)
		}
		if got == original {
			t.Fatalf("row %d kept the original email", id)
		}
	}
}

// applyFixture opens one fixture with a writer spy and publishes its plan.
func applyFixture(t *testing.T, cfg config.Runtime) (*Engine, contracts.Plan, *writerSpy) {
	t.Helper()
	engine := scopeEngine(t, cfg)
	plan := mustPlan(t, engine)
	spy := &writerSpy{}
	spyEngine(t, engine, spy)
	return engine, plan, spy
}

// TestApplyWritesEveryPlannedTable is the primary B2b-2 case: two planned tables
// with one column each are both written, the receipts follow plan order with a
// per-table 1-based ordinal sequence, and a strict validation passes afterwards.
func TestApplyWritesEveryPlannedTable(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	engine, plan, spy := applyFixture(t, cfg)
	ctx := context.Background()
	// Only the source file is compared: the target is what this run rewrites.
	before := acceptanceTreeDigest(t, cfg.Source)["source.db"]

	report, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if err != nil {
		t.Fatalf("multi-table apply: %v", err)
	}
	if !report.Complete || report.CommitOutcomeUnknown {
		t.Fatalf("report=%+v, want a complete run with known outcomes", report)
	}
	want := []contracts.BatchReceipt{
		{Table: tableRef("people"), Ordinal: 1, Rows: 1},
		{Table: tableRef("people"), Ordinal: 2, Rows: 1},
		{Table: tableRef("contacts"), Ordinal: 1, Rows: 1},
		{Table: tableRef("contacts"), Ordinal: 2, Rows: 1},
	}
	if !reflect.DeepEqual(report.Committed, want) {
		t.Fatalf("receipts=%+v, want %+v", report.Committed, want)
	}
	if report.PlanID != plan.ID {
		t.Fatalf("report PlanID=%q, want the supplied plan", report.PlanID)
	}
	// One Writer for the run, one transaction per batch, tables in plan order.
	if spy.opens != 1 || spy.begins != 4 || spy.updates != 4 || spy.commits != 4 || spy.closes != 1 {
		t.Fatalf("writer usage opens/begins/updates/commits/closes=%d/%d/%d/%d/%d, want 1/4/4/4/1",
			spy.opens, spy.begins, spy.updates, spy.commits, spy.closes)
	}
	wantBegun := []contracts.TableRef{tableRef("people"), tableRef("people"),
		tableRef("contacts"), tableRef("contacts")}
	if !reflect.DeepEqual(spy.begun, wantBegun) {
		t.Fatalf("writer began %+v, want %+v (plan order)", spy.begun, wantBegun)
	}
	assertOriginalsChanged(t, readEmailColumn(t, cfg.Target, "people"),
		map[int]string{1: "a@example.invalid", 2: "b@example.invalid"})
	assertOriginalsChanged(t, readEmailColumn(t, cfg.Target, "contacts"),
		map[int]string{1: "c@example.invalid", 2: "d@example.invalid"})
	if after := acceptanceTreeDigest(t, cfg.Source)["source.db"]; before != after {
		t.Fatal("apply changed the source snapshot")
	}
	if strings.Contains(fmt.Sprintf("%+v", report), "example.invalid") {
		t.Fatal("apply report leaked a synthetic sensitive value")
	}
	validation, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
	if err != nil || !validation.Passed(true) {
		t.Fatalf("strict validate after multi-table apply: passed=%t issues=%+v error=%v",
			validation.Passed(true), validation.Issues, err)
	}
}

// TestApplyWritesEveryPlannedColumnOfOneTable is the multi-column case: both
// planned columns of every row are assigned in one UPDATE, one receipt records
// the batch, and a strict validation passes.
func TestApplyWritesEveryPlannedColumnOfOneTable(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, backup_email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', 'b@example.invalid')`,
		`INSERT INTO people VALUES (2, 'c@example.invalid', 'd@example.invalid')`,
	)
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "backup_email"}}
	ctx := context.Background()
	spy := db.NewSQLSpy()
	engine := spyEngineReaders(t, cfg, scopeKey(t), spy)
	engine.openWriter = spy.OpenWriter
	plan := mustPlan(t, engine)

	report, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if err != nil {
		t.Fatalf("multi-column apply: %v", err)
	}
	want := []contracts.BatchReceipt{{Table: tableRef("people"), Ordinal: 1, Rows: 2}}
	if !report.Complete || !reflect.DeepEqual(report.Committed, want) {
		t.Fatalf("report=%+v, want one receipt of two rows", report)
	}
	updates := spyUpdateStatements(spy)
	if len(updates) != 2 {
		t.Fatalf("UPDATE statements=%d, want one per row (2): %+v", len(updates), updates)
	}
	for _, statement := range updates {
		if !strings.Contains(statement, `SET "email"=?,"backup_email"=? WHERE`) {
			t.Fatalf("UPDATE does not assign both planned columns: %s", statement)
		}
	}
	validation, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
	if err != nil || !validation.Passed(true) {
		t.Fatalf("strict validate after multi-column apply: passed=%t issues=%+v error=%v",
			validation.Passed(true), validation.Issues, err)
	}
}

// TestApplySkipsAllNullPlannedRow is the B2a NULL contract under the new
// multi-column Apply: a row whose planned columns are all NULL issues no
// statement and produces no receipt, while a mixed row updates only its
// non-NULL column.
func TestApplySkipsAllNullPlannedRow(t *testing.T) {
	cfg := mixedNullFixture(t)
	cfg.BatchSize = 2
	ctx := context.Background()
	spy := db.NewSQLSpy()
	engine := spyEngineReaders(t, cfg, scopeKey(t), spy)
	engine.openWriter = spy.OpenWriter
	plan := mustPlan(t, engine)

	report, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if err != nil {
		t.Fatalf("apply with mixed NULL rows: %v", err)
	}
	// Page one holds rows 1 and 2 (both writable), page two holds only the
	// all-NULL row 3, which is skipped without a transaction or a receipt.
	want := []contracts.BatchReceipt{{Table: tableRef("people"), Ordinal: 1, Rows: 2}}
	if !report.Complete || !reflect.DeepEqual(report.Committed, want) {
		t.Fatalf("report=%+v, want one receipt of the two writable rows", report)
	}
	updates := spyUpdateStatements(spy)
	if len(updates) != 2 {
		t.Fatalf("UPDATE statements=%d, want 2 (the all-NULL row writes nothing): %+v", len(updates), updates)
	}
	assigned := make([]string, 0, len(updates))
	for _, statement := range updates {
		_, rest, ok := strings.Cut(statement, " SET ")
		if !ok {
			t.Fatalf("UPDATE without SET clause: %s", statement)
		}
		set, _, ok := strings.Cut(rest, " WHERE ")
		if !ok {
			t.Fatalf("UPDATE without WHERE clause: %s", statement)
		}
		assigned = append(assigned, set)
	}
	sort.Strings(assigned)
	wantAssigned := []string{`"backup_email"=?`, `"email"=?`}
	if !reflect.DeepEqual(assigned, wantAssigned) {
		t.Fatalf("assigned columns=%v, want %v (one column per mixed NULL row)", assigned, wantAssigned)
	}
	// The all-NULL row must still be NULL in both planned columns.
	database, err := sql.Open("sqlite", "file:"+cfg.Target+"?mode=ro")
	if err != nil {
		t.Fatalf("open readback: %v", err)
	}
	defer func() { _ = database.Close() }()
	var email, backup sql.NullString
	if err = database.QueryRow(`SELECT email, backup_email FROM people WHERE id=3`).Scan(&email, &backup); err != nil {
		t.Fatalf("read all-NULL row: %v", err)
	}
	if email.Valid || backup.Valid {
		t.Fatal("the all-NULL row gained a value")
	}
}

// TestApplyRefusesIdentityMask pins the fail-closed masking rule: a strategy
// that returns the original non-NULL value is refused with ErrUnsafe, and the
// refusal comes from the dry run, so no Writer is ever opened. The fixture pairs
// an empty string (which "blank" masks to itself) with a detectable address.
func TestApplyRefusesIdentityMask(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, ''), (2, 'a@example.invalid')`,
	)
	cfg.Table, cfg.Column = "people", "email"
	cfg.Strategy = contracts.StrategyRef{ID: "blank", Version: "1"}
	// Half the sampled values match the email rule, which is enough evidence for
	// this fixture: the refusal under test must come from the identity mask.
	cfg.MinRatio = 0.5
	engine, plan, spy := applyFixture(t, cfg)
	before := acceptanceTreeDigest(t, cfg.Target)

	report, err := engine.Apply(context.Background(),
		contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("identity mask apply error=%v, want ErrUnsafe", err)
	}
	if report.Complete || len(report.Committed) != 0 || report.CommitOutcomeUnknown {
		t.Fatalf("refused apply reported work: %+v", report)
	}
	if spy.opens != 0 || spy.begins != 0 || spy.updates != 0 || spy.commits != 0 {
		t.Fatalf("refused apply used the Writer: %+v", spy)
	}
	if after := acceptanceTreeDigest(t, cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("refused identity mask changed the target fixture tree")
	}
}

// TestApplyRefusesSecondTableConstraintClass is the static whole-plan gate for a
// plan whose FIRST table is writable: every constraint class of the second table
// refuses the whole plan before any Writer capability is requested, so nothing is
// written to the first table either.
func TestApplyRefusesSecondTableConstraintClass(t *testing.T) {
	people := []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
	}
	cases := []struct {
		name       string
		statements []string
		want       error
	}{
		{
			name: "foreign key column",
			statements: []string{
				`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL REFERENCES domains(name))`,
				`CREATE TABLE domains (id INTEGER PRIMARY KEY, name TEXT UNIQUE)`,
			},
			want: contracts.ErrUnsupported,
		},
		{
			name:       "unique constraint",
			statements: []string{`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL UNIQUE)`},
			want:       contracts.ErrUnsafe,
		},
		{
			name:       "check constraint",
			statements: []string{`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL CHECK (email <> 'x'))`},
			want:       contracts.ErrUnsupported,
		},
		{
			name: "trigger",
			statements: []string{
				`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL)`,
				`CREATE TRIGGER contacts_email_audit AFTER UPDATE OF email ON contacts BEGIN SELECT 1; END`,
			},
			want: contracts.ErrUnsupported,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statements := append(append([]string(nil), people...), tc.statements...)
			statements = append(statements, `INSERT INTO contacts VALUES (1, 'c@example.invalid'), (2, 'd@example.invalid')`)
			cfg := scopeFixture(t, statements...)
			cfg.Scopes = twoTableScopes()
			engine, plan, _ := applyFixture(t, cfg)
			// The first table alone passes the capability rules, so only the second
			// table can be the reason for the refusal.
			schemas, err := engine.describePlanTables(context.Background(), plan)
			if err != nil {
				t.Fatalf("describe: %v", err)
			}
			if err = engine.checkTableCapabilities(context.Background(), plan.Tables[0], schemas["people"]); err != nil {
				t.Fatalf("first table capabilities=%v, want nil", err)
			}
			assertApplyRefused(t, engine, plan, tc.want)
		})
	}
}

// TestApplyStopsInSecondTableAndKeepsCommittedReceipts injects the failure inside
// the SECOND table's second batch. The writer spy records every Begin call, so the
// test proves execution really reached that batch instead of stopping earlier, and
// the receipts of the three committed batches must survive the failure.
func TestApplyStopsInSecondTableAndKeepsCommittedReceipts(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	engine, plan, spy := applyFixture(t, cfg)
	spy.updateErr = contracts.ErrDatabase
	spy.updateErrAt = 4

	report, err := engine.Apply(context.Background(),
		contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if !errors.Is(err, contracts.ErrDatabase) {
		t.Fatalf("injected second-table failure error=%v, want ErrDatabase", err)
	}
	if report.Complete || report.CommitOutcomeUnknown {
		t.Fatalf("failed run report=%+v, want incomplete with a known failure", report)
	}
	want := []contracts.BatchReceipt{
		{Table: tableRef("people"), Ordinal: 1, Rows: 1},
		{Table: tableRef("people"), Ordinal: 2, Rows: 1},
		{Table: tableRef("contacts"), Ordinal: 1, Rows: 1},
	}
	if !reflect.DeepEqual(report.Committed, want) {
		t.Fatalf("receipts=%+v, want the three committed batches %+v", report.Committed, want)
	}
	// Instrumentation: the fourth Begin is contacts batch 2, the failing batch.
	wantBegun := []contracts.TableRef{tableRef("people"), tableRef("people"),
		tableRef("contacts"), tableRef("contacts")}
	if !reflect.DeepEqual(spy.begun, wantBegun) {
		t.Fatalf("writer began %+v, want %+v (the failing second-table batch must be reached)",
			spy.begun, wantBegun)
	}
	if spy.updates != 4 || spy.commits != 3 || spy.rollbacks != 1 || spy.closes != 1 {
		t.Fatalf("updates/commits/rollbacks/closes=%d/%d/%d/%d, want 4/3/1/1",
			spy.updates, spy.commits, spy.rollbacks, spy.closes)
	}
	assertOriginalsChanged(t, readEmailColumn(t, cfg.Target, "people"),
		map[int]string{1: "a@example.invalid", 2: "b@example.invalid"})
	contacts := readEmailColumn(t, cfg.Target, "contacts")
	if contacts[1] == "c@example.invalid" {
		t.Fatal("the committed batch of the second table was lost")
	}
	if contacts[2] != "d@example.invalid" {
		t.Fatalf("the failed batch wrote row 2 of contacts: %q", contacts[2])
	}
}

// TestApplyStopsAtUnknownCommitInSecondTable is the unknown-outcome case: the
// designated Commit returns ErrCommitUnknown, the batch is reported as unknown,
// no later batch is executed and nothing is retried or rolled back.
func TestApplyStopsAtUnknownCommitInSecondTable(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	engine, plan, spy := applyFixture(t, cfg)
	spy.commitErr = fmt.Errorf("driver status: %w", contracts.ErrCommitUnknown)
	spy.commitErrAt = 3

	report, err := engine.Apply(context.Background(),
		contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if !errors.Is(err, contracts.ErrCommitUnknown) {
		t.Fatalf("unknown commit error=%v, want ErrCommitUnknown", err)
	}
	if !report.CommitOutcomeUnknown || report.Complete {
		t.Fatalf("unknown commit report=%+v, want outcome_unknown with complete=false", report)
	}
	want := []contracts.BatchReceipt{
		{Table: tableRef("people"), Ordinal: 1, Rows: 1},
		{Table: tableRef("people"), Ordinal: 2, Rows: 1},
	}
	if !reflect.DeepEqual(report.Committed, want) {
		t.Fatalf("receipts=%+v, want only the two batches committed before the unknown one %+v",
			report.Committed, want)
	}
	// Instrumentation: the third Begin is contacts batch 1, whose Commit is the
	// designated unknown one. A retry or a later batch would raise these counts.
	wantBegun := []contracts.TableRef{tableRef("people"), tableRef("people"), tableRef("contacts")}
	if !reflect.DeepEqual(spy.begun, wantBegun) {
		t.Fatalf("writer began %+v, want %+v (no later batch after the unknown commit)",
			spy.begun, wantBegun)
	}
	if spy.commits != 3 || spy.updates != 3 || spy.rollbacks != 0 || spy.closes != 1 {
		t.Fatalf("commits/updates/rollbacks/closes=%d/%d/%d/%d, want 3/3/0/1",
			spy.commits, spy.updates, spy.rollbacks, spy.closes)
	}
	contacts := readEmailColumn(t, cfg.Target, "contacts")
	if contacts[1] != "c@example.invalid" || contacts[2] != "d@example.invalid" {
		t.Fatalf("contacts rows=%v, want both unchanged: the spy never committed that batch", contacts)
	}
}

// TestApplyCancelledInSecondTableRollsBackAndReleasesWriter cancels the context
// while the second table's first batch is open. The batch must roll back, the
// receipts of the first table must survive, and the writer must release its
// connection so the target file is writable again.
func TestApplyCancelledInSecondTableRollsBackAndReleasesWriter(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	engine, plan, spy := applyFixture(t, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	spy.cancel = cancel
	spy.cancelAt = 3

	report, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled apply error=%v, want context.Canceled", err)
	}
	if report.Complete || report.CommitOutcomeUnknown || len(report.Committed) != 2 {
		t.Fatalf("cancelled report=%+v, want the two committed batches and complete=false", report)
	}
	if spy.rollbacks != 1 || spy.closes != 1 {
		t.Fatalf("rollbacks/closes=%d/%d, want 1/1", spy.rollbacks, spy.closes)
	}
	if !spy.rollbackDeadline || spy.rollbackContextErr != nil || spy.rollbackDeadlineRemaining <= 0 {
		t.Fatalf("rollback used context deadline=%t err=%v remaining=%s",
			spy.rollbackDeadline, spy.rollbackContextErr, spy.rollbackDeadlineRemaining)
	}
	contacts := readEmailColumn(t, cfg.Target, "contacts")
	if contacts[1] != "c@example.invalid" || contacts[2] != "d@example.invalid" {
		t.Fatalf("contacts rows=%v, want both unchanged after the rollback", contacts)
	}
	// The write connection must be released: a fresh writer can take the lock.
	probe, err := sql.Open("sqlite", "file:"+cfg.Target+"?mode=rw&_pragma=busy_timeout(2000)")
	if err != nil {
		t.Fatalf("open probe: %v", err)
	}
	defer func() { _ = probe.Close() }()
	if _, err = probe.Exec(`UPDATE contacts SET id = id WHERE id = 1`); err != nil {
		t.Fatalf("target write lock was not released: %v", err)
	}
}
