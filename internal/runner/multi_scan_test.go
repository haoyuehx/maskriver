package runner

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// scopeFixture builds identical source and target SQLite snapshots from one
// statement list. The shared single-table helper can not express the several
// tables/columns a multi-scope plan needs, so this fixture is separate.
func scopeFixture(t *testing.T, statements ...string) config.Runtime {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Runtime{
		Source: filepath.Join(dir, "source.db"), Target: filepath.Join(dir, "target.db"),
		Strategy:   contracts.StrategyRef{ID: "fake_email", Version: "1"},
		KeyID:      "synthetic",
		Approved:   true,
		MinSamples: 1, SampleLimit: 100, MinRatio: .9, BatchSize: 2, Workers: 1,
	}
	for _, path := range []string{cfg.Source, cfg.Target} {
		database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			t.Fatalf("open fixture: %v", err)
		}
		for _, statement := range statements {
			if _, err = database.Exec(statement); err != nil {
				_ = database.Close()
				t.Fatalf("fixture statement failed: %v", err)
			}
		}
		if err = database.Close(); err != nil {
			t.Fatalf("close fixture: %v", err)
		}
	}
	return cfg
}

func scopeEngine(t *testing.T, cfg config.Runtime) *Engine {
	t.Helper()
	key, err := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	engine, err := Open(context.Background(), cfg, key)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

// twoTableFixture is a sensitive people.email plus a sensitive contacts.email.
// Unlike alphabetical order it is created people-first, so a plan that keeps
// the tables in this order proves the declared order was not sorted.
func twoTableFixture(t *testing.T) config.Runtime {
	t.Helper()
	return scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
		`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO contacts VALUES (1, 'c@example.invalid'), (2, 'd@example.invalid')`,
	)
}

func TestPlanCoversEachDeclaredTableInScopeOrder(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "contacts", Column: "email"}}
	engine := scopeEngine(t, cfg)

	plan, err := engine.Plan(context.Background())
	if err != nil {
		t.Fatalf("multi-table plan: %v", err)
	}
	if !plan.ScanComplete {
		t.Fatal("plan claims an incomplete scan of sensitive evidence")
	}
	if len(plan.Tables) != 2 {
		t.Fatalf("planned tables=%d, want 2", len(plan.Tables))
	}
	for i, want := range []string{"people", "contacts"} {
		table := plan.Tables[i]
		if table.Table.Table != want {
			t.Fatalf("table %d = %q, want %q (declared order)", i, table.Table.Table, want)
		}
		if len(table.Columns) != 1 || table.Columns[0].Column.Column != "email" {
			t.Fatalf("table %q columns=%+v, want one email column", want, table.Columns)
		}
		if len(table.PrimaryKey) != 1 || table.PrimaryKey[0] != "id" {
			t.Fatalf("table %q primary key=%v, want [id]", want, table.PrimaryKey)
		}
		if table.Columns[0].Scope != "sqlite/"+want+"/email" {
			t.Fatalf("table %q scope=%q", want, table.Columns[0].Scope)
		}
	}

	again, err := engine.Plan(context.Background())
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	if again.ID != plan.ID {
		t.Fatalf("plan ID is not reproducible: %q then %q", plan.ID, again.ID)
	}

	report, err := engine.Scan(context.Background(), contracts.ScanRequest{})
	if err != nil {
		t.Fatalf("multi-table scan: %v", err)
	}
	if !report.Coverage.Complete || report.Coverage.Tables != 2 ||
		report.Coverage.Columns != 2 || report.Coverage.Rows != 4 {
		t.Fatalf("scan coverage=%+v, want complete 2 tables/2 columns/4 rows", report.Coverage)
	}
	if len(report.Decisions) != 2 {
		t.Fatalf("scan decisions=%d, want one per scope", len(report.Decisions))
	}
	for i, want := range []string{"people", "contacts"} {
		if report.Decisions[i].Column.TableRef.Table != want {
			t.Fatalf("decision %d table=%q, want %q", i, report.Decisions[i].Column.TableRef.Table, want)
		}
	}
}

func TestPlanGroupsColumnsOfOneTableInScopeOrder(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, backup_email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', 'b@example.invalid'), (2, 'c@example.invalid', 'd@example.invalid')`,
	)
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "backup_email"}}
	engine := scopeEngine(t, cfg)

	plan, err := engine.Plan(context.Background())
	if err != nil {
		t.Fatalf("same-table plan: %v", err)
	}
	if len(plan.Tables) != 1 {
		t.Fatalf("planned tables=%d, want the single declared table", len(plan.Tables))
	}
	columns := plan.Tables[0].Columns
	if len(columns) != 2 {
		t.Fatalf("planned columns=%d, want 2", len(columns))
	}
	for i, want := range []string{"email", "backup_email"} {
		if columns[i].Column.Column != want {
			t.Fatalf("column %d = %q, want %q (declared order)", i, columns[i].Column.Column, want)
		}
	}
	if columns[0].Scope != "sqlite/people/email" || columns[1].Scope != "sqlite/people/backup_email" {
		t.Fatalf("per-column scopes=%q/%q", columns[0].Scope, columns[1].Scope)
	}
}

func TestPlanRefusesWhenOneScopeIsNotSensitive(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
		`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO contacts VALUES (1, 'alpha'), (2, 'beta')`,
	)
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "contacts", Column: "email"}}
	engine := scopeEngine(t, cfg)
	if !engine.cfg.Approved {
		t.Fatal("fixture must exercise the approved path")
	}

	plan, err := engine.Plan(context.Background())
	if !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatalf("plan with one unresolved scope error=%v, want ErrUnsafe", err)
	}
	if plan.ID != "" || plan.ScanComplete || len(plan.Tables) != 0 {
		t.Fatalf("refused plan must be empty: %+v", plan)
	}

	report, err := engine.Scan(context.Background(), contracts.ScanRequest{})
	if err != nil {
		t.Fatalf("scan with unresolved scope: %v", err)
	}
	if report.Coverage.Complete {
		t.Fatal("scan claimed complete coverage with an unresolved scope")
	}
	if report.Coverage.Tables != 2 || report.Coverage.Columns != 2 || report.Coverage.Rows != 4 {
		t.Fatalf("scan coverage=%+v, want 2 tables/2 columns/4 rows", report.Coverage)
	}
	if len(report.Decisions) != 2 {
		t.Fatalf("scan decisions=%d, want one per scope", len(report.Decisions))
	}
	if report.Decisions[0].Sensitivity != contracts.Sensitive {
		t.Fatalf("people.email decision=%v, want Sensitive", report.Decisions[0].Sensitivity)
	}
	if report.Decisions[1].Sensitivity != contracts.Unknown {
		t.Fatalf("contacts.email decision=%v, want Unknown", report.Decisions[1].Sensitivity)
	}
	want := contracts.ColumnRef{
		TableRef: contracts.TableRef{Database: "maskriver", Schema: "main", Table: "contacts"},
		Column:   "email",
	}
	if len(report.Issues) != 1 {
		t.Fatalf("scan issues=%+v, want exactly one for the unresolved scope", report.Issues)
	}
	issue := report.Issues[0]
	if issue.Status != contracts.Warning || issue.Code != contracts.UnresolvedColumn ||
		issue.Column != want || issue.Count != 1 {
		t.Fatalf("unresolved issue=%+v, want Warning/UnresolvedColumn for %+v", issue, want)
	}
}

func TestPlanRefusesTruncatedSampleInOneScope(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid'), (3, 'c@example.invalid')`,
		`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO contacts VALUES (1, 'd@example.invalid'), (2, 'e@example.invalid')`,
	)
	cfg.Scopes = []config.Scope{{Table: "contacts", Column: "email"}, {Table: "people", Column: "email"}}
	// Fewer samples than people's distinct values truncates only that scope.
	cfg.SampleLimit = 2
	engine := scopeEngine(t, cfg)

	plan, err := engine.Plan(context.Background())
	if !errors.Is(err, contracts.ErrIncomplete) {
		t.Fatalf("truncated scope plan error=%v, want ErrIncomplete", err)
	}
	if plan.ID != "" || plan.ScanComplete || len(plan.Tables) != 0 {
		t.Fatalf("refused plan must be empty: %+v", plan)
	}
}

func TestPlanRefusesMissingOrPrimaryKeyScope(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
	)
	tests := []struct {
		name  string
		scope config.Scope
		want  error
	}{
		{"missing column", config.Scope{Table: "people", Column: "absent"}, contracts.ErrInvalid},
		{"primary key column", config.Scope{Table: "people", Column: "id"}, contracts.ErrUnsafe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scoped := cfg
			scoped.Scopes = []config.Scope{{Table: "people", Column: "email"}, tt.scope}
			engine := scopeEngine(t, scoped)
			plan, err := engine.Plan(context.Background())
			if !errors.Is(err, tt.want) {
				t.Fatalf("plan error=%v, want %v", err, tt.want)
			}
			if plan.ID != "" || plan.ScanComplete || len(plan.Tables) != 0 {
				t.Fatalf("refused plan must not be partial: %+v", plan)
			}
		})
	}
}

func TestOpenRejectsDuplicateOrAmbiguousScopes(t *testing.T) {
	cfg := twoTableFixture(t)
	key, err := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*config.Runtime)
	}{
		{"duplicate scope", func(c *config.Runtime) {
			c.Table, c.Column = "", ""
			c.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "email"}}
		}},
		{"empty column scope", func(c *config.Runtime) {
			c.Table, c.Column = "", ""
			c.Scopes = []config.Scope{{Table: "people", Column: ""}}
		}},
		{"scopes with table", func(c *config.Runtime) {
			c.Table = "people"
			c.Scopes = []config.Scope{{Table: "contacts", Column: "email"}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scoped := cfg
			tt.mutate(&scoped)
			engine, err := Open(context.Background(), scoped, key)
			if !errors.Is(err, contracts.ErrInvalid) {
				if engine != nil {
					_ = engine.Close()
				}
				t.Fatalf("Open error=%v, want ErrInvalid", err)
			}
		})
	}
}

// TestMultiScopeApplyIsRefusedWithoutWriter was the B1 write boundary. Work
// package B2b-2 removed that limit: a multi-scope plan is now executed table by
// table, so the case moved to multi_apply_test.go and asserts real writes
// instead of a refusal.

func TestPlanIDBindsScopeOrder(t *testing.T) {
	cfg := twoTableFixture(t)
	forward := cfg
	forward.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "contacts", Column: "email"}}
	swapped := cfg
	swapped.Scopes = []config.Scope{{Table: "contacts", Column: "email"}, {Table: "people", Column: "email"}}
	ctx := context.Background()

	first, err := scopeEngine(t, forward).Plan(ctx)
	if err != nil {
		t.Fatalf("forward plan: %v", err)
	}
	second, err := scopeEngine(t, swapped).Plan(ctx)
	if err != nil {
		t.Fatalf("swapped plan: %v", err)
	}
	if first.SourceSnapshotID != second.SourceSnapshotID || first.TargetID != second.TargetID {
		t.Fatal("fixture mismatch: both plans must bind the same source/target pair")
	}
	if first.ID == second.ID {
		t.Fatal("scope order is not bound into the plan ID")
	}
	if first.ConfigDigest == second.ConfigDigest {
		t.Fatal("scope order is not bound into the config digest")
	}
	if first.SchemaDigest == second.SchemaDigest {
		t.Fatal("table order is not bound into the schema digest")
	}
}
