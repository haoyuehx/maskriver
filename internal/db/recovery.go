package db

// SQLite local transactional recovery (m2-recovery-v1).
//
// This file is additive to the Issue #13 outbox. The manifest and append-only
// progress log live in the same database as the business rows and the outbox
// receipts, so a batch can commit:
//
//   business UPDATE + receipt INSERT + progress INSERT + manifest CAS UPDATE
//
// in exactly one SQLite transaction. The progress log stores table ordinals,
// row counts and sequence numbers only; it never stores a plaintext primary key
// or a source/replacement value. Recovery reconstructs the keyset cursor by
// re-scanning the immutable source.

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"strings"
	"time"

	"github.com/haoyuehx/maskriver/internal/testhooks"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// ErrTargetIdentity is returned when a stable target file identity can not be
// established on this platform. Recovery itself is Linux-only.
var ErrTargetIdentity = errors.New("maskriver: target identity unavailable")

// RecoveryProtocolRevision identifies the local transactional recovery
// protocol. It is independent of (and layered above) m2-stream-v1.
const RecoveryProtocolRevision = "m2-recovery-v1"

// RecoverySchemaVersion is the only recovery schema this package understands.
const RecoverySchemaVersion = "maskriver-recovery-v1"

const (
	recoverySchemaTable   = "maskriver_recovery_schema"
	RecoveryManifestTable = "maskriver_recovery_manifest"
	RecoveryProgressTable = "maskriver_recovery_progress"
)

// RecoveryTables returns the reserved recovery table names in a stable order.
func RecoveryTables() []string {
	return []string{recoverySchemaTable, RecoveryManifestTable, RecoveryProgressTable}
}

//go:embed recovery_schema_v1.sql
var recoverySchemaSQL string

// RecoveryState is the manifest state machine value. It is informational: the
// authoritative progress is always re-derived from the committed progress log.
type RecoveryState string

const (
	RecoveryStateNew              RecoveryState = "new"
	RecoveryStateRunning          RecoveryState = "running"
	RecoveryStateRecoverable      RecoveryState = "recoverable"
	RecoveryStateRecoveryRequired RecoveryState = "recovery_required"
	RecoveryStateCommitUnknown    RecoveryState = "commit_unknown"
	RecoveryStateConflict         RecoveryState = "conflict"
	RecoveryStateValidationFailed RecoveryState = "validation_failed"
	RecoveryStateComplete         RecoveryState = "complete"
	RecoveryStateAborted          RecoveryState = "aborted"
)

// ValidationState records whether the strict post-apply validation ran.
type ValidationState string

const (
	ValidationNone   ValidationState = "none"
	ValidationPassed ValidationState = "passed"
	ValidationFailed ValidationState = "failed"
)

func validRecoveryState(s RecoveryState) bool {
	switch s {
	case RecoveryStateNew, RecoveryStateRunning, RecoveryStateRecoverable,
		RecoveryStateRecoveryRequired, RecoveryStateCommitUnknown,
		RecoveryStateConflict, RecoveryStateValidationFailed,
		RecoveryStateComplete, RecoveryStateAborted:
		return true
	default:
		return false
	}
}

func validValidationState(s ValidationState) bool {
	return s == ValidationNone || s == ValidationPassed || s == ValidationFailed
}

// terminalForWrites reports states that must never accept a new business write.
func terminalForWrites(s RecoveryState) bool {
	switch s {
	case RecoveryStateConflict, RecoveryStateAborted, RecoveryStateComplete,
		RecoveryStateValidationFailed:
		return true
	default:
		return false
	}
}

// RecoveryManifest is the non-sensitive run manifest. It carries identifiers,
// digests, counts, states and timestamps only.
type RecoveryManifest struct {
	RunID                string
	Protocol             string
	StreamingProtocol    string
	ApprovalBinding      string
	PlanID               string
	PlanContentDigest    string
	ScopeDigest          string
	SourceSnapshotDigest string
	TargetID             string
	TargetIdentity       string
	SchemaDigest         string
	ConfigDigest         string
	StrategyID           string
	StrategyVersion      string
	NormalizationVersion string
	KeyID                string
	KeyCheck             string
	SettingsDigest       string
	State                RecoveryState
	BusinessComplete     bool
	Validation           ValidationState
	CheckpointSequence   int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// RecoveryProgress is one committed progress event. Kind is batch, progress or
// boundary. A batch always affected at least one row; progress and boundary
// events never claim affected rows.
type RecoveryProgress struct {
	RunID            string
	Sequence         int64
	Kind             string
	TableOrdinal     int
	RowsProcessed    int64
	RowsCommitted    int64
	CommittedBatches int64
	BatchID          string
	AffectedRows     int64
	BusinessComplete bool
	CreatedAt        time.Time
}

// Recovery is the recovery-aware view of an open outbox. It reuses the pinned
// outbox connection and its verified session/schema, and adds the recovery
// tables. Reads use the pool; writes use the same pinned connection.
type Recovery struct {
	*Outbox
}

// recoveryShape is the exact schema shape written by recovery_schema_v1.sql.
var recoveryShape = []struct {
	table   string
	columns []outboxColumn
}{
	{recoverySchemaTable, []outboxColumn{
		{"version", "TEXT", 1, 1},
		{"created_at", "TEXT", 1, 0},
	}},
	{RecoveryManifestTable, []outboxColumn{
		{"run_id", "TEXT", 1, 1},
		{"protocol", "TEXT", 1, 0},
		{"streaming_protocol", "TEXT", 1, 0},
		{"approval_binding", "TEXT", 1, 0},
		{"plan_id", "TEXT", 1, 0},
		{"plan_content_digest", "TEXT", 1, 0},
		{"scope_digest", "TEXT", 1, 0},
		{"source_snapshot_digest", "TEXT", 1, 0},
		{"target_id", "TEXT", 1, 0},
		{"target_identity", "TEXT", 1, 0},
		{"schema_digest", "TEXT", 1, 0},
		{"config_digest", "TEXT", 1, 0},
		{"strategy_id", "TEXT", 1, 0},
		{"strategy_version", "TEXT", 1, 0},
		{"normalization_version", "TEXT", 1, 0},
		{"key_id", "TEXT", 1, 0},
		{"key_check", "TEXT", 1, 0},
		{"settings_digest", "TEXT", 1, 0},
		{"state", "TEXT", 1, 0},
		{"business_complete", "INTEGER", 1, 0},
		{"validation_state", "TEXT", 1, 0},
		{"checkpoint_sequence", "INTEGER", 1, 0},
		{"created_at", "TEXT", 1, 0},
		{"updated_at", "TEXT", 1, 0},
	}},
	{RecoveryProgressTable, []outboxColumn{
		{"run_id", "TEXT", 1, 1},
		{"checkpoint_sequence", "INTEGER", 1, 2},
		{"kind", "TEXT", 1, 0},
		{"table_ordinal", "INTEGER", 1, 0},
		{"rows_processed", "INTEGER", 1, 0},
		{"rows_committed", "INTEGER", 1, 0},
		{"committed_batches", "INTEGER", 1, 0},
		{"batch_id", "TEXT", 1, 0},
		{"affected_rows", "INTEGER", 1, 0},
		{"business_complete", "INTEGER", 1, 0},
		{"created_at", "TEXT", 1, 0},
	}},
}

// InstallRecoverySchema creates the recovery tables in one explicit transaction
// after verifying the exact maskriver-outbox-v1 schema. Equal version and exact
// shape is a no-op; anything else is refused. It never drops or alters data.
func InstallRecoverySchema(ctx context.Context, opts OutboxOptions) error {
	if ctx == nil || opts.Dialect != contracts.SQLite {
		return contracts.ErrUnsupported
	}
	a, err := open(ctx, opts.DatabaseOptions, false, nil)
	if err != nil {
		return err
	}
	defer a.Close()
	if _, _, err := outboxPathPreflight(opts.DSN.Payload()); err != nil {
		return err
	}
	// Recovery is only meaningful on top of the complete outbox schema.
	version, err := schemaVersionOn(ctx, a.db, outboxSchemaTable)
	if errors.Is(err, contracts.ErrNotFound) {
		return contracts.ErrUnsupported
	}
	if err != nil {
		return err
	}
	if version != OutboxSchemaVersion {
		return contracts.ErrUnsupported
	}
	for _, table := range outboxShape {
		if err := verifyOutboxTableShape(ctx, a.db, table); err != nil {
			return err
		}
	}
	existing, err := schemaVersionOn(ctx, a.db, recoverySchemaTable)
	switch {
	case err == nil:
		if existing != RecoverySchemaVersion {
			return contracts.ErrUnsupported
		}
		for _, table := range recoveryShape {
			if err := verifyOutboxTableShape(ctx, a.db, table); err != nil {
				return err
			}
		}
		return nil
	case errors.Is(err, contracts.ErrNotFound):
		// fall through to create
	default:
		return err
	}
	for _, name := range RecoveryTables() {
		var found string
		err := a.db.QueryRowContext(ctx,
			"SELECT name FROM main.sqlite_schema WHERE type='table' AND name=?", name).Scan(&found)
		if err == nil {
			// A partially created recovery schema is a conflict, never repaired.
			return contracts.ErrConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return dbError(err)
		}
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return dbError(err)
	}
	defer tx.Rollback()
	for _, statement := range schemaDDL(recoverySchemaSQL) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return dbError(err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO main."+recoverySchemaTable+"(version,created_at) VALUES(?,?)",
		RecoverySchemaVersion, outboxTimestamp()); err != nil {
		return dbError(err)
	}
	if err := tx.Commit(); err != nil {
		return contracts.ErrDatabase
	}
	return nil
}

// schemaQueryer is satisfied by *sql.DB and *sql.Conn.
type schemaQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// schemaVersionOn reads a single schema-version marker, distinguishing a missing
// table (ErrNotFound) from an unreadable or ambiguous one without inspecting
// driver error text.
func schemaVersionOn(ctx context.Context, q schemaQueryer, table string) (string, error) {
	var name string
	err := q.QueryRowContext(ctx,
		"SELECT name FROM main.sqlite_schema WHERE type='table' AND name=?", table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", contracts.ErrNotFound
	}
	if err != nil {
		return "", dbError(err)
	}
	var version string
	if err := q.QueryRowContext(ctx, "SELECT version FROM main."+table).Scan(&version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", contracts.ErrConflict
		}
		return "", dbError(err)
	}
	return version, nil
}

// RecoveryAvailable reports whether the recovery schema exists. A legacy
// Issue #13 database has the outbox but no recovery schema and is never
// silently upgraded.
func (o *Outbox) RecoveryAvailable(ctx context.Context) (bool, error) {
	if err := o.check(ctx); err != nil {
		return false, err
	}
	var name string
	err := o.conn.QueryRowContext(ctx,
		"SELECT name FROM main.sqlite_schema WHERE type='table' AND name=?", recoverySchemaTable).Scan(&name)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, dbError(err)
	}
	// No marker is only "legacy" when no recovery table exists at all. A
	// partial schema fails closed instead of silently falling back.
	for _, table := range RecoveryTables() {
		if table == recoverySchemaTable {
			continue
		}
		var found string
		err := o.conn.QueryRowContext(ctx,
			"SELECT name FROM main.sqlite_schema WHERE type='table' AND name=?", table).Scan(&found)
		if err == nil {
			return false, contracts.ErrUnsupported
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, dbError(err)
		}
	}
	return false, nil
}

// OpenRecovery verifies and returns the recovery view of this outbox. It does
// not install anything.
func (o *Outbox) OpenRecovery(ctx context.Context) (*Recovery, error) {
	if err := o.check(ctx); err != nil {
		return nil, err
	}
	version, err := schemaVersionOn(ctx, o.conn, recoverySchemaTable)
	if errors.Is(err, contracts.ErrNotFound) {
		return nil, contracts.ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	if version != RecoverySchemaVersion {
		return nil, contracts.ErrUnsupported
	}
	for _, table := range recoveryShape {
		if err := verifyOutboxTableShape(ctx, o.conn, table); err != nil {
			return nil, err
		}
	}
	return &Recovery{Outbox: o}, nil
}

// CreateManifest records the run identity in its own autocommit statement. It
// is never evidence that a business batch committed.
func (r *Recovery) CreateManifest(ctx context.Context, m RecoveryManifest) error {
	if r == nil || r.Outbox == nil {
		return contracts.ErrInvalid
	}
	if err := r.check(ctx); err != nil {
		return err
	}
	if !contracts.ValidRunID(m.RunID) || m.Protocol != RecoveryProtocolRevision ||
		m.StreamingProtocol != contracts.StreamingProtocolRevision ||
		m.State != RecoveryStateNew || m.BusinessComplete || m.Validation != ValidationNone ||
		m.CheckpointSequence != 0 {
		return contracts.ErrInvalid
	}
	for _, label := range []struct {
		value string
		max   int
	}{
		{m.ApprovalBinding, 64}, {m.PlanID, 128}, {m.PlanContentDigest, 128},
		{m.ScopeDigest, 128}, {m.SourceSnapshotDigest, 128}, {m.TargetID, 128},
		{m.TargetIdentity, 128}, {m.SchemaDigest, 128}, {m.ConfigDigest, 128},
		{m.StrategyID, 64}, {m.StrategyVersion, 32}, {m.NormalizationVersion, 64},
		{m.KeyID, 128}, {m.KeyCheck, 64}, {m.SettingsDigest, 128},
	} {
		if !boundedLabel(label.value, label.max) {
			return contracts.ErrInvalid
		}
	}
	now := outboxTimestamp()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return contracts.ErrClosed
	}
	_, err := r.conn.ExecContext(ctx,
		"INSERT INTO main."+RecoveryManifestTable+
			"(run_id,protocol,streaming_protocol,approval_binding,plan_id,plan_content_digest,"+
			"scope_digest,source_snapshot_digest,target_id,target_identity,schema_digest,config_digest,"+
			"strategy_id,strategy_version,normalization_version,key_id,key_check,settings_digest,state,"+
			"business_complete,validation_state,checkpoint_sequence,created_at,updated_at) "+
			"VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
		m.RunID, m.Protocol, m.StreamingProtocol, m.ApprovalBinding, m.PlanID,
		m.PlanContentDigest, m.ScopeDigest, m.SourceSnapshotDigest, m.TargetID,
		m.TargetIdentity, m.SchemaDigest, m.ConfigDigest, m.StrategyID,
		m.StrategyVersion, m.NormalizationVersion, m.KeyID, m.KeyCheck, m.SettingsDigest,
		string(m.State), 0, string(m.Validation), 0, now, now)
	if err != nil {
		return classifyOutboxError(err)
	}
	return nil
}

// Manifest reads the non-sensitive run manifest.
func (r *Recovery) Manifest(ctx context.Context, runID string) (RecoveryManifest, error) {
	if r == nil || r.Outbox == nil {
		return RecoveryManifest{}, contracts.ErrInvalid
	}
	if err := r.check(ctx); err != nil {
		return RecoveryManifest{}, err
	}
	if !contracts.ValidRunID(runID) {
		return RecoveryManifest{}, contracts.ErrInvalid
	}
	return readManifest(ctx, r.adapter.db, runID)
}

// readManifest is shared by the writable recovery store and the read-only
// status reader.
func readManifest(ctx context.Context, q schemaQueryer, runID string) (RecoveryManifest, error) {
	var m RecoveryManifest
	var state, validation string
	var business int
	var created, updated string
	err := q.QueryRowContext(ctx,
		"SELECT run_id,protocol,streaming_protocol,approval_binding,plan_id,plan_content_digest,"+
			"scope_digest,source_snapshot_digest,target_id,target_identity,schema_digest,config_digest,"+
			"strategy_id,strategy_version,normalization_version,key_id,key_check,settings_digest,state,"+
			"business_complete,validation_state,checkpoint_sequence,created_at,updated_at "+
			"FROM main."+RecoveryManifestTable+" WHERE run_id=?", runID).Scan(
		&m.RunID, &m.Protocol, &m.StreamingProtocol, &m.ApprovalBinding, &m.PlanID,
		&m.PlanContentDigest, &m.ScopeDigest, &m.SourceSnapshotDigest, &m.TargetID,
		&m.TargetIdentity, &m.SchemaDigest, &m.ConfigDigest, &m.StrategyID,
		&m.StrategyVersion, &m.NormalizationVersion, &m.KeyID, &m.KeyCheck, &m.SettingsDigest,
		&state, &business, &validation, &m.CheckpointSequence, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return RecoveryManifest{}, contracts.ErrNotFound
	}
	if err != nil {
		return RecoveryManifest{}, dbError(err)
	}
	m.State = RecoveryState(state)
	m.Validation = ValidationState(validation)
	m.BusinessComplete = business == 1
	if !validRecoveryState(m.State) || !validValidationState(m.Validation) ||
		m.Protocol != RecoveryProtocolRevision ||
		m.StreamingProtocol != contracts.StreamingProtocolRevision {
		return RecoveryManifest{}, contracts.ErrUnsupported
	}
	m.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return RecoveryManifest{}, contracts.ErrDatabase
	}
	m.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return RecoveryManifest{}, contracts.ErrDatabase
	}
	return m, nil
}

// ProgressPage reads committed progress strictly after afterSequence, bounded by
// limit. done is true when no further row exists.
func (r *Recovery) ProgressPage(ctx context.Context, runID string, afterSequence int64, limit int) ([]RecoveryProgress, bool, error) {
	if r == nil || r.Outbox == nil {
		return nil, false, contracts.ErrInvalid
	}
	if err := r.check(ctx); err != nil {
		return nil, false, err
	}
	if !contracts.ValidRunID(runID) || afterSequence < 0 || limit < 1 || limit > r.maxPage {
		return nil, false, contracts.ErrInvalid
	}
	return readProgressPage(ctx, r.adapter.db, runID, afterSequence, limit)
}

// readProgressPage is shared by the writable recovery store and the read-only
// status reader.
func readProgressPage(ctx context.Context, q schemaQueryer, runID string, afterSequence int64, limit int) ([]RecoveryProgress, bool, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT run_id,checkpoint_sequence,kind,table_ordinal,rows_processed,rows_committed,"+
			"committed_batches,batch_id,affected_rows,business_complete,created_at "+
			"FROM main."+RecoveryProgressTable+
			" WHERE run_id=? AND checkpoint_sequence>? ORDER BY checkpoint_sequence LIMIT ?",
		runID, afterSequence, limit+1)
	if err != nil {
		return nil, false, dbError(err)
	}
	defer rows.Close()
	page := make([]RecoveryProgress, 0, limit+1)
	for rows.Next() {
		var p RecoveryProgress
		var business int
		var created string
		if err := rows.Scan(&p.RunID, &p.Sequence, &p.Kind, &p.TableOrdinal,
			&p.RowsProcessed, &p.RowsCommitted, &p.CommittedBatches, &p.BatchID,
			&p.AffectedRows, &business, &created); err != nil {
			return nil, false, dbError(err)
		}
		p.BusinessComplete = business == 1
		p.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, false, contracts.ErrDatabase
		}
		page = append(page, p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, dbError(err)
	}
	if len(page) > limit {
		return page[:limit], false, nil
	}
	return page, true, nil
}

// BeginRecoveryBatch starts one business transaction whose Commit inserts the
// receipt, appends the progress event and advances the manifest checkpoint with
// a compare-and-swap, all before the single Commit call.
func (r *Recovery) BeginRecoveryBatch(ctx context.Context, runID string, plan contracts.TablePlan,
	receipt contracts.StreamingBatchReceipt, progress RecoveryProgress) (contracts.WriteTx, error) {
	if r == nil || r.Outbox == nil {
		return nil, contracts.ErrInvalid
	}
	if progress.RunID != runID || progress.Kind != "batch" ||
		progress.BatchID != receipt.BatchID || progress.AffectedRows != receipt.Rows ||
		progress.Sequence < 1 || progress.TableOrdinal < 0 ||
		progress.RowsProcessed < 0 || progress.RowsCommitted < 0 ||
		progress.CommittedBatches < 0 || progress.BusinessComplete {
		return nil, contracts.ErrInvalid
	}
	return r.beginBatch(ctx, runID, plan, receipt, func(ctx context.Context, tx *sql.Tx) error {
		return updateProgressInTx(ctx, tx, runID, progress)
	})
}

// CommitProgress commits a no-op page or a table boundary. It writes only
// recovery metadata; it never pretends that business rows were changed.
func (r *Recovery) CommitProgress(ctx context.Context, runID string, progress RecoveryProgress) error {
	if r == nil || r.Outbox == nil {
		return contracts.ErrInvalid
	}
	if progress.RunID != runID || (progress.Kind != "progress" && progress.Kind != "boundary") ||
		progress.BatchID != "" || progress.AffectedRows != 0 ||
		progress.Sequence < 1 || progress.TableOrdinal < 0 ||
		progress.RowsProcessed < 0 || progress.RowsCommitted < 0 ||
		progress.CommittedBatches < 0 {
		return contracts.ErrInvalid
	}
	if err := r.check(ctx); err != nil {
		return err
	}
	if err := r.verifyFileIdentity(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return contracts.ErrClosed
	}
	if r.active != nil {
		return contracts.ErrUnsafe
	}
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return dbError(err)
	}
	if err := updateProgressInTx(ctx, tx, runID, progress); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := testhooks.Fire("before_commit"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return contracts.ErrCommitUnknown
	}
	_ = testhooks.Fire("after_commit_before_response")
	return nil
}

// updateProgressInTx appends one progress row and advances the manifest
// checkpoint in the caller's transaction, enforcing monotonic counts and the
// compare-and-swap on checkpoint_sequence.
func updateProgressInTx(ctx context.Context, tx *sql.Tx, runID string, p RecoveryProgress) error {
	var current int64
	var state, validation string
	var business int
	err := tx.QueryRowContext(ctx,
		"SELECT checkpoint_sequence,state,business_complete,validation_state FROM main."+
			RecoveryManifestTable+" WHERE run_id=?", runID).
		Scan(&current, &state, &business, &validation)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ErrNotFound
	}
	if err != nil {
		return dbError(err)
	}
	if terminalForWrites(RecoveryState(state)) || business != 0 || validation != string(ValidationNone) {
		return contracts.ErrConflict
	}
	if p.Sequence != current+1 {
		return contracts.ErrConflict
	}
	prev := RecoveryProgress{TableOrdinal: 0, RowsProcessed: 0}
	if current > 0 {
		err := tx.QueryRowContext(ctx,
			"SELECT table_ordinal,rows_processed,rows_committed,committed_batches FROM main."+
				RecoveryProgressTable+" WHERE run_id=? AND checkpoint_sequence=?", runID, current).
			Scan(&prev.TableOrdinal, &prev.RowsProcessed, &prev.RowsCommitted, &prev.CommittedBatches)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return contracts.ErrConflict
			}
			return dbError(err)
		}
	}
	// Monotonic progress: same table advances rows, a boundary moves exactly one
	// table forward and resets the per-table counter.
	switch {
	case p.TableOrdinal == prev.TableOrdinal:
		if p.Kind == "boundary" {
			return contracts.ErrConflict
		}
		if p.RowsProcessed < prev.RowsProcessed {
			return contracts.ErrConflict
		}
	case p.TableOrdinal == prev.TableOrdinal+1 && p.Kind == "boundary":
		if p.RowsProcessed != 0 {
			return contracts.ErrConflict
		}
	default:
		return contracts.ErrConflict
	}
	wantCommitted := prev.RowsCommitted
	wantBatches := prev.CommittedBatches
	if p.Kind == "batch" {
		wantCommitted += p.AffectedRows
		wantBatches++
	}
	if p.RowsCommitted != wantCommitted || p.CommittedBatches != wantBatches {
		return contracts.ErrConflict
	}
	if p.BusinessComplete && p.Kind != "boundary" {
		return contracts.ErrConflict
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO main."+RecoveryProgressTable+
			"(run_id,checkpoint_sequence,kind,table_ordinal,rows_processed,rows_committed,"+
			"committed_batches,batch_id,affected_rows,business_complete,created_at) "+
			"VALUES(?,?,?,?,?,?,?,?,?,?,?)",
		runID, p.Sequence, p.Kind, p.TableOrdinal, p.RowsProcessed, p.RowsCommitted,
		p.CommittedBatches, p.BatchID, p.AffectedRows, boolInt(p.BusinessComplete), outboxTimestamp())
	if err != nil {
		return classifyOutboxError(err)
	}
	result, err := tx.ExecContext(ctx,
		"UPDATE main."+RecoveryManifestTable+
			" SET checkpoint_sequence=?, state=?, business_complete=?, updated_at=? "+
			"WHERE run_id=? AND checkpoint_sequence=?",
		p.Sequence, string(RecoveryStateRunning), boolInt(business == 1 || p.BusinessComplete),
		outboxTimestamp(), runID, current)
	if err != nil {
		return dbError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return contracts.ErrDatabase
	}
	if affected != 1 {
		return contracts.ErrConflict
	}
	return nil
}

// SetManifestState records a terminal or informational state after a read-only
// reconciliation. The compare-and-swap on checkpoint_sequence makes a stale or
// concurrent finaliser fail with ErrConflict instead of overwriting a newer
// state. A terminal state (conflict/aborted/complete/validation_failed) is
// immutable within one run.
func (r *Recovery) SetManifestState(ctx context.Context, runID string, expectedSequence int64,
	state RecoveryState, businessComplete bool, validation ValidationState) error {
	if r == nil || r.Outbox == nil {
		return contracts.ErrInvalid
	}
	if !contracts.ValidRunID(runID) || expectedSequence < 0 || !validRecoveryState(state) ||
		!validValidationState(validation) || (validation == ValidationPassed && !businessComplete) {
		return contracts.ErrInvalid
	}
	if err := r.check(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return contracts.ErrClosed
	}
	result, err := r.conn.ExecContext(ctx,
		"UPDATE main."+RecoveryManifestTable+
			" SET state=?, business_complete=?, validation_state=?, updated_at=? "+
			"WHERE run_id=? AND checkpoint_sequence=? AND state NOT IN ('conflict','aborted','complete','validation_failed')",
		string(state), boolInt(businessComplete), string(validation), outboxTimestamp(), runID, expectedSequence)
	if err != nil {
		return dbError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return contracts.ErrDatabase
	}
	if affected != 1 {
		return contracts.ErrConflict
	}
	return nil
}

// VerifyOutboxRun cross-checks the duplicated run identity against the outbox
// run row. Divergence is tamper evidence and is refused.
func (r *Recovery) VerifyOutboxRun(ctx context.Context, m RecoveryManifest) error {
	if r == nil || r.Outbox == nil {
		return contracts.ErrInvalid
	}
	if err := r.check(ctx); err != nil {
		return err
	}
	var planID, approval, target, scope, protocol string
	err := r.adapter.db.QueryRowContext(ctx,
		"SELECT plan_id,approval_binding,target_digest,scope_digest,protocol FROM main."+
			OutboxRunTable+" WHERE run_id=?", m.RunID).
		Scan(&planID, &approval, &target, &scope, &protocol)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ErrConflict
	}
	if err != nil {
		return dbError(err)
	}
	if planID != m.PlanID || approval != m.ApprovalBinding || target != m.TargetID ||
		scope != m.PlanContentDigest || protocol != m.StreamingProtocol {
		return contracts.ErrConflict
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// schemaDDL splits an embedded migration into executable statements. SQL line
// comments are removed before splitting so a semicolon inside a comment can not
// truncate a statement.
func schemaDDL(source string) []string {
	var clean strings.Builder
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		clean.WriteString(line)
		clean.WriteByte('\n')
	}
	var statements []string
	for _, chunk := range strings.Split(clean.String(), ";") {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		statements = append(statements, trimmed)
	}
	return statements
}
