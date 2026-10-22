// Package runner coordinates the first bounded SQLite workflow. Only Apply
// can construct a Writer; the other methods use read-only database sessions.
package runner

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash"
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
	// approvals is the read-only review-decision seam Apply requires. It is nil
	// unless injected, and a nil seam refuses Apply instead of allowing writes.
	approvals ApprovalQuery
	// streamingApprovals is the separate streaming (m2) verification seam. It is
	// deliberately independent from approvals so a legacy record can never
	// authorize the streaming path.
	streamingApprovals StreamingApprovalQuery
}

// OpenForApply opens the same readers as Open after letting SQLite recover a
// hot rollback journal on the target. It is used only on explicit apply/resume
// paths; preview, scan and status remain read-only.
func OpenForApply(ctx context.Context, cfg config.Runtime, key contracts.Value) (*Engine, error) {
	targetOpts, err := cfg.DatabaseOptions(false)
	if err != nil {
		return nil, err
	}
	if targetOpts.Dialect == contracts.SQLite {
		if err := db.RecoverSQLiteJournal(ctx, targetOpts); err != nil {
			return nil, err
		}
	}
	return Open(ctx, cfg, key)
}

// Open requires two independent SQLite files or MySQL databases and opens
// only Readers. No Writer, audit file, checkpoint or mapping store is created.
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
	if sourceOpts.Dialect == contracts.MySQL {
		if err := distinctMySQL(ctx, source, target); err != nil {
			_ = source.Close()
			_ = target.Close()
			return nil, err
		}
	}
	return &Engine{cfg: cfg, source: source, target: target, key: key, openWriter: db.OpenWriter}, nil
}

// distinctMySQL compares live server identity and database name. Textual DSN
// comparison in config is only a fast path; aliases of one database must be
// refused here, after connecting and before any plan or write.
func distinctMySQL(ctx context.Context, source, target contracts.Reader) error {
	type identityReader interface {
		ServerIdentity(context.Context) (string, error)
	}
	src, ok := source.(identityReader)
	if !ok {
		return contracts.ErrUnsupported
	}
	dst, ok := target.(identityReader)
	if !ok {
		return contracts.ErrUnsupported
	}
	sourceID, err := src.ServerIdentity(ctx)
	if err != nil {
		return err
	}
	targetID, err := dst.ServerIdentity(ctx)
	if err != nil {
		return err
	}
	if sourceID == targetID {
		return contracts.ErrUnsafe
	}
	return nil
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

// PoolStats reports the database/sql pool statistics of the connections this
// engine actually uses. It issues no SQL and creates no second handle, so an
// observed zero means the real pool is idle rather than that nothing was watched.
// The boolean is false when either connection cannot report statistics.
func (e *Engine) PoolStats() (source, target sql.DBStats, ok bool) {
	if e == nil {
		return sql.DBStats{}, sql.DBStats{}, false
	}
	readerStats, readerOK := db.DBStats(e.source)
	writerStats, writerOK := db.DBStats(e.target)
	return readerStats, writerStats, readerOK && writerOK
}

// tableRef names one scoped table in the frozen reference form. The logical
// schema is shared by both sides; the MySQL adapter maps it to each connection's
// physical database so differently named source/target databases stay comparable.
func tableRef(name string) contracts.TableRef {
	return contracts.TableRef{Database: "maskriver", Schema: "main", Table: name}
}

func (e *Engine) tableRef(name string) contracts.TableRef {
	return contracts.TableRef{Database: "maskriver", Schema: e.cfg.SchemaName(), Table: name}
}

// capabilityReader is the internal db capability set beyond the frozen
// contracts.Reader surface. The m1a-v1 DTOs have no journal-mode or trigger
// metadata, and the preflight must not claim a snapshot or an absence it can
// not prove, so those probes live in internal/db and are required here. A
// Reader without them fails closed with ErrUnsupported.
type capabilityReader interface {
	Quiescent(context.Context) error
	HasTriggers(context.Context, contracts.TableRef) (bool, error)
}

func sourceCapabilities(r contracts.Reader) (capabilityReader, error) {
	caps, ok := r.(capabilityReader)
	if !ok {
		return nil, contracts.ErrUnsupported
	}
	return caps, nil
}

// evidence accumulates the agreed schema of every scoped table and the sampled
// detection evidence of every declared scope, all in declared order.
type evidenceSet struct {
	// tables holds distinct table names in first-appearance order.
	tables  []string
	schemas map[string]contracts.TableSchema
	scopes  []scopeEvidence
}

// scopeEvidence is the complete evidence of one declared scope. Scan and Plan
// both read it, so a report and a plan can never disagree about the evidence.
type scopeEvidence struct {
	table    string
	column   contracts.Column
	sample   contracts.Sample
	decision contracts.Decision
}

// describe requires identical source and target schemas with complete column
// and primary-key metadata, exactly as the single-scope workflow did.
func (e *Engine) describe(ctx context.Context, name string) (contracts.TableSchema, error) {
	table := e.tableRef(name)
	src, err := e.source.Describe(ctx, table)
	if err != nil {
		return contracts.TableSchema{}, err
	}
	dst, err := e.target.Describe(ctx, table)
	if err != nil {
		return contracts.TableSchema{}, err
	}
	if !src.Coverage.Columns || !src.Coverage.PrimaryKey || !dst.Coverage.Columns ||
		!dst.Coverage.PrimaryKey || !reflect.DeepEqual(src, dst) || len(src.PrimaryKey) == 0 {
		return contracts.TableSchema{}, contracts.ErrIncomplete
	}
	return src, nil
}

// gather samples and classifies every declared scope in declared order. Any
// scope that can not produce evidence fails the whole call: neither a scan
// report nor a plan may silently describe a subset of the declared scopes.
func (e *Engine) gather(ctx context.Context) (evidenceSet, error) {
	// Reserved outbox tables are audit infrastructure, never business scopes.
	for _, scope := range e.cfg.ResolvedScopes() {
		if reservedOutboxName(scope.Table) {
			return evidenceSet{}, contracts.ErrUnsafe
		}
	}
	set := evidenceSet{schemas: make(map[string]contracts.TableSchema)}
	for _, scope := range e.cfg.ResolvedScopes() {
		if err := ctx.Err(); err != nil {
			return evidenceSet{}, err
		}
		schema, ok := set.schemas[scope.Table]
		if !ok {
			var err error
			if schema, err = e.describe(ctx, scope.Table); err != nil {
				return evidenceSet{}, err
			}
			set.schemas[scope.Table] = schema
			set.tables = append(set.tables, scope.Table)
		}
		col, ok := columnByName(schema, scope.Column)
		if !ok {
			return evidenceSet{}, contracts.ErrInvalid
		}
		for _, key := range schema.PrimaryKey {
			if key == scope.Column {
				return evidenceSet{}, contracts.ErrUnsafe
			}
		}
		sample, err := e.source.Sample(ctx, contracts.SampleRequest{
			Column: col.Ref, Limit: e.cfg.SampleLimit,
		})
		if err != nil {
			return evidenceSet{}, err
		}
		decision, err := detect.New().Detect(ctx, contracts.DetectionRequest{
			Column: col, Sample: sample, MinSamples: e.cfg.MinSamples,
			MinRatio: e.cfg.MinRatio, DateOrder: "MDY",
		})
		if err != nil {
			return evidenceSet{}, err
		}
		set.scopes = append(set.scopes, scopeEvidence{
			table: scope.Table, column: col, sample: sample, decision: decision,
		})
	}
	if len(set.scopes) == 0 {
		return evidenceSet{}, contracts.ErrInvalid
	}
	return set, nil
}

func columnByName(schema contracts.TableSchema, name string) (contracts.Column, bool) {
	for _, candidate := range schema.Columns {
		if candidate.Ref.Column == name {
			return candidate, true
		}
	}
	return contracts.Column{}, false
}

func (e *Engine) Scan(ctx context.Context, req contracts.ScanRequest) (contracts.ScanReport, error) {
	var report contracts.ScanReport
	if ctx == nil {
		return report, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	set, err := e.gather(ctx)
	if err != nil {
		return report, err
	}
	complete := true
	var rows int64
	for _, scope := range set.scopes {
		report.Decisions = append(report.Decisions, scope.decision)
		rows += int64(len(scope.sample.Values))
		if !scanComplete(scope.sample, scope.decision) {
			complete = false
		}
		// One UNKNOWN/NotSensitive scope is one unresolved column, named by its
		// own reference so callers can locate the refusal without row data.
		if scope.decision.Sensitivity != contracts.Sensitive {
			report.Issues = append(report.Issues, contracts.Issue{
				Status: contracts.Warning, Code: contracts.UnresolvedColumn,
				Column: scope.column.Ref, Count: 1,
			})
		}
	}
	// Coverage.Rows follows the existing single-scope meaning: the number of
	// sampled values examined, summed across scopes. It is not a physical row
	// count, so it can exceed the table's row total when a table is sampled
	// once per column.
	report.Coverage = contracts.Coverage{
		Tables: int64(len(set.tables)), Columns: int64(len(set.scopes)),
		Rows: rows, Complete: complete,
	}
	return report, nil
}

// scanComplete reports whether the scan produced enough evidence to authorize a
// plan. Truncated evidence and any non-Sensitive verdict are both incomplete:
// UNKNOWN is never treated as safe.
func scanComplete(sample contracts.Sample, decision contracts.Decision) bool {
	return !sample.Truncated && decision.Sensitivity == contracts.Sensitive
}

// Plan builds a deterministic digest from the source snapshot, the live schemas
// and the declared scope list. The digest is evidence binding, NOT write
// authorization.
//
// Snapshot identity: SourceSnapshotID is the SHA-256 of the whole main .db
// file, so it changes whenever that file changes. That is only an identity of
// the snapshot when the main file is the whole committed state, which is why a
// source whose journal mode can keep committed data in a sidecar (WAL) is
// refused before the digest is computed; see db.Quiescent. It is still not a
// proof against concurrent external writers on a rollback-journal database, so
// the source must be a quiescent isolated copy. TargetID is the digest of the
// target PATH only and says nothing about the target's contents.
//
// Plan always scans every scope, even when cfg.Approved is set: approval is an
// extra intent signal, never a substitute for detection evidence. A stored
// reviewer decision authenticates a plan that was already built from complete
// Sensitive evidence; it is not detection evidence, so this package never feeds
// a decision into detection and an UNKNOWN or truncated scan still yields no
// plan for any decision to authenticate.
func (e *Engine) Plan(ctx context.Context) (contracts.Plan, error) {
	var empty contracts.Plan
	if ctx == nil {
		return empty, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if e.cfg.Strategy.ID == "" || e.cfg.Strategy.Version == "" || e.cfg.KeyID == "" {
		return empty, contracts.ErrInvalid
	}
	if _, err := mask.NewStrategy(e.cfg.Strategy); err != nil {
		return empty, err
	}
	set, err := e.gather(ctx)
	if err != nil {
		return empty, err
	}
	// Every scope must independently carry complete, Sensitive evidence; one
	// weak scope refuses the whole plan instead of publishing a partial one.
	// The refusal category is decided from the whole set, not from whichever
	// scope happens to come first, so it cannot depend on declaration order:
	// an evidence gap (truncation) outranks an unresolved verdict.
	allComplete := true
	allSensitive := true
	for _, scope := range set.scopes {
		if scope.sample.Truncated {
			allComplete = false
		}
		if scope.decision.Sensitivity != contracts.Sensitive {
			allSensitive = false
		}
	}
	if !allComplete {
		return empty, contracts.ErrIncomplete
	}
	if !allSensitive {
		return empty, contracts.ErrUnsafe
	}
	// A source that is not provably quiescent has no snapshot identity: the
	// main-file digest could stay constant while committed rows live in a WAL
	// sidecar, so the digest would not describe the data this plan read.
	if err := e.requireQuiescentSnapshot(ctx); err != nil {
		return empty, err
	}
	var digest string
	if e.cfg.DialectName() == contracts.MySQL {
		digest, err = e.mysqlSnapshotDigest(ctx, set)
	} else {
		digest, err = fileDigest(e.cfg.Source)
	}
	if err != nil {
		return empty, err
	}
	schemas := make([]contracts.TableSchema, 0, len(set.tables))
	tablePlans := make([]contracts.TablePlan, 0, len(set.tables))
	for _, name := range set.tables {
		schema := set.schemas[name]
		schemas = append(schemas, schema)
		tablePlan := contracts.TablePlan{
			Table: schema.Ref, PrimaryKey: append([]string(nil), schema.PrimaryKey...),
		}
		for _, scope := range set.scopes {
			if scope.table != name {
				continue
			}
			tablePlan.Columns = append(tablePlan.Columns, contracts.ColumnPlan{
				Column: scope.column.Ref, Type: scope.column.Type, Strategy: e.cfg.Strategy,
				Scope:                dialectLabel(e.cfg.DialectName()) + "/" + name + "/" + scope.column.Ref.Column,
				NormalizationVersion: "typed-lexical-v1", KeyID: e.cfg.KeyID,
			})
		}
		tablePlans = append(tablePlans, tablePlan)
	}
	// The key is the number of tables, not scopes: one table keeps the original
	// single-object encoding so existing plan digests stay stable across the
	// multi-table change, while several tables bind the ordered slice. A second
	// column on the same table therefore keeps the same schema digest, which is
	// correct because the digest describes the schema, not the selected columns;
	// the column selection is bound by ConfigDigest.
	var schemaJSON []byte
	if len(schemas) == 1 {
		schemaJSON, err = json.Marshal(schemas[0])
	} else {
		schemaJSON, err = json.Marshal(schemas)
	}
	if err != nil {
		return empty, contracts.ErrInvalid
	}
	schemaDigest := hashString(string(schemaJSON))
	// The ordered scope list is part of the digest, so the same scopes in a
	// different order bind a different plan ID. A single scope keeps the
	// original payload so existing plan IDs stay stable for unchanged configs.
	type planScope struct{ Table, Column string }
	ordered := make([]planScope, 0, len(set.scopes))
	for _, scope := range set.scopes {
		ordered = append(ordered, planScope{scope.table, scope.column.Ref.Column})
	}
	var configJSON []byte
	if len(ordered) == 1 {
		legacy := struct{ Table, Column, Strategy, Version, KeyID string }{
			ordered[0].Table, ordered[0].Column, e.cfg.Strategy.ID,
			e.cfg.Strategy.Version, e.cfg.KeyID,
		}
		configJSON, err = json.Marshal(legacy)
	} else {
		conf := struct {
			Scopes         []planScope
			Strategy       string
			Version, KeyID string
		}{
			ordered, e.cfg.Strategy.ID, e.cfg.Strategy.Version, e.cfg.KeyID,
		}
		configJSON, err = json.Marshal(conf)
	}
	if err != nil {
		return empty, contracts.ErrInvalid
	}
	configDigest := hashString(string(configJSON))
	targetIdentity, err := e.cfg.TargetIdentity()
	if err != nil {
		return empty, err
	}
	p := contracts.Plan{
		Revision:         contracts.APIRevision,
		SourceSnapshotID: digest,
		// TargetID binds the target PATH, not the target's initial contents: it
		// proves which file Apply would write and nothing about that file's
		// state. Target equivalence is proven by the mandatory full dry run, not
		// by this digest.
		TargetID:     hashString(targetIdentity),
		SchemaDigest: schemaDigest, ConfigDigest: configDigest,
		// Derived from the gates above, not assumed: reaching this point means
		// every declared scope was untruncated and Sensitive.
		ScanComplete: allComplete && allSensitive,
		Tables:       tablePlans,
	}
	p.ID = hashString(p.Revision + p.SourceSnapshotID + p.TargetID +
		p.SchemaDigest + p.ConfigDigest)
	if err := p.ValidateShape(); err != nil {
		return empty, err
	}
	return p, nil
}

// requireQuiescentSnapshot refuses a source whose journal mode can keep
// committed data outside the main .db file.
func (e *Engine) requireQuiescentSnapshot(ctx context.Context) error {
	if e.cfg.DialectName() != contracts.SQLite {
		// MySQL identity is a content digest over the planned tables, not a
		// single-file snapshot, so the SQLite journal-mode probe does not apply.
		return nil
	}
	caps, err := sourceCapabilities(e.source)
	if err != nil {
		return err
	}
	return caps.Quiescent(ctx)
}

func dialectLabel(d contracts.Dialect) string {
	if d == contracts.MySQL {
		return "mysql"
	}
	return "sqlite"
}

// mysqlSnapshotDigest hashes the complete ordered content of every planned
// table (all columns, not only the masked ones) on the source. It is
// deliberately a content identity: a MySQL server has no one file whose bytes
// describe the committed state. Cost is proportional to the planned tables'
// size; this is a bounded-workflow CLI, not a streaming snapshot protocol.
func (e *Engine) mysqlSnapshotDigest(ctx context.Context, set evidenceSet) (string, error) {
	h := sha256.New()
	for _, name := range set.tables {
		schema := set.schemas[name]
		if len(schema.PrimaryKey) == 0 {
			return "", contracts.ErrUnsupported
		}
		columns := make([]string, len(schema.Columns))
		for i, col := range schema.Columns {
			columns[i] = col.Ref.Column
		}
		writeField(h, "table", name)
		for _, col := range schema.Columns {
			writeField(h, "column", col.Ref.Column)
			h.Write([]byte{byte(col.Type.Kind)})
			writeField(h, "native", col.Type.Native)
		}
		var after []contracts.Value
		for {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			page, err := e.source.Page(ctx, contracts.PageRequest{
				Table: e.tableRef(name), Columns: columns,
				KeyColumns: schema.PrimaryKey, After: after, Limit: 256,
			})
			if err != nil {
				return "", err
			}
			if len(page.Rows) > 256 || (!page.Done && len(page.Rows) == 0) {
				return "", contracts.ErrIncomplete
			}
			for _, row := range page.Rows {
				writeField(h, "row", "")
				for _, col := range schema.Columns {
					value, ok := row[col.Ref.Column]
					if !ok {
						return "", contracts.ErrIncomplete
					}
					writeValue(h, value)
				}
			}
			if page.Done {
				break
			}
			if len(page.Next) != len(schema.PrimaryKey) {
				return "", contracts.ErrIncomplete
			}
			after = append([]contracts.Value(nil), page.Next...)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeField(h hash.Hash, label, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(label)))
	h.Write(size[:])
	h.Write([]byte(label))
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	h.Write(size[:])
	h.Write([]byte(value))
}

func writeValue(h hash.Hash, value contracts.Value) {
	h.Write([]byte{byte(value.Kind())})
	if value.IsNull() {
		h.Write([]byte{0})
		return
	}
	h.Write([]byte{1})
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value.Payload())))
	h.Write(size[:])
	h.Write([]byte(value.Payload()))
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

// maskColumn applies one column plan to one original value and enforces the
// replacement contract verify re-checks after a write: a non-NULL sensitive
// value must become a different, non-NULL value. The dry run (Preview and
// Apply) runs this over every planned row before a Writer exists, so a plan
// whose output could never pass validation is refused before the first write
// instead of being discovered by a later validate.
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
	if !original.IsNull() && masked.IsNull() {
		// verify reports a NULL target value for a non-NULL source value as
		// conflicting evidence, so replacing a sensitive value with NULL is a
		// write no later strict validation could pass. The "null" strategy is
		// therefore not applicable to a sensitive column, even when the column
		// is nullable and the strategy itself is willing to emit NULL.
		return contracts.Value{}, contracts.ErrUnsafe
	}
	if !original.IsNull() && masked == original {
		return contracts.Value{}, contracts.ErrUnsafe
	}
	return masked, nil
}

// Validate re-checks the supplied plan against the live snapshots and then runs
// the read-only verifier over every planned table. Validate never writes, so the
// write-only gates Apply runs do not apply here; all constraint logic (schema,
// key sets, PK/FK/UNIQUE/CHECK/index gates, row counts and unchanged values)
// stays in verify and is not duplicated here.
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
