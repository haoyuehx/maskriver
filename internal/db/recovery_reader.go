package db

// Read-only recovery introspection for `maskriver status`.
//
// The reader opens the target with SQLite mode=ro, verifies both versioned
// schemas and never begins a transaction that can write, install a schema or
// recover a hot journal. If the database can not be read consistently it fails
// closed instead of guessing.

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// ReceiptSummary is a constant-size aggregate over one run's receipts.
type ReceiptSummary struct {
	Count  int64
	Rows   int64
	NextID int64
}

// RecoveryReader is a read-only view of one target database's recovery state.
type RecoveryReader struct {
	db *sql.DB
}

// OpenRecoveryReader opens an existing SQLite target read-only and verifies the
// exact outbox v1 and recovery v1 schema shapes. It performs no migration.
func OpenRecoveryReader(ctx context.Context, opts contracts.DatabaseOptions) (*RecoveryReader, error) {
	if ctx == nil || opts.Dialect != contracts.SQLite {
		return nil, contracts.ErrUnsupported
	}
	a, err := open(ctx, opts, true, nil)
	if err != nil {
		return nil, err
	}
	closeOnError := func(err error) (*RecoveryReader, error) {
		_ = a.Close()
		return nil, err
	}
	version, err := schemaVersionOn(ctx, a.db, outboxSchemaTable)
	if errors.Is(err, contracts.ErrNotFound) {
		return closeOnError(contracts.ErrUnsupported)
	}
	if err != nil {
		return closeOnError(err)
	}
	if version != OutboxSchemaVersion {
		return closeOnError(contracts.ErrUnsupported)
	}
	for _, table := range outboxShape {
		if err := verifyOutboxTableShape(ctx, a.db, table); err != nil {
			return closeOnError(err)
		}
	}
	version, err = schemaVersionOn(ctx, a.db, recoverySchemaTable)
	if errors.Is(err, contracts.ErrNotFound) {
		return closeOnError(contracts.ErrUnsupported)
	}
	if err != nil {
		return closeOnError(err)
	}
	if version != RecoverySchemaVersion {
		return closeOnError(contracts.ErrUnsupported)
	}
	for _, table := range recoveryShape {
		if err := verifyOutboxTableShape(ctx, a.db, table); err != nil {
			return closeOnError(err)
		}
	}
	return &RecoveryReader{db: a.db}, nil
}

// Close releases the read-only handle. It is idempotent.
func (r *RecoveryReader) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	err := r.db.Close()
	r.db = nil
	if err != nil {
		return contracts.ErrDatabase
	}
	return nil
}

// Manifest reads one run's non-sensitive manifest.
func (r *RecoveryReader) Manifest(ctx context.Context, runID string) (RecoveryManifest, error) {
	if r == nil || r.db == nil || ctx == nil {
		return RecoveryManifest{}, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return RecoveryManifest{}, err
	}
	if !contracts.ValidRunID(runID) {
		return RecoveryManifest{}, contracts.ErrInvalid
	}
	return readManifest(ctx, r.db, runID)
}

// LatestProgress returns the newest progress row, or the zero value when the run
// has not committed any progress yet.
func (r *RecoveryReader) LatestProgress(ctx context.Context, runID string) (RecoveryProgress, bool, error) {
	if r == nil || r.db == nil || ctx == nil {
		return RecoveryProgress{}, false, contracts.ErrInvalid
	}
	if !contracts.ValidRunID(runID) {
		return RecoveryProgress{}, false, contracts.ErrInvalid
	}
	var p RecoveryProgress
	var business int
	var created string
	err := r.db.QueryRowContext(ctx,
		"SELECT run_id,checkpoint_sequence,kind,table_ordinal,rows_processed,rows_committed,"+
			"committed_batches,batch_id,affected_rows,business_complete,created_at "+
			"FROM main."+RecoveryProgressTable+
			" WHERE run_id=? ORDER BY checkpoint_sequence DESC LIMIT 1", runID).
		Scan(&p.RunID, &p.Sequence, &p.Kind, &p.TableOrdinal, &p.RowsProcessed,
			&p.RowsCommitted, &p.CommittedBatches, &p.BatchID, &p.AffectedRows,
			&business, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return RecoveryProgress{RunID: runID}, false, nil
	}
	if err != nil {
		return RecoveryProgress{}, false, dbError(err)
	}
	p.BusinessComplete = business == 1
	parsed, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return RecoveryProgress{}, false, contracts.ErrDatabase
	}
	p.CreatedAt = parsed
	return p, true, nil
}

// Receipts returns a constant-size aggregate over the run's business receipts.
func (r *RecoveryReader) Receipts(ctx context.Context, runID string) (ReceiptSummary, error) {
	var summary ReceiptSummary
	if r == nil || r.db == nil || ctx == nil {
		return summary, contracts.ErrInvalid
	}
	if !contracts.ValidRunID(runID) {
		return summary, contracts.ErrInvalid
	}
	var next sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*),COALESCE(SUM(affected_rows),0),MAX(batch_sequence) FROM main."+
			OutboxBatchTable+" WHERE run_id=?", runID).
		Scan(&summary.Count, &summary.Rows, &next)
	if err != nil {
		return ReceiptSummary{}, dbError(err)
	}
	if next.Valid {
		summary.NextID = next.Int64 + 1
	}
	return summary, nil
}
