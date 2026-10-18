package runner

import (
	"context"
	"reflect"
	"testing"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// TestPreviewIssuesNoWriteSQLThroughSQLSpy is the driver-level counterpart of
// the Writer-spy Preview assertions: instead of counting Writer calls, it
// observes the SQL text the real SQLite driver receives. Preview must open no
// Writer, so the Engine is assembled from spy-backed Readers only.
func TestPreviewIssuesNoWriteSQLThroughSQLSpy(t *testing.T) {
	rows := []string{"a@example.invalid", "<NULL>", "b@example.invalid"}
	cfg, key := acceptanceFixture(t, rows)
	ctx := context.Background()
	spy := db.NewSQLSpy()
	// Baseline is taken before any connection is opened, so an open-time side
	// effect can not hide inside the measurement.
	before := acceptanceTreeDigest(t, cfg.Source)
	engine := spyEngineReaders(t, cfg, key, spy)

	plan, err := engine.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan, LimitPerTable: 10000})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !report.Coverage.Complete || report.RowsRead != int64(len(rows)) {
		t.Fatalf("preview report=%+v", report)
	}

	if writes := spy.Writes(); len(writes) != 0 {
		t.Fatalf("Preview issued write/transaction intent: %+v", writes)
	}
	selects := 0
	for _, statement := range spy.Statements() {
		if statement.Verb == "SELECT" {
			selects++
		}
	}
	if selects == 0 {
		t.Fatalf("spy observed no SELECT statement; the read assertion would be vacuous: %+v", spy.Statements())
	}
	if err = engine.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if after := acceptanceTreeDigest(t, cfg.Source); !reflect.DeepEqual(before, after) {
		t.Fatal("Preview changed the fixture directory")
	}
}

// spyEngineReaders mirrors Open but builds both Readers through a db.SQLSpy, so
// the read path can be inspected at the driver level.
func spyEngineReaders(t *testing.T, cfg config.Runtime, key contracts.Value, spy *db.SQLSpy) *Engine {
	t.Helper()
	ctx := context.Background()
	sourceOpts, err := cfg.DatabaseOptions(true)
	if err != nil {
		t.Fatal(err)
	}
	targetOpts, err := cfg.DatabaseOptions(false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := spy.OpenReader(ctx, sourceOpts)
	if err != nil {
		t.Fatal(err)
	}
	target, err := spy.OpenReader(ctx, targetOpts)
	if err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	engine := &Engine{cfg: cfg, source: source, target: target, key: key}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}
