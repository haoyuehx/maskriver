package runner

// Regression for #9: an I/O failure inside runner.Scan must fail the whole scan
// instead of yielding a partial report. Scan reads evidence through three seams
// - source.Describe, target.Describe and source.Sample - and every one of them
// must abort the call; a context cancelled after the first scope is complete
// must abort it too. Scan can never open a Writer, so the assertion is that a
// failed scan reports an error, an empty report and an unchanged target tree.

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

var (
	injectedDescribe = errors.New("synthetic injected describe failure")
	injectedSample   = errors.New("synthetic injected sample failure")
)

// scanFaultReader wraps a real Reader and fails only the evidence seam named by
// describeErr or sampleErr. The embedded capabilityReader keeps the internal
// preflight probes working, so only Scan's reads change.
type scanFaultReader struct {
	contracts.Reader
	capabilityReader
	describeErr, sampleErr error
}

func (r *scanFaultReader) Describe(ctx context.Context, ref contracts.TableRef) (contracts.TableSchema, error) {
	if r.describeErr != nil {
		return contracts.TableSchema{}, r.describeErr
	}
	return r.Reader.Describe(ctx, ref)
}

func (r *scanFaultReader) Sample(ctx context.Context, req contracts.SampleRequest) (contracts.Sample, error) {
	if r.sampleErr != nil {
		return contracts.Sample{}, r.sampleErr
	}
	return r.Reader.Sample(ctx, req)
}

// cancelSecondSampleReader cancels the scan while the second declared scope is
// sampled: the first scope is already gathered, so the cancellation really lands
// mid-scan instead of before the first read.
type cancelSecondSampleReader struct {
	contracts.Reader
	capabilityReader
	cancel  context.CancelFunc
	samples atomic.Int64
}

func (r *cancelSecondSampleReader) Sample(ctx context.Context, req contracts.SampleRequest) (contracts.Sample, error) {
	if r.samples.Add(1) == 2 {
		r.cancel()
	}
	return r.Reader.Sample(ctx, req)
}

func capabilityOf(t *testing.T, reader contracts.Reader) capabilityReader {
	t.Helper()
	caps, ok := reader.(capabilityReader)
	if !ok {
		t.Fatal("fixture reader has no internal capabilities")
	}
	return caps
}

// assertScanFailed proves one failed Scan fails closed: the injected error is
// returned unchanged, the report is empty, no Writer factory is called and the
// target fixture tree is byte-identical.
func assertScanFailed(t *testing.T, e *Engine, ctx context.Context, want error) {
	t.Helper()
	before := acceptanceTreeDigest(t, e.cfg.Target)
	opens := 0
	e.openWriter = func(context.Context, contracts.DatabaseOptions) (contracts.Writer, error) {
		opens++
		return nil, contracts.ErrUnsafe
	}
	report, err := e.Scan(ctx, contracts.ScanRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("Scan error=%v, want %v", err, want)
	}
	if report.Coverage != (contracts.Coverage{}) || len(report.Decisions) != 0 || len(report.Issues) != 0 {
		t.Fatalf("failed scan reported evidence: %+v", report)
	}
	if opens != 0 {
		t.Fatalf("failed scan asked for a Writer %d times", opens)
	}
	if after := acceptanceTreeDigest(t, e.cfg.Target); !reflect.DeepEqual(before, after) {
		t.Fatal("failed scan changed the target fixture tree")
	}
}

// TestScanFailsClosedOnEvidenceSeamError injects one failure at each of Scan's
// three read seams. A two-table plan is used so a seam that failed only on a
// later scope would still be caught.
func TestScanFailsClosedOnEvidenceSeamError(t *testing.T) {
	tests := []struct {
		name  string
		side  string
		fault func(t *testing.T, reader contracts.Reader) contracts.Reader
		want  error
	}{
		{
			name: "source describe", side: "source", want: injectedDescribe,
			fault: func(t *testing.T, reader contracts.Reader) contracts.Reader {
				return &scanFaultReader{Reader: reader, capabilityReader: capabilityOf(t, reader), describeErr: injectedDescribe}
			},
		},
		{
			name: "target describe", side: "target", want: injectedDescribe,
			fault: func(t *testing.T, reader contracts.Reader) contracts.Reader {
				return &scanFaultReader{Reader: reader, capabilityReader: capabilityOf(t, reader), describeErr: injectedDescribe}
			},
		},
		{
			name: "source sample", side: "source", want: injectedSample,
			fault: func(t *testing.T, reader contracts.Reader) contracts.Reader {
				return &scanFaultReader{Reader: reader, capabilityReader: capabilityOf(t, reader), sampleErr: injectedSample}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := twoTableFixture(t)
			cfg.Scopes = twoTableScopes()
			engine := scopeEngine(t, cfg)
			if tc.side == "source" {
				engine.source = tc.fault(t, engine.source)
			} else {
				engine.target = tc.fault(t, engine.target)
			}
			assertScanFailed(t, engine, context.Background(), tc.want)
		})
	}
}

// TestScanFailsClosedOnMidScanCancellation cancels the scan while the second
// scope is being sampled. The first scope was gathered successfully, so the
// cancellation must surface instead of the scan returning that partial scope.
func TestScanFailsClosedOnMidScanCancellation(t *testing.T) {
	cfg := twoTableFixture(t)
	cfg.Scopes = twoTableScopes()
	engine := scopeEngine(t, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.source = &cancelSecondSampleReader{
		Reader: engine.source, capabilityReader: capabilityOf(t, engine.source), cancel: cancel,
	}
	assertScanFailed(t, engine, ctx, context.Canceled)
}
