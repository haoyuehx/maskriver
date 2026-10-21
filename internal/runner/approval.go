package runner

import (
	"context"
	"errors"
	"reflect"
	"time"

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
const approvalFutureIssuedSkew = 5 * time.Minute

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
// binding; this check re-derives the plan binding, the record version, the
// validity window, revocation, and the strategy/key binding from the record it
// receives, so a decision for a subset, a reordered scope list, an expired,
// future-issued or revoked version of this plan, or another plan can not
// authorize this one even if the injected seam is wrong. It is not an
// authenticity check: the store's Ed25519 verification remains authoritative.
func (e *Engine) checkApproval(ctx context.Context, plan contracts.Plan) error {
	if nilApprovalQuery(e.approvals) {
		return contracts.ErrUnsafe
	}
	record, err := e.approvals.Lookup(ctx, plan)
	if err != nil {
		return approvalRefusal(err)
	}
	if !recordAuthorizes(record, plan, time.Now()) {
		return contracts.ErrUnsafe
	}
	return nil
}

// nilApprovalQuery reports whether the injected seam can not be called safely.
// A nil interface is the common case, but an interface holding a typed-nil
// pointer (or a nil function) is not equal to nil and would panic inside Lookup.
func nilApprovalQuery(query ApprovalQuery) bool {
	if query == nil {
		return true
	}
	value := reflect.ValueOf(query)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	}
	return false
}

// approvalRefusal keeps a refusal category the store or the context already
// carries (no record, conflict, unsafe, cancellation) so the caller can still
// distinguish them. Any other seam failure is an abnormal dependency state and
// collapses to contracts.ErrUnsafe; either way the run refuses before a Writer
// exists.
func approvalRefusal(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, contracts.ErrInvalid) || errors.Is(err, contracts.ErrUnsupported) ||
		errors.Is(err, contracts.ErrIncomplete) || errors.Is(err, contracts.ErrUnsafe) ||
		errors.Is(err, contracts.ErrConflict) || errors.Is(err, contracts.ErrNotFound) {
		return err
	}
	return contracts.ErrUnsafe
}

// recordAuthorizes reports whether a decision re-binds exactly this plan: every
// plan digest, the whole ordered (table, column) scope list, the record version,
// the validity window, revocation, and the plan's strategy pair and KeyID.
//
// This is defence in depth against a wrong or compromised Lookup seam, NOT the
// authenticity check. It does not verify the Ed25519 signature, cannot prove the
// record was issued by a trusted reviewer, and must never be treated as such:
// history.Store.Lookup remains the authority that authenticates a record, and
// this re-check only prevents a seam bug from turning a record the store would
// never release into an authorized write.
func recordAuthorizes(record history.Record, plan contracts.Plan, now time.Time) bool {
	if record.Version != history.RecordVersion ||
		record.PlanID != plan.ID || record.SourceSnapshotID != plan.SourceSnapshotID ||
		record.TargetID != plan.TargetID || record.SchemaDigest != plan.SchemaDigest ||
		record.ConfigDigest != plan.ConfigDigest || record.RevokedAt != nil {
		return false
	}
	// Tolerate the same small clock skew as history.Store; a record dated far
	// in the future is still refused, and expiry is still exact.
	if record.IssuedAt.After(now.Add(approvalFutureIssuedSkew)) || !now.Before(record.ExpiresAt) {
		return false
	}
	index := 0
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if index >= len(record.Scopes) {
				return false
			}
			if column.Strategy.ID != record.StrategyID ||
				column.Strategy.Version != record.StrategyVersion ||
				column.KeyID != record.KeyID {
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
