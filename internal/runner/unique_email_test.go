package runner

// Issue #113, main baseline 4de94d77b6ecf83ca83813eba6554a89fa694197.
// All databases, emails and reviewer keys in these tests are synthetic.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

func TestUniqueEmailApprovedSQLiteTransaction(t *testing.T) {
	for _, ddl := range []string{
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE)`,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL); CREATE UNIQUE INDEX people_email ON people(email)`,
	} {
		t.Run(ddl, func(t *testing.T) {
			cfg := preflightFixture(t, ddl,
				`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`)
			engine := uniqueEmailEngine(t, cfg)
			plan := mustPlan(t, engine)
			reviewer := newReviewer(t, 113)
			now := time.Now()
			_, dir := reviewer.issue(t, plan, now)
			engine.SetApprovalQuery(reviewer.readOnly(t, dir, func() time.Time { return now }))
			before := acceptanceTreeDigest(t, cfg.Source)
			originals := readEmailColumn(t, cfg.Source, "people")
			spy := &writerSpy{}
			spyEngine(t, engine, spy)
			ctx := context.Background()
			preview, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan, LimitPerTable: 10})
			if err != nil || !preview.Coverage.Complete || preview.RowsRead != 2 {
				t.Fatalf("UNIQUE preview: report=%+v error=%v", preview, err)
			}
			if spy.opens != 0 || !reflect.DeepEqual(before, acceptanceTreeDigest(t, cfg.Source)) {
				t.Fatal("UNIQUE preview performed persistent I/O")
			}
			report, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
			if err != nil || !report.Complete || report.CommitOutcomeUnknown || len(report.Committed) != 1 || report.Committed[0].Rows != 2 {
				t.Fatalf("UNIQUE apply: report=%+v error=%v", report, err)
			}
			if spy.opens != 1 || spy.begins != 1 || spy.updates != 1 || spy.commits != 1 {
				t.Fatal("UNIQUE apply did not use exactly one write transaction")
			}
			values := readEmailColumn(t, cfg.Target, "people")
			assertOriginalsChanged(t, values, originals)
			if values[1] == values[2] || len(values) != 2 {
				t.Fatal("UNIQUE apply lost rows or produced duplicate candidates")
			}
			if acceptanceTreeDigest(t, cfg.Source)["source.db"] != before["source.db"] ||
				!reflect.DeepEqual(originals, readEmailColumn(t, cfg.Source, "people")) {
				t.Fatal("UNIQUE apply changed the source")
			}
			validation, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: true})
			if err != nil || !validation.Passed(true) {
				t.Fatalf("UNIQUE strict validation: report=%+v error=%v", validation, err)
			}
			// The original database constraint still rejects duplicates; ordinary
			// distinct inserts and indexed lookups remain usable after masking.
			database, err := sql.Open("sqlite", "file:"+cfg.Target+"?mode=rw")
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if _, err := database.Exec(`INSERT INTO people VALUES (3, ?)`, values[1]); err == nil {
				t.Fatal("SQLite accepted a duplicate after masking")
			}
			if _, err := database.Exec(`INSERT INTO people VALUES (3, 'new@example.invalid')`); err != nil {
				t.Fatal("SQLite refused a distinct synthetic email")
			}
			var id int
			if err := database.QueryRow(`SELECT id FROM people WHERE email=?`, values[1]).Scan(&id); err != nil || id != 1 {
				t.Fatal("email lookup no longer resolves the masked row")
			}
		})
	}
}

func uniqueEmailFixture(t *testing.T) config.Runtime {
	t.Helper()
	return preflightFixture(t,
		`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE)`,
		`INSERT INTO people VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`)
}

func uniqueEmailEngine(t *testing.T, cfg config.Runtime) *Engine {
	t.Helper()
	engine := scopeEngine(t, cfg)
	engine.SetSQLiteUniqueEmailEnabled(true)
	return engine
}

func TestUniqueEmailRequiresExplicitRunnerOptIn(t *testing.T) {
	engine := scopeEngine(t, uniqueEmailFixture(t))
	plan := mustPlan(t, engine)
	assertApplyRefusedBeforeWriter(t, engine, plan, contracts.ErrUnsafe)
	if _, err := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan, LimitPerTable: 2}); !errors.Is(err, contracts.ErrUnsafe) {
		t.Fatal("UNIQUE preview was enabled by default")
	}
	engine.SetSQLiteUniqueEmailEnabled(true)
	if _, err := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan, LimitPerTable: 2}); err != nil {
		t.Fatal(err)
	}
	engine.SetSQLiteUniqueEmailEnabled(false)
	assertApplyRefusedBeforeWriter(t, engine, plan, contracts.ErrUnsafe)
	// A nil Engine follows the other setters' safe no-op convention.
	var absent *Engine
	absent.SetSQLiteUniqueEmailEnabled(true)
}

func TestUniqueEmailAuthorizationAndPlanGates(t *testing.T) {
	for _, mode := range []string{"no intent", "no declaration", "no query", "no record", "untrusted signature", "expired", "revoked", "tampered plan"} {
		t.Run(mode, func(t *testing.T) {
			engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
			plan := mustPlan(t, engine)
			now := time.Now()
			signer := newReviewer(t, 113)
			if mode == "expired" {
				now = now.Add(-2 * time.Hour)
			}
			store, dir := signer.issue(t, plan, now)
			engine.SetApprovalQuery(signer.readOnly(t, dir, time.Now))
			req := contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2}
			switch mode {
			case "no intent":
				req.ExplicitApply = false
			case "no declaration":
				engine.cfg.Approved = false
			case "no query":
				engine.SetApprovalQuery(nil)
			case "no record":
				engine.SetApprovalQuery(signer.readOnly(t, t.TempDir(), time.Now))
			case "untrusted signature":
				engine.SetApprovalQuery(newReviewer(t, 114).readOnly(t, dir, time.Now))
			case "revoked":
				if _, err := store.Revoke(context.Background(), history.RevokeRequest{Plan: plan, RevokedAt: now}); err != nil {
					t.Fatal(err)
				}
			case "tampered plan":
				req.Plan.ID = "tampered"
			}
			spy := &writerSpy{}
			spyEngine(t, engine, spy)
			before := acceptanceTreeDigest(t, engine.cfg.Target)
			report, err := engine.Apply(context.Background(), req)
			if err == nil || report.Complete || len(report.Committed) != 0 || spy.opens != 0 {
				t.Fatalf("authorization refusal: report=%+v error=%v opens=%d", report, err, spy.opens)
			}
			if !reflect.DeepEqual(before, acceptanceTreeDigest(t, engine.cfg.Target)) {
				t.Fatal("unauthorized UNIQUE apply changed files")
			}
		})
	}
}

func TestUniqueEmailUnsupportedConstraints(t *testing.T) {
	for _, tc := range []struct {
		name, ddl string
	}{
		{"nullable", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT UNIQUE)`},
		{"composite", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL, UNIQUE(email,id))`},
		{"partial", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL); CREATE UNIQUE INDEX emails ON people(email) WHERE id>0`},
		{"expression", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL); CREATE UNIQUE INDEX emails ON people(lower(email))`},
		{"nocase column", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL COLLATE NOCASE UNIQUE)`},
		{"rtrim index", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL); CREATE UNIQUE INDEX emails ON people(email COLLATE RTRIM)`},
		{"nocase index", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL); CREATE UNIQUE INDEX emails ON people(email COLLATE NOCASE)`},
		{"column collation overridden by index", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL COLLATE NOCASE); CREATE UNIQUE INDEX emails ON people(email COLLATE BINARY)`},
		{"descending", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL); CREATE UNIQUE INDEX emails ON people(email DESC)`},
		{"check", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE CHECK(length(email)<100))`},
		{"outgoing foreign key", `CREATE TABLE parent(email TEXT UNIQUE); CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE REFERENCES parent(email))`},
		{"incoming foreign key", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE); CREATE TABLE child(email TEXT REFERENCES people(email))`},
		{"trigger", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE); CREATE TRIGGER audit AFTER UPDATE ON people BEGIN SELECT 1; END`},
		{"replace policy", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE ON CONFLICT REPLACE)`},
		{"ignore policy", `CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE ON CONFLICT IGNORE)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := preflightFixture(t, tc.ddl,
				`INSERT INTO people(id,email) VALUES (1, 'a@example.invalid'), (2, 'b@example.invalid')`)
			engine := uniqueEmailEngine(t, cfg)
			plan, err := engine.Plan(context.Background())
			if tc.name == "expression" {
				// The existing adapter already rejects expression indexes while
				// describing them, before a plan can even be published.
				if !errors.Is(err, contracts.ErrUnsupported) {
					t.Fatalf("expression index Plan error=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertApplyRefusedBeforeWriter(t, engine, plan, nil)
			if _, err := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan, LimitPerTable: 10}); err == nil {
				t.Fatal("preview accepted unsupported UNIQUE constraints")
			}
		})
	}
}

func TestUniqueEmailScopeAndResourceLimits(t *testing.T) {
	t.Run("multiple columns", func(t *testing.T) {
		cfg := preflightFixture(t,
			`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE, backup TEXT NOT NULL)`,
			`INSERT INTO people VALUES (1, 'a@example.invalid', 'c@example.invalid'), (2, 'b@example.invalid', 'd@example.invalid')`)
		cfg.Table, cfg.Column = "", ""
		cfg.Scopes = []config.Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "backup"}}
		engine := uniqueEmailEngine(t, cfg)
		assertApplyRefusedBeforeWriter(t, engine, mustPlan(t, engine), contracts.ErrUnsafe)
	})
	t.Run("multiple tables", func(t *testing.T) {
		cfg := uniqueEmailFixture(t)
		for _, path := range []string{cfg.Source, cfg.Target} {
			uniqueEmailExec(t, path, `CREATE TABLE contacts(id INTEGER PRIMARY KEY,email TEXT NOT NULL); INSERT INTO contacts VALUES(1,'c@example.invalid')`)
		}
		cfg.Table, cfg.Column = "", ""
		cfg.Scopes = twoTableScopes()
		engine := uniqueEmailEngine(t, cfg)
		assertApplyRefusedBeforeWriter(t, engine, mustPlan(t, engine), contracts.ErrUnsafe)
	})
	t.Run("other strategy", func(t *testing.T) {
		cfg := uniqueEmailFixture(t)
		cfg.Strategy = contracts.StrategyRef{ID: "format_random", Version: "1"}
		engine := uniqueEmailEngine(t, cfg)
		assertApplyRefusedBeforeWriter(t, engine, mustPlan(t, engine), contracts.ErrUnsafe)
	})
	t.Run("table larger than transaction", func(t *testing.T) {
		cfg := uniqueEmailFixture(t)
		for _, path := range []string{cfg.Source, cfg.Target} {
			uniqueEmailExec(t, path, `INSERT INTO people VALUES(3,'c@example.invalid')`)
		}
		engine := uniqueEmailEngine(t, cfg)
		assertApplyRefusedBeforeWriter(t, engine, mustPlan(t, engine), contracts.ErrUnsafe)
	})
	t.Run("row byte limit", func(t *testing.T) {
		cfg := preflightFixture(t, `CREATE TABLE people(id TEXT NOT NULL PRIMARY KEY,email TEXT NOT NULL UNIQUE)`)
		for _, path := range []string{cfg.Source, cfg.Target} {
			uniqueEmailExec(t, path, `INSERT INTO people VALUES(?,'a@example.invalid'),('small','b@example.invalid')`, strings.Repeat("a", maxUniqueEmailRowBytes))
		}
		engine := uniqueEmailEngine(t, cfg)
		assertApplyRefusedBeforeWriter(t, engine, mustPlan(t, engine), contracts.ErrUnsafe)
	})
	t.Run("missing bounded reader", func(t *testing.T) {
		engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
		plan := mustPlan(t, engine)
		engine.source = &scanFaultReader{Reader: engine.source, capabilityReader: capabilityOf(t, engine.source)}
		assertApplyRefusedBeforeWriter(t, engine, plan, contracts.ErrUnsupported)
	})
	t.Run("streaming remains refused", func(t *testing.T) {
		engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
		plan := mustPlan(t, engine)
		if err := engine.preflight(context.Background(), plan); !errors.Is(err, contracts.ErrUnsafe) {
			t.Fatalf("streaming preflight error=%v", err)
		}
	})
	t.Run("mysql remains refused", func(t *testing.T) {
		engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
		plan := mustPlan(t, engine)
		schema, err := engine.source.Describe(context.Background(), plan.Tables[0].Table)
		if err != nil {
			t.Fatal(err)
		}
		engine.cfg.Dialect = contracts.MySQL
		if err := engine.checkUniqueEmailScope(context.Background(), plan, schema); !errors.Is(err, contracts.ErrUnsafe) {
			t.Fatal("SQLite UNIQUE exception accepted a MySQL configuration")
		}
	})
	t.Run("preview truncation", func(t *testing.T) {
		engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
		plan := mustPlan(t, engine)
		reads := &mutateAfterCallsReader{Reader: engine.source, bounded: engine.source.(db.BoundedReader)}
		engine.source = reads
		before := acceptanceTreeDigest(t, engine.cfg.Target)
		report, err := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan, LimitPerTable: 1})
		if !errors.Is(err, contracts.ErrIncomplete) || report.Coverage.Complete || reads.calls != 0 || report.RowsRead != 0 || !reflect.DeepEqual(before, acceptanceTreeDigest(t, engine.cfg.Target)) {
			t.Fatal("truncated UNIQUE preview exceeded its read cap, claimed complete or changed files")
		}
	})
}

func uniqueEmailExec(t *testing.T, path, query string, args ...any) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestUniqueEmailAcrossPagesAndRowCap(t *testing.T) {
	for _, n := range []int{maxPage + 1, maxUniqueEmailRows, maxUniqueEmailRows + 1} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			cfg := preflightFixture(t,
				`CREATE TABLE people (id INTEGER PRIMARY KEY,email TEXT NOT NULL UNIQUE)`,
				fmt.Sprintf(`WITH RECURSIVE ids(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM ids WHERE x<%d)
				 INSERT INTO people SELECT x, 'synthetic'||x||'@example.invalid' FROM ids`, n))
			cfg.SampleLimit = 10000
			cfg.BatchSize = maxUniqueEmailRows
			engine := uniqueEmailEngine(t, cfg)
			plan := mustPlan(t, engine)
			spy := &writerSpy{}
			spyEngine(t, engine, spy)
			before := acceptanceTreeDigest(t, cfg.Target)
			report, err := engine.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: cfg.BatchSize})
			if n > maxUniqueEmailRows {
				if !errors.Is(err, contracts.ErrUnsafe) || spy.opens != 0 || report.Complete || !reflect.DeepEqual(before, acceptanceTreeDigest(t, cfg.Target)) {
					t.Fatal("row cap did not refuse before writing")
				}
				return
			}
			if err != nil || !report.Complete || len(report.Committed) != 1 || report.Committed[0].Rows != int64(n) || spy.begins != 1 {
				t.Fatalf("paged UNIQUE apply: report=%+v error=%v begins=%d", report, err, spy.begins)
			}
			validation, err := engine.Validate(context.Background(), contracts.ValidateRequest{Plan: plan, Strict: true})
			if err != nil || !validation.Passed(true) {
				t.Fatal("paged UNIQUE apply failed strict validation")
			}
		})
	}
}

func TestUniqueEmailLargeKeyTransaction(t *testing.T) {
	cfg := preflightFixture(t,
		`CREATE TABLE people(id TEXT NOT NULL PRIMARY KEY,email TEXT NOT NULL UNIQUE)`,
		`WITH RECURSIVE ids(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM ids WHERE x<1000)
		 INSERT INTO people SELECT printf('%03900d', x), 'synthetic'||x||'@example.invalid' FROM ids`)
	cfg.BatchSize, cfg.SampleLimit = 1000, 10000
	engine := uniqueEmailEngine(t, cfg)
	plan := mustPlan(t, engine)
	before := acceptanceTreeDigest(t, cfg.Target)
	spy := &writerSpy{}
	spyEngine(t, engine, spy)
	report, err := engine.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 1000})
	// This shape previously passed the row-count/projected-byte bounds, then
	// spilled SQLite's cache and blocked its own commit-time reader. The resource
	// preflight must now refuse it before any Writer, rather than deadlocking
	// itself or omitting the mandatory drift check to make it succeed.
	if !errors.Is(err, contracts.ErrUnsafe) || report.Complete || len(report.Committed) != 0 || spy.opens != 0 || !reflect.DeepEqual(before, acceptanceTreeDigest(t, cfg.Target)) {
		t.Fatalf("large file preflight: report=%+v error=%v opens=%d", report, err, spy.opens)
	}
}

func TestUniqueEmailCacheAndIndexBudget(t *testing.T) {
	for _, mode := range []string{"small persistent cache", "index fanout"} {
		t.Run(mode, func(t *testing.T) {
			cfg := uniqueEmailFixture(t)
			for _, path := range []string{cfg.Source, cfg.Target} {
				if mode == "small persistent cache" {
					uniqueEmailExec(t, path, `PRAGMA default_cache_size=1`)
				} else {
					for n := 0; n < maxUniqueEmailIndexes; n++ {
						uniqueEmailExec(t, path, fmt.Sprintf(`CREATE INDEX extra%d ON people(email)`, n))
					}
				}
			}
			engine := uniqueEmailEngine(t, cfg)
			assertApplyRefusedBeforeWriter(t, engine, mustPlan(t, engine), contracts.ErrUnsupported)
		})
	}
}

func TestUniqueEmailOccupiedCandidateRefusedBeforeWriter(t *testing.T) {
	for _, occupantFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(occupantFirst), func(t *testing.T) {
			cfg := uniqueEmailFixture(t)
			engine := uniqueEmailEngine(t, cfg)
			initial := mustPlan(t, engine)
			original, _ := contracts.NewValue(contracts.Text, "a@example.invalid")
			candidate, err := engine.maskColumn(context.Background(), original, initial.Tables[0].Columns[0])
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{cfg.Source, cfg.Target} {
				uniqueEmailExec(t, path, `DELETE FROM people`)
				first, second := original.Payload(), candidate.Payload()
				if occupantFirst {
					first, second = second, first
				}
				uniqueEmailExec(t, path, `INSERT INTO people VALUES(1,?),(2,?)`, first, second)
			}
			// Regenerate the real plan after changing the synthetic source.
			plan := mustPlan(t, engine)
			assertApplyRefusedBeforeWriter(t, engine, plan, contracts.ErrConflict)
			if _, err := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan, LimitPerTable: 2}); !errors.Is(err, contracts.ErrConflict) {
				t.Fatalf("preview failed to catch occupied candidate: %v", err)
			}
		})
	}
}

func TestUniqueEmailCandidateCollisionChecks(t *testing.T) {
	value := func(s string) contracts.Value { v, _ := contracts.NewValue(contracts.Text, s); return v }
	change := func(old, next string) contracts.RowChange {
		return contracts.RowChange{Expected: contracts.Row{"email": value(old)}, Replacement: contracts.Row{"email": value(next)}}
	}
	for _, tc := range []struct {
		name    string
		changes []contracts.RowChange
		want    error
	}{
		{"candidate collision", []contracts.RowChange{change("a@x.invalid", "same@maskriver.local"), change("b@x.invalid", "same@maskriver.local")}, contracts.ErrConflict},
		{"swap", []contracts.RowChange{change("a@x.invalid", "b@x.invalid"), change("b@x.invalid", "a@x.invalid")}, contracts.ErrConflict},
		{"unchanged", []contracts.RowChange{change("a@x.invalid", "a@x.invalid")}, contracts.ErrUnsafe},
		{"null candidate", []contracts.RowChange{{Expected: contracts.Row{"email": value("a@x.invalid")}, Replacement: contracts.Row{"email": {}}}}, contracts.ErrUnsafe},
		{"distinct", []contracts.RowChange{change("a@x.invalid", "c@maskriver.local"), change("b@x.invalid", "d@maskriver.local")}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkUniqueEmailCandidates(context.Background(), "email", tc.changes)
			if !errors.Is(err, tc.want) {
				t.Fatalf("collision check error=%v, want %v", err, tc.want)
			}
		})
	}
}

type uniqueEmailFaultWriter struct {
	contracts.Writer
	mode   string
	cancel context.CancelFunc
	wrote  *int64
}

func (w uniqueEmailFaultWriter) Begin(ctx context.Context, plan contracts.TablePlan) (contracts.WriteTx, error) {
	tx, err := w.Writer.Begin(ctx, plan)
	if err != nil {
		return nil, err
	}
	return uniqueEmailFaultTx{WriteTx: tx, mode: w.mode, cancel: w.cancel, wrote: w.wrote}, nil
}

type uniqueEmailFaultTx struct {
	contracts.WriteTx
	mode   string
	cancel context.CancelFunc
	wrote  *int64
}

func (tx uniqueEmailFaultTx) Update(ctx context.Context, changes []contracts.RowChange) (int64, error) {
	n, err := tx.WriteTx.Update(ctx, changes[:1])
	*tx.wrote = n
	if err != nil {
		return n, err
	}
	switch tx.mode {
	case "cancel":
		tx.cancel()
		return n, ctx.Err()
	case "update failure":
		return n, contracts.ErrDatabase
	default:
		// After a real first UPDATE, deliberately hit SQLite's real UNIQUE
		// constraint on the second. The driver must roll back the entire batch.
		broken := changes[1]
		broken.Replacement = changes[0].Replacement
		return tx.WriteTx.Update(ctx, []contracts.RowChange{broken})
	}
}

func TestUniqueEmailMidWriteFailureRollsBackWholeTransaction(t *testing.T) {
	for _, mode := range []string{"constraint failure", "update failure", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
			plan := mustPlan(t, engine)
			before := acceptanceTreeDigest(t, engine.cfg.Target)
			originals := readEmailColumn(t, engine.cfg.Target, "people")
			spy := &writerSpy{}
			spyEngine(t, engine, spy)
			open := engine.openWriter
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var firstUpdated int64
			engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
				writer, err := open(ctx, opts)
				if err != nil {
					return nil, err
				}
				return uniqueEmailFaultWriter{Writer: writer, mode: mode, cancel: cancel, wrote: &firstUpdated}, nil
			}
			report, err := engine.Apply(ctx, contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
			if err == nil || firstUpdated != 1 || report.Complete || len(report.Committed) != 0 || report.CommitOutcomeUnknown || spy.begins != 1 || spy.commits != 0 || spy.rollbacks != 1 {
				t.Fatalf("mid-write failure: report=%+v error=%v wrote=%d begin/commit/rollback=%d/%d/%d", report, err, firstUpdated, spy.begins, spy.commits, spy.rollbacks)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation identity was lost")
			}
			if !reflect.DeepEqual(originals, readEmailColumn(t, engine.cfg.Target, "people")) || !reflect.DeepEqual(before, acceptanceTreeDigest(t, engine.cfg.Target)) {
				t.Fatal("a failed UNIQUE transaction persisted a prefix or changed source/target files")
			}
		})
	}
}

func TestUniqueEmailTargetDriftAndUnknownCommit(t *testing.T) {
	t.Run("target drift", func(t *testing.T) {
		engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
		plan := mustPlan(t, engine)
		uniqueEmailExec(t, engine.cfg.Target, `UPDATE people SET email='changed@example.invalid' WHERE id=2`)
		assertApplyRefusedBeforeWriter(t, engine, plan, contracts.ErrConflict)
	})
	t.Run("unknown commit", func(t *testing.T) {
		engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
		plan := mustPlan(t, engine)
		spy := &writerSpy{commitErr: contracts.ErrCommitUnknown, commitThenErr: true}
		spyEngine(t, engine, spy)
		report, err := engine.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
		if !errors.Is(err, contracts.ErrCommitUnknown) || !report.CommitOutcomeUnknown || report.Complete || len(report.Committed) != 0 || spy.commits != 1 || spy.rollbacks != 0 {
			t.Fatalf("unknown commit was retried or misreported: report=%+v error=%v", report, err)
		}
	})
}

func TestUniqueEmailRangeDriftBeforeWriterRollsBack(t *testing.T) {
	for _, side := range []string{"target", "source", "both"} {
		t.Run(side, func(t *testing.T) {
			engine := uniqueEmailEngine(t, uniqueEmailFixture(t))
			plan := mustPlan(t, engine)
			spy := &writerSpy{}
			spyEngine(t, engine, spy)
			open := engine.openWriter
			var afterExternalChange map[string][32]byte
			engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
				// This change lands AFTER candidate collection and BEFORE Begin.
				// The first two expected-value UPDATEs still match, so only a
				// transaction-wide recheck can notice the unprocessed third row.
				if side != "source" {
					uniqueEmailExec(t, engine.cfg.Target, `INSERT INTO people VALUES(3,'new@example.invalid')`)
				}
				if side != "target" {
					uniqueEmailExec(t, engine.cfg.Source, `INSERT INTO people VALUES(3,'new@example.invalid')`)
				}
				afterExternalChange = acceptanceTreeDigest(t, engine.cfg.Target)
				return open(ctx, opts)
			}
			report, err := engine.Apply(context.Background(), contracts.ApplyRequest{Plan: plan, ExplicitApply: true, BatchSize: 2})
			if !errors.Is(err, contracts.ErrConflict) || report.Complete || len(report.Committed) != 0 || spy.begins != 1 || spy.updates != 1 || spy.commits != 0 || spy.rollbacks != 1 {
				t.Fatalf("post-collection drift: report=%+v error=%v begin/update/commit/rollback=%d/%d/%d/%d", report, err, spy.begins, spy.updates, spy.commits, spy.rollbacks)
			}
			if !reflect.DeepEqual(afterExternalChange, acceptanceTreeDigest(t, engine.cfg.Target)) {
				t.Fatal("range drift left a masked prefix after rollback")
			}
		})
	}
}

func TestUniqueEmailPreviewSQLSpy(t *testing.T) {
	cfg := uniqueEmailFixture(t)
	key, _ := contracts.NewValue(contracts.Bytes, strings.Repeat("k", 32))
	spy := db.NewSQLSpy()
	before := acceptanceTreeDigest(t, cfg.Target)
	engine := spyEngineReaders(t, cfg, key, spy)
	engine.SetSQLiteUniqueEmailEnabled(true)
	plan := mustPlan(t, engine)
	if _, err := engine.Preview(context.Background(), contracts.PreviewRequest{Plan: plan, LimitPerTable: 2}); err != nil {
		t.Fatal(err)
	}
	// The supplemental catalog session is mode=ro and SELECT-only; all actual
	// value reads below are observed through the existing driver-level spy.
	for _, statement := range spy.Writes() {
		// BoundedReader.PageWithin pins each length check and value read in
		// one read-only transaction. SQLSpy conservatively labels even these
		// mode=ro BEGIN/COMMIT operations as transaction intent.
		if statement.SQL != "BEGIN" && statement.SQL != "COMMIT" && statement.SQL != "ROLLBACK" {
			t.Fatalf("UNIQUE preview issued write SQL: %+v", statement)
		}
	}
	if !reflect.DeepEqual(before, acceptanceTreeDigest(t, cfg.Target)) {
		t.Fatal("UNIQUE preview changed the fixture tree")
	}
}
