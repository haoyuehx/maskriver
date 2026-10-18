package runner

// Regression tests for #51: a planned table that held rows at plan, preflight
// and dry-run time and is then emptied during Apply must refuse with
// contracts.ErrConflict instead of reporting Complete=true with zero rows
// written for that table. Both snapshots are emptied together, so the page
// comparison can not notice: only the per-table row count can.

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// deleteAllRows empties one table of one synthetic fixture file.
func deleteAllRows(t *testing.T, path, table string) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatalf("open fixture for emptying: %v", err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec(`DELETE FROM "` + table + `"`); err != nil {
		t.Fatalf("empty table %q: %v", table, err)
	}
}

// pageHookReader wraps a Reader and runs hook before a Page call. The embedded
// capabilityReader keeps the preflight trigger and quiescence probes working,
// so the wrapper changes nothing except the injected state change.
type pageHookReader struct {
	contracts.Reader
	capabilityReader
	hook func(contracts.PageRequest)
}

func (r *pageHookReader) Page(ctx context.Context, req contracts.PageRequest) (contracts.Page, error) {
	if r.hook != nil {
		r.hook(req)
	}
	return r.Reader.Page(ctx, req)
}

// emptiedTableApply runs a two-table apply whose contacts table is emptied by
// hook after the dry run. It returns the report and error of the apply.
func emptiedTableApply(t *testing.T, hook func(*Engine, func())) (contracts.ApplyReport, *writerSpy, error) {
	t.Helper()
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	engine, plan, spy := applyFixture(t, cfg)
	hook(engine, func() {
		deleteAllRows(t, cfg.Source, "contacts")
		deleteAllRows(t, cfg.Target, "contacts")
	})
	report, err := engine.Apply(context.Background(), contracts.ApplyRequest{
		Plan: plan, ExplicitApply: true, BatchSize: 1,
	})
	return report, spy, err
}

// TestApplyRefusesTableEmptiedBeforeItsTurn empties the second planned table
// when the run opens its Writer for the first table, which is after the dry run
// proved both tables non-empty and before contacts is visited.
func TestApplyRefusesTableEmptiedBeforeItsTurn(t *testing.T) {
	report, spy, err := emptiedTableApply(t, func(engine *Engine, empty func()) {
		open := engine.openWriter
		emptied := false
		engine.openWriter = func(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error) {
			if !emptied {
				emptied = true
				empty()
			}
			return open(ctx, opts)
		}
	})
	if !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("Apply() error = %v, want ErrConflict", err)
	}
	if report.Complete {
		t.Fatalf("Apply() reported Complete with an emptied planned table: %+v", report)
	}
	// Work committed before the refusal is not hidden: people was written.
	want := []contracts.BatchReceipt{
		{Table: tableRef("people"), Ordinal: 1, Rows: 1},
		{Table: tableRef("people"), Ordinal: 2, Rows: 1},
	}
	if !reflect.DeepEqual(report.Committed, want) {
		t.Fatalf("receipts=%+v, want %+v", report.Committed, want)
	}
	if spy.updates != 2 {
		t.Fatalf("updates=%d, want the two people batches only", spy.updates)
	}
}

// TestApplyRefusesTableEmptiedAfterItsStartCount empties contacts on the first
// apply-page read of contacts, i.e. after the run counted both sides as
// non-empty. The page comparison can not notice because both snapshots are
// emptied, so the end-of-table re-count must refuse.
func TestApplyRefusesTableEmptiedAfterItsStartCount(t *testing.T) {
	report, _, err := emptiedTableApply(t, func(engine *Engine, empty func()) {
		source := engine.source
		capabilities, ok := source.(capabilityReader)
		if !ok {
			t.Fatalf("source reader %T does not expose the capability probes", source)
		}
		emptied := false
		engine.source = &pageHookReader{
			Reader: source, capabilityReader: capabilities,
			// Apply pages with BatchSize=1; the dry run pages with maxPage, so
			// Limit identifies the apply-phase read that must observe the
			// post-count state change.
			hook: func(req contracts.PageRequest) {
				if emptied || req.Table.Table != "contacts" || req.Limit != 1 {
					return
				}
				emptied = true
				empty()
			},
		}
	})
	if !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("Apply() error = %v, want ErrConflict", err)
	}
	if report.Complete {
		t.Fatalf("Apply() reported Complete with an emptied planned table: %+v", report)
	}
}
