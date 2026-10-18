package runner

import (
	"context"

	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// ApprovalQuery is the read-only review-decision dependency Apply consumes. It
// is deliberately narrower than history.DecisionStore: Apply can only look a
// decision up, so signing and persistence stay in the explicit issuance path and
// no Apply path can create, replace or revoke a record.
//
// Scan, Preview, Validate and Plan never touch this dependency, so a read-only
// workflow can not reach the history store at all.
type ApprovalQuery interface {
	Lookup(context.Context, contracts.Plan) (history.Record, error)
}

// SetApprovalQuery injects the read-only approval query Apply requires. A nil
// query is not a permissive default: Apply refuses with contracts.ErrUnsafe
// until a query is supplied and returns a decision bound to the plan.
func (e *Engine) SetApprovalQuery(query ApprovalQuery) {
	if e != nil {
		e.approvals = query
	}
}

// checkApproval requires a decision that Lookup accepted for exactly this plan.
// A missing query, a lookup error (no record, unreadable store, unknown signer,
// bad signature, foreign plan, expiry, revocation, future issue) and a returned
// record that does not re-bind this plan all refuse the run.
//
// The lookup seam owns signature verification, expiry and the ordered scope
// binding; this check re-derives the plan binding from the record it receives so
// a decision for a subset, a reordered scope list or another plan can not
// authorize this one even if the injected seam is wrong.
func (e *Engine) checkApproval(ctx context.Context, plan contracts.Plan) error {
	if e.approvals == nil {
		return contracts.ErrUnsafe
	}
	record, err := e.approvals.Lookup(ctx, plan)
	if err != nil {
		return err
	}
	if !recordAuthorizes(record, plan) {
		return contracts.ErrUnsafe
	}
	return nil
}

// recordAuthorizes reports whether a decision re-binds exactly this plan: every
// plan digest and the whole ordered (table, column) scope list must match, and a
// revoked record authorizes nothing. Strategy and key bindings are covered by
// ConfigDigest here and are checked field-by-field by the history store.
func recordAuthorizes(record history.Record, plan contracts.Plan) bool {
	if record.PlanID != plan.ID || record.SourceSnapshotID != plan.SourceSnapshotID ||
		record.TargetID != plan.TargetID || record.SchemaDigest != plan.SchemaDigest ||
		record.ConfigDigest != plan.ConfigDigest || record.RevokedAt != nil {
		return false
	}
	index := 0
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if index >= len(record.Scopes) {
				return false
			}
			scope := record.Scopes[index]
			if scope.Table != table.Table || scope.Column != column.Column.Column {
				return false
			}
			index++
		}
	}
	return index == len(record.Scopes)
}
