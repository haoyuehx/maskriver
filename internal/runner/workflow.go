// Package runner coordinates the first bounded SQLite workflow. Only Apply
// can construct a Writer; the other methods use read-only database sessions.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"reflect"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/detect"
	"github.com/haoyuehx/maskriver/internal/mask"
	"github.com/haoyuehx/maskriver/internal/verify"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

type Engine struct {
	cfg        config.Runtime
	source     contracts.Reader
	target     contracts.Reader
	key        contracts.Value
	openWriter func(context.Context, contracts.DatabaseOptions) (contracts.Writer, error)
}

// Open requires two independent, existing SQLite files and opens only Readers.
// No Writer, audit file, checkpoint or mapping store is created.
func Open(ctx context.Context, cfg config.Runtime, key contracts.Value) (*Engine, error) {
	sourceOpts, err := cfg.DatabaseOptions(true)
	if err != nil {
		return nil, err
	}
	targetOpts, err := cfg.DatabaseOptions(false)
	if err != nil {
		return nil, err
	}
	source, err := db.OpenReader(ctx, sourceOpts)
	if err != nil {
		return nil, err
	}
	target, err := db.OpenReader(ctx, targetOpts)
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	return &Engine{cfg: cfg, source: source, target: target, key: key, openWriter: db.OpenWriter}, nil
}

func (e *Engine) Close() error {
	if e == nil {
		return nil
	}
	a := e.source.Close()
	b := e.target.Close()
	if a != nil {
		return a
	}
	return b
}

func (e *Engine) table() contracts.TableRef {
	return contracts.TableRef{Database: "maskriver", Schema: "main", Table: e.cfg.Table}
}

func (e *Engine) lookup(ctx context.Context) (contracts.TableSchema, contracts.Column, error) {
	table := e.table()
	src, err := e.source.Describe(ctx, table)
	if err != nil {
		return contracts.TableSchema{}, contracts.Column{}, err
	}
	dst, err := e.target.Describe(ctx, table)
	if err != nil {
		return contracts.TableSchema{}, contracts.Column{}, err
	}
	if !src.Coverage.Columns || !src.Coverage.PrimaryKey || !dst.Coverage.Columns ||
		!dst.Coverage.PrimaryKey || !reflect.DeepEqual(src, dst) || len(src.PrimaryKey) == 0 {
		return contracts.TableSchema{}, contracts.Column{}, contracts.ErrIncomplete
	}
	for _, col := range src.Columns {
		if col.Ref.Column == e.cfg.Column {
			for _, pk := range src.PrimaryKey {
				if pk == e.cfg.Column {
					return contracts.TableSchema{}, contracts.Column{}, contracts.ErrUnsafe
				}
			}
			return src, col, nil
		}
	}
	return contracts.TableSchema{}, contracts.Column{}, contracts.ErrInvalid
}

func (e *Engine) Scan(ctx context.Context, req contracts.ScanRequest) (contracts.ScanReport, error) {
	var report contracts.ScanReport
	if err := ctx.Err(); err != nil {
		return report, err
	}
	col, sample, decision, err := e.sampleColumn(ctx)
	if err != nil {
		return report, err
	}
	report.Decisions = []contracts.Decision{decision}
	report.Coverage = contracts.Coverage{
		Tables: 1, Columns: 1, Rows: int64(len(sample.Values)),
		Complete: scanComplete(sample, decision),
	}
	if decision.Sensitivity != contracts.Sensitive {
		report.Issues = append(report.Issues, contracts.Issue{
			Status: contracts.Warning, Code: contracts.UnresolvedColumn,
			Column: col.Ref, Count: 1,
		})
	}
	return report, nil
}

// sampleColumn is the single place that samples and classifies the scoped
// column, so Scan and Plan can never disagree about the evidence.
func (e *Engine) sampleColumn(ctx context.Context) (contracts.Column, contracts.Sample, contracts.Decision, error) {
	var sample contracts.Sample
	var decision contracts.Decision
	schema, col, err := e.lookup(ctx)
	if err != nil {
		return contracts.Column{}, sample, decision, err
	}
	_ = schema // coverage was checked by lookup
	sample, err = e.source.Sample(ctx, contracts.SampleRequest{
		Column: col.Ref, Limit: e.cfg.SampleLimit,
	})
	if err != nil {
		return contracts.Column{}, sample, decision, err
	}
	decision, err = detect.New().Detect(ctx, contracts.DetectionRequest{
		Column: col, Sample: sample, MinSamples: e.cfg.MinSamples,
		MinRatio: e.cfg.MinRatio, DateOrder: "MDY",
	})
	if err != nil {
		return contracts.Column{}, sample, decision, err
	}
	return col, sample, decision, nil
}

// scanComplete reports whether the scan produced enough evidence to authorize a
// plan. Truncated evidence and any non-Sensitive verdict are both incomplete:
// UNKNOWN is never treated as safe.
func scanComplete(sample contracts.Sample, decision contracts.Decision) bool {
	return !sample.Truncated && decision.Sensitivity == contracts.Sensitive
}

// Plan builds a deterministic digest from the source snapshot, live schema and
// explicit scope. The digest is evidence binding, NOT write authorization.
//
// Plan always scans, even when cfg.Approved is set: approval is an extra intent
// signal, never a substitute for detection evidence. A dedicated reviewed-
// decision store does not exist yet, so an approval cannot be authenticated and
// must not turn an UNKNOWN or truncated scan into an applicable plan.
func (e *Engine) Plan(ctx context.Context) (contracts.Plan, error) {
	var empty contracts.Plan
	schema, col, err := e.lookup(ctx)
	if err != nil {
		return empty, err
	}
	if e.cfg.Strategy.ID == "" || e.cfg.Strategy.Version == "" || e.cfg.KeyID == "" {
		return empty, contracts.ErrInvalid
	}
	if _, err = mask.NewStrategy(e.cfg.Strategy); err != nil {
		return empty, err
	}
	_, sample, decision, err := e.sampleColumn(ctx)
	if err != nil {
		return empty, err
	}
	if sample.Truncated {
		return empty, contracts.ErrIncomplete
	}
	if decision.Sensitivity != contracts.Sensitive {
		return empty, contracts.ErrUnsafe
	}
	digest, err := fileDigest(e.cfg.Source)
	if err != nil {
		return empty, err
	}
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return empty, contracts.ErrInvalid
	}
	schemaDigest := hashString(string(schemaJSON))
	conf := struct{ Table, Column, Strategy, Version, KeyID string }{
		e.cfg.Table, e.cfg.Column, e.cfg.Strategy.ID,
		e.cfg.Strategy.Version, e.cfg.KeyID,
	}
	configJSON, err := json.Marshal(conf)
	if err != nil {
		return empty, contracts.ErrInvalid
	}
	configDigest := hashString(string(configJSON))
	tablePlan := contracts.TablePlan{
		Table: schema.Ref, PrimaryKey: append([]string(nil), schema.PrimaryKey...),
		Columns: []contracts.ColumnPlan{{
			Column: col.Ref, Type: col.Type, Strategy: e.cfg.Strategy,
			Scope:                "sqlite/" + schema.Ref.Table + "/" + col.Ref.Column,
			NormalizationVersion: "typed-lexical-v1", KeyID: e.cfg.KeyID,
		}},
	}
	p := contracts.Plan{
		Revision:         contracts.APIRevision,
		SourceSnapshotID: digest,
		TargetID:         hashString(e.cfg.Target),
		SchemaDigest:     schemaDigest, ConfigDigest: configDigest,
		// ScanComplete is derived from the scan above; a plan is only
		// constructed when that evidence was complete and Sensitive. Unresolved
		// therefore stays empty by construction rather than by assumption.
		ScanComplete: scanComplete(sample, decision),
		Tables:       []contracts.TablePlan{tablePlan},
	}
	p.ID = hashString(p.Revision + p.SourceSnapshotID + p.TargetID +
		p.SchemaDigest + p.ConfigDigest)
	if err := p.ValidateShape(); err != nil {
		return empty, err
	}
	return p, nil
}

func hashString(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", contracts.ErrDatabase
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", contracts.ErrDatabase
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (e *Engine) maskColumn(ctx context.Context, original contracts.Value, col contracts.ColumnPlan) (contracts.Value, error) {
	if e.key.Kind() != contracts.Bytes || len(e.key.Payload()) < 32 {
		return contracts.Value{}, contracts.ErrInvalid
	}
	strategy, err := mask.NewStrategy(col.Strategy)
	if err != nil {
		return contracts.Value{}, err
	}
	masked, err := strategy.Mask(ctx, original, contracts.MaskContext{Plan: col, Key: e.key})
	if err != nil {
		return contracts.Value{}, err
	}
	if !original.IsNull() && masked == original {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	return masked, nil
}

func (e *Engine) Validate(ctx context.Context, req contracts.ValidateRequest) (contracts.ValidationReport, error) {
	if err := req.Plan.ValidateShape(); err != nil {
		return contracts.ValidationReport{}, err
	}
	if err := e.checkPlan(ctx, req.Plan); err != nil {
		return contracts.ValidationReport{}, err
	}
	return verify.New().Validate(ctx, e.source, e.target, req)
}

func (e *Engine) checkPlan(ctx context.Context, supplied contracts.Plan) error {
	plan, err := e.Plan(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(plan, supplied) {
		return contracts.ErrConflict
	}
	return nil
}
