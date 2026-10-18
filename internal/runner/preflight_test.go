package runner

// Regression tests for work package B2b-1, the whole-plan static capability
// preflight. Every refusal here must happen before any Writer capability is
// requested and before the target tree changes, because a partly writable plan
// is never authorized. Every fixture is synthetic and lives in t.TempDir().

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// preflightFixture builds a single-scope people.email fixture from the given
// statements; the planned table is always people.
func preflightFixture(t *testing.T, statements ...string) config.Runtime {
	t.Helper()
	cfg := scopeFixture(t, statements...)
	cfg.Table, cfg.Column = "people", "email"
	return cfg
}

// twoPeopleRows is the synthetic data every preflight fixture stores.
var twoPeopleRows = []string{
	`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
	`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
}

// assertApplyRefused proves an Apply refusal happens before any Writer
// capability is requested and before the target tree changes.
func assertApplyRefused(t *testing.T, e *Engine, plan contracts.Plan, want error) {
	t.Helper()
	before := acceptanceTreeDigest(t, e.cfg.Target)
	spy := &writerSpy{}
	spyEngine(t, e, spy)
	report, err := e.Apply(context.Background(), contracts.ApplyRequest{
		Plan: plan, ExplicitApply: true, BatchSize: 2,
	})
	if !errors.Is(err, want) {
		t.Fatalf("Apply error=%v, want %v", err, want)
	}
	if report.Complete || len(report.Committed) != 0 {
		t.Fatalf("refused apply reported success: %+v", report)
	}
	if spy.opens != 0 || spy.begins != 0 || spy.updates != 0 || spy.commits != 0 {
		t.Fatalf("refused apply used the Writer: opens=%d begins=%d updates=%d commits=%d",
			spy.opens, spy.begins, spy.updates, spy.commits)
	}
	if after := acceptanceTreeDigest(t, e.cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("refused apply changed the target fixture tree")
	}
}

// TestPreflightRefusesTriggerOnPlannedTable covers the trigger rule.
// pkg/contracts has no trigger metadata, so schema equality can not prove that
// no trigger exists; the gate refuses any trigger on a table the plan writes.
func TestPreflightRefusesTriggerOnPlannedTable(t *testing.T) {
	statements := append(append([]string(nil), twoPeopleRows...),
		`CREATE TABLE audit (id INTEGER PRIMARY KEY, note TEXT NULL)`,
		`CREATE TRIGGER people_email_audit AFTER UPDATE OF email ON people
		 BEGIN INSERT INTO audit(note) VALUES ('changed'); END`,
	)
	engine := scopeEngine(t, preflightFixture(t, statements...))
	plan := mustPlan(t, engine)
	assertApplyRefused(t, engine, plan, contracts.ErrUnsupported)
	// Preview is Apply's dry run, so the same gate stops it before any row read.
	if _, err := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan}); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("preview error=%v, want ErrUnsupported", err)
	}
}

// TestPreflightRefusesTargetOnlyTrigger is the P1 regression from review: the
// write lands in the target snapshot, so a trigger that exists only there still
// fires. Checking the source alone reported success while the trigger wrote
// extra rows.
func TestPreflightRefusesTargetOnlyTrigger(t *testing.T) {
	statements := append(append([]string(nil), twoPeopleRows...),
		`CREATE TABLE audit (id INTEGER PRIMARY KEY, note TEXT NULL)`,
	)
	cfg := preflightFixture(t, statements...)
	// Add the trigger to the target only, after both snapshots were built.
	targetDB, err := sql.Open("sqlite", "file:"+cfg.Target+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = targetDB.Exec(`CREATE TRIGGER people_email_audit AFTER UPDATE OF email ON people
		BEGIN INSERT INTO audit(note) VALUES ('changed'); END`); err != nil {
		_ = targetDB.Close()
		t.Fatal(err)
	}
	if err = targetDB.Close(); err != nil {
		t.Fatal(err)
	}
	engine := scopeEngine(t, cfg)
	assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
}

// TestPreflightRefusesCaseVariantTrigger is the second P1 regression: SQLite
// stores tbl_name exactly as written in the trigger DDL, so a trigger declared
// ON PEOPLE for table people was previously missed by an exact-match lookup.
func TestPreflightRefusesCaseVariantTrigger(t *testing.T) {
	engine := scopeEngine(t, preflightFixture(t, append(append([]string(nil), twoPeopleRows...),
		`CREATE TABLE audit (id INTEGER PRIMARY KEY, note TEXT NULL)`,
		`CREATE TRIGGER people_email_audit AFTER UPDATE OF email ON PEOPLE
		 BEGIN INSERT INTO audit(note) VALUES ('changed'); END`,
	)...))
	assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
}

// TestPreflightRefusesForeignKeyOnModifiedColumn covers both directions of the
// foreign-key rule: the modified column is referenced by another table, or the
// modified column is itself a foreign-key column.
func TestPreflightRefusesForeignKeyOnModifiedColumn(t *testing.T) {
	t.Run("referenced by another table", func(t *testing.T) {
		engine := scopeEngine(t, preflightFixture(t,
			`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
			`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
			`CREATE TABLE children (id INTEGER PRIMARY KEY, parent_email TEXT NULL REFERENCES people(email))`,
		))
		assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
	})

	t.Run("modified column is a foreign key", func(t *testing.T) {
		engine := scopeEngine(t, preflightFixture(t,
			`CREATE TABLE domains (id INTEGER PRIMARY KEY, name TEXT UNIQUE)`,
			`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL REFERENCES domains(name))`,
			`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
		))
		assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
	})
}

// TestPreflightRefusesUniqueConstraintOnModifiedColumn covers both unique forms:
// an explicit unique index and an inline UNIQUE column constraint. Masking is
// deterministic but not proven collision-free, so it can break the constraint.
func TestPreflightRefusesUniqueConstraintOnModifiedColumn(t *testing.T) {
	t.Run("unique index", func(t *testing.T) {
		statements := append(append([]string(nil), twoPeopleRows...),
			`CREATE UNIQUE INDEX people_email_unique ON people(email)`)
		engine := scopeEngine(t, preflightFixture(t, statements...))
		assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsafe)
	})

	t.Run("inline unique column", func(t *testing.T) {
		engine := scopeEngine(t, preflightFixture(t,
			`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL UNIQUE)`,
			`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
		))
		assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsafe)
	})
}

// TestPreflightRefusesCheckOnModifiedColumn covers the CHECK rule: the gate
// does not evaluate SQL expressions, so any CHECK that may mention the modified
// column refuses the plan.
func TestPreflightRefusesCheckOnModifiedColumn(t *testing.T) {
	engine := scopeEngine(t, preflightFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL CHECK (email <> 'forbidden@example.invalid'))`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
	))
	assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
}

// TestPreflightAllowsPlainIndexAndUnrelatedCheck is the allow case: a plain
// non-unique index carries no constraint, so an UPDATE that changes the indexed
// column is allowed, and a CHECK that does not mention the column can not be
// affected by changing it.
func TestPreflightAllowsPlainIndexAndUnrelatedCheck(t *testing.T) {
	engine := scopeEngine(t, preflightFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, nickname TEXT NULL CHECK (nickname <> 'reserved'))`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', 'n1'), (2, 'b@example.invalid', 'n2')`,
		`CREATE INDEX people_email_lookup ON people(email)`,
	))
	ctx := context.Background()
	plan := mustPlan(t, engine)
	spy := &writerSpy{}
	spyEngine(t, engine, spy)
	report, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
	if err != nil || !report.Complete {
		t.Fatalf("apply with a plain index report=%+v error=%v", report, err)
	}
	if spy.opens != 1 || spy.begins != 1 || spy.updates != 1 || spy.commits != 1 {
		t.Fatalf("allowed plan used the Writer %d/%d/%d/%d times, want 1/1/1/1",
			spy.opens, spy.begins, spy.updates, spy.commits)
	}
	validation, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
	if err != nil || !validation.Passed(true) {
		t.Fatalf("strict validate after apply: passed=%t issues=%+v error=%v",
			validation.Passed(true), validation.Issues, err)
	}
}

// coverageDroppingReader wraps a real Reader and clears one coverage flag, the
// way an adapter that could not verify a constraint class reports itself. It
// forwards the internal capability probes unchanged.
type coverageDroppingReader struct {
	contracts.Reader
	caps capabilityReader
	drop func(*contracts.SchemaCoverage)
}

func (r *coverageDroppingReader) Describe(ctx context.Context, ref contracts.TableRef) (contracts.TableSchema, error) {
	schema, err := r.Reader.Describe(ctx, ref)
	if err == nil {
		r.drop(&schema.Coverage)
	}
	return schema, err
}

func (r *coverageDroppingReader) Quiescent(ctx context.Context) error { return r.caps.Quiescent(ctx) }

func (r *coverageDroppingReader) HasTriggers(ctx context.Context, ref contracts.TableRef) (bool, error) {
	return r.caps.HasTriggers(ctx, ref)
}

// dropCoverage wraps both snapshots so source and target still describe
// identically and only the preflight coverage gate can refuse.
func dropCoverage(t *testing.T, e *Engine, drop func(*contracts.SchemaCoverage)) {
	t.Helper()
	for _, side := range []*contracts.Reader{&e.source, &e.target} {
		caps, ok := (*side).(capabilityReader)
		if !ok {
			t.Fatalf("fixture reader has no internal capabilities")
		}
		*side = &coverageDroppingReader{Reader: *side, caps: caps, drop: drop}
	}
}

// TestPreflightRefusesIncompleteCoverage requires every constraint class to be
// described before the capability rules may trust it: an undescribed class is
// not an absent class. Columns and PrimaryKey are already required by describe
// during Plan, so those two classes fail even earlier; the preflight owns the
// other four.
func TestPreflightRefusesIncompleteCoverage(t *testing.T) {
	for _, tc := range []struct {
		name string
		drop func(*contracts.SchemaCoverage)
		// planRefuses marks a class describe already gates during Plan.
		planRefuses bool
	}{
		{"columns", func(c *contracts.SchemaCoverage) { c.Columns = false }, true},
		{"primary key", func(c *contracts.SchemaCoverage) { c.PrimaryKey = false }, true},
		{"indexes", func(c *contracts.SchemaCoverage) { c.Indexes = false }, false},
		{"unique", func(c *contracts.SchemaCoverage) { c.Unique = false }, false},
		{"foreign keys", func(c *contracts.SchemaCoverage) { c.ForeignKeys = false }, false},
		{"checks", func(c *contracts.SchemaCoverage) { c.Checks = false }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := scopeEngine(t, preflightFixture(t, twoPeopleRows...))
			dropCoverage(t, engine, tc.drop)
			// Plan itself does not require the constraint classes, so such a plan
			// still publishes; the write path refuses it.
			plan, err := engine.Plan(context.Background())
			if tc.planRefuses {
				if !errors.Is(err, contracts.ErrIncomplete) {
					t.Fatalf("plan error=%v, want ErrIncomplete", err)
				}
				if plan.ID != "" {
					t.Fatalf("refused plan must stay empty: %+v", plan)
				}
				return
			}
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			assertApplyRefused(t, engine, plan, contracts.ErrIncomplete)
		})
	}
}

// TestPreflightRefusesWholePlanWhenSecondTableIsUnsupported proves the gate is
// whole-plan: table people is provably writable, but the plan is refused because
// the second table has a trigger. Apply still stops earlier at the
// single-scope write gate (see TestMultiScopeApplyIsRefusedWithoutWriter), so
// the whole-plan result is observed through the shared preflight and Preview;
// no Writer is opened in either path.
func TestPreflightRefusesWholePlanWhenSecondTableIsUnsupported(t *testing.T) {
	cfg := scopeFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
		`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO contacts VALUES (1, 'c@example.invalid'), (2, 'd@example.invalid')`,
		`CREATE TRIGGER contacts_email_audit AFTER UPDATE OF email ON contacts BEGIN SELECT 1; END`,
	)
	cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "contacts", Column: "email"}}
	cfg.Table, cfg.Column = "", ""
	engine := scopeEngine(t, cfg)
	ctx := context.Background()
	plan := mustPlan(t, engine)
	if err := engine.preflight(ctx, plan); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("preflight error=%v, want ErrUnsupported from the second table", err)
	}
	before := acceptanceTreeDigest(t, engine.cfg.Target)
	spy := &writerSpy{}
	spyEngine(t, engine, spy)
	if _, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan}); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("preview error=%v, want ErrUnsupported", err)
	}
	if _, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2}); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("apply error=%v, want ErrUnsupported", err)
	}
	if spy.opens != 0 || spy.begins != 0 || spy.updates != 0 || spy.commits != 0 {
		t.Fatalf("unsupported plan used the Writer: %+v", spy)
	}
	if after := acceptanceTreeDigest(t, engine.cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("unsupported plan changed the target fixture tree")
	}
}

// TestCheckPlanBindingRejectsMismatchedScopes exercises the binding recheck
// directly: a plan whose scope list, primary key or column type disagrees with
// the live schema is rejected even though ValidateShape saw a well-formed plan.
func TestCheckPlanBindingRejectsMismatchedScopes(t *testing.T) {
	engine := scopeEngine(t, preflightFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL, nickname TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid', 'n1'), (2, 'b@example.invalid', 'n2')`,
	))
	ctx := context.Background()
	plan := mustPlan(t, engine)
	schemas, err := engine.describePlanTables(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.checkPlanBinding(plan, schemas); err != nil {
		t.Fatalf("live plan binding refused: %v", err)
	}

	omitted := plan
	omitted.Tables = append([]contracts.TablePlan(nil), plan.Tables...)
	omitted.Tables[0] = plan.Tables[0]
	omitted.Tables[0].Columns = nil
	if err := engine.checkPlanBinding(omitted, schemas); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("omitted column error=%v, want ErrInvalid", err)
	}

	wrongColumn := plan
	wrongColumn.Tables = append([]contracts.TablePlan(nil), plan.Tables...)
	wrongColumn.Tables[0] = plan.Tables[0]
	wrongColumn.Tables[0].Columns = append([]contracts.ColumnPlan(nil), plan.Tables[0].Columns...)
	wrongColumn.Tables[0].Columns[0].Column.Column = "nickname"
	if err := engine.checkPlanBinding(wrongColumn, schemas); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("out-of-scope column error=%v, want ErrInvalid", err)
	}

	wrongKey := plan
	wrongKey.Tables = append([]contracts.TablePlan(nil), plan.Tables...)
	wrongKey.Tables[0] = plan.Tables[0]
	wrongKey.Tables[0].PrimaryKey = []string{"email"}
	if err := engine.checkPlanBinding(wrongKey, schemas); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("wrong primary key error=%v, want ErrInvalid", err)
	}
}

// execOnTarget runs statements on the target snapshot only, the way an
// out-of-band writer or a drifted copy would.
func execOnTarget(t *testing.T, cfg config.Runtime, statements ...string) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+cfg.Target+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range statements {
		if _, err = database.Exec(statement); err != nil {
			_ = database.Close()
			t.Fatalf("target statement %q: %v", statement, err)
		}
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestPreflightRefusesTargetOnlyIncomingReference is the B2b-2 regression: the
// write lands in the target, so a referencing table - or a foreign key on a
// table that exists in both - that exists only on the target still guards the
// rows this plan changes. Enumerating the source alone missed it; before the fix
// the plan opened a Writer, began a transaction and only the driver refused the
// UPDATE (ErrDatabase), which relies on the writer DSN keeping foreign_keys=1.
func TestPreflightRefusesTargetOnlyIncomingReference(t *testing.T) {
	t.Run("referencing table only on the target", func(t *testing.T) {
		cfg := preflightFixture(t, twoPeopleRows...)
		execOnTarget(t, cfg,
			`CREATE TABLE children (id INTEGER PRIMARY KEY, person_email TEXT NULL REFERENCES people(email))`,
			`INSERT INTO children VALUES (1, 'a@example.invalid')`,
		)
		engine := scopeEngine(t, cfg)
		assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
	})

	t.Run("foreign key only in the target schema", func(t *testing.T) {
		cfg := preflightFixture(t, append(append([]string(nil), twoPeopleRows...),
			`CREATE TABLE children (id INTEGER PRIMARY KEY, person_email TEXT NULL)`,
			`INSERT INTO children VALUES (1, 'a@example.invalid')`,
		)...)
		execOnTarget(t, cfg,
			`DROP TABLE children`,
			`CREATE TABLE children (id INTEGER PRIMARY KEY, person_email TEXT NULL REFERENCES people(email))`,
			`INSERT INTO children VALUES (1, 'a@example.invalid')`,
		)
		engine := scopeEngine(t, cfg)
		assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
	})
}

// TestPreflightRefusesCaseVariantIncomingReference is the second B2b-2
// regression: SQLite resolves identifiers case-insensitively, so
// REFERENCES PEOPLE (EMAIL) points at table people, column email, but
// PRAGMA foreign_key_list reports the names exactly as the DDL wrote them. The
// exact-match comparison missed the reference and the plan reached the writer.
func TestPreflightRefusesCaseVariantIncomingReference(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reference string
	}{
		{"table name", `REFERENCES PEOPLE (email)`},
		{"column name", `REFERENCES people (EMAIL)`},
		{"table and column name", `REFERENCES PEOPLE (EMAIL)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := scopeEngine(t, preflightFixture(t,
				`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
				`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
				`CREATE TABLE children (id INTEGER PRIMARY KEY, person_email TEXT NULL `+tc.reference+`)`,
				`INSERT INTO children VALUES (1, 'a@example.invalid')`,
			))
			assertApplyRefused(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
		})
	}
}

// TestPreflightRefusesIncomingReferenceFromUnplannedTable pins the normal case
// and proves which gate refuses it: the referencing table is in neither the plan
// nor the planned table's own schema, and the planned column carries no unique
// or foreign-key constraint, so only the incoming-reference gate can refuse.
func TestPreflightRefusesIncomingReferenceFromUnplannedTable(t *testing.T) {
	engine := scopeEngine(t, preflightFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
		`CREATE TABLE children (id INTEGER PRIMARY KEY, person_email TEXT NULL REFERENCES people(email))`,
	))
	ctx := context.Background()
	plan := mustPlan(t, engine)
	schemas, err := engine.describePlanTables(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err = engine.checkTableCapabilities(ctx, plan.Tables[0], schemas["people"]); err != nil {
		t.Fatalf("planned table capabilities=%v, want nil: only the incoming-reference gate may refuse this fixture", err)
	}
	if err = engine.checkIncomingReferences(ctx, plan); !errors.Is(err, contracts.ErrUnsupported) {
		t.Fatalf("checkIncomingReferences=%v, want ErrUnsupported", err)
	}
	assertApplyRefused(t, engine, plan, contracts.ErrUnsupported)
}

// TestPreflightIncomingReferenceAlreadyBlockedByAnotherGate records the shapes
// that need no incoming-reference fix, and proves the blocking gate with a direct
// call instead of asserting it: an incoming reference SQLite can enforce needs a
// unique parent key, so the UNIQUE gate refuses the plan with ErrUnsafe before
// the incoming check runs, and a primary-key parent can never be planned at all.
func TestPreflightIncomingReferenceAlreadyBlockedByAnotherGate(t *testing.T) {
	ctx := context.Background()
	t.Run("unique parent column", func(t *testing.T) {
		engine := scopeEngine(t, preflightFixture(t,
			`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL UNIQUE)`,
			`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
			`CREATE TABLE children (id INTEGER PRIMARY KEY, person_email TEXT NULL REFERENCES people(email))`,
			`INSERT INTO children VALUES (1, 'a@example.invalid')`,
		))
		plan := mustPlan(t, engine)
		schemas, err := engine.describePlanTables(ctx, plan)
		if err != nil {
			t.Fatal(err)
		}
		if err = engine.checkTableCapabilities(ctx, plan.Tables[0], schemas["people"]); !errors.Is(err, contracts.ErrUnsafe) {
			t.Fatalf("checkTableCapabilities=%v, want ErrUnsafe from the unique gate", err)
		}
		assertApplyRefused(t, engine, plan, contracts.ErrUnsafe)
	})

	t.Run("primary key parent", func(t *testing.T) {
		// REFERENCES people without a column list targets the primary key, which a
		// plan can never include: the scope itself is refused during Plan.
		cfg := scopeFixture(t,
			`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
			`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`,
			`CREATE TABLE children (id INTEGER PRIMARY KEY, person_id INTEGER NULL REFERENCES people)`,
		)
		cfg.Table, cfg.Column = "people", "id"
		engine := scopeEngine(t, cfg)
		plan, err := engine.Plan(ctx)
		if !errors.Is(err, contracts.ErrUnsafe) {
			t.Fatalf("primary-key scope plan error=%v, want ErrUnsafe", err)
		}
		if plan.ID != "" {
			t.Fatalf("refused plan must stay empty: %+v", plan)
		}
	})
}
