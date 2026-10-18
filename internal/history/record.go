package history

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// RecordVersion is the record format version carried, signed and verified by
// every record. It is not contracts.APIRevision: it versions this store's file
// format, which may change independently of the frozen contracts.
const RecordVersion = "history-record-v1"

const (
	// magicLine opens every stored record so a wrong or unrelated file fails
	// before any signature work.
	magicLine = "maskriver-history-record-v1"
	// maxIdentityBytes bounds self-declared identity labels so a record cannot
	// carry an unbounded attacker-chosen payload.
	maxIdentityBytes = 256
	// maxFutureIssuedSkew bounds how far in the future an IssuedAt may be before
	// Lookup refuses the record. A record cannot have been issued after the
	// moment it is presented, but two hosts' clocks can disagree by a small
	// amount, so the skew is tolerated instead of compared exactly. It is a fixed
	// bound, not a caller-supplied window: a record dated weeks ahead must not
	// become usable by waiting, and must not be accepted at all.
	maxFutureIssuedSkew = 5 * time.Minute
)

// Scope is one reviewed (table, column) target. The full table identity is bound
// so two schemas holding the same table name cannot share an approval.
type Scope struct {
	Table  contracts.TableRef
	Column string
}

// Record is one reviewer decision bound to one immutable plan. It holds only
// identifiers, digests and review metadata: never source values, primary-key
// values, masking keys, key material or DSNs. ReviewerID and every digest are
// attacker-controlled labels unless the signature verifies; the binding check
// against the live plan is what makes the record usable.
//
// Version is the record format version (RecordVersion). Re-approving a plan
// replaces its file rather than adding a revision, so a Record is the current
// decision and not an audit entry.
type Record struct {
	Version          string
	PlanID           string
	SourceSnapshotID string
	TargetID         string
	SchemaDigest     string
	ConfigDigest     string
	StrategyID       string
	StrategyVersion  string
	KeyID            string
	Scopes           []Scope
	ReviewerID       string
	SignerKeyID      string
	IssuedAt         time.Time
	ExpiresAt        time.Time
	RevokedAt        *time.Time
}

// canonicalRecord is the signed wire form. Field order and names are part of the
// format: changing either invalidates every existing signature.
type canonicalRecord struct {
	Version          string           `json:"version"`
	PlanID           string           `json:"plan_id"`
	SourceSnapshotID string           `json:"source_snapshot_id"`
	TargetID         string           `json:"target_id"`
	SchemaDigest     string           `json:"schema_digest"`
	ConfigDigest     string           `json:"config_digest"`
	StrategyID       string           `json:"strategy_id"`
	StrategyVersion  string           `json:"strategy_version"`
	KeyID            string           `json:"key_id"`
	Scopes           []canonicalScope `json:"scopes"`
	ReviewerID       string           `json:"reviewer_id"`
	SignerKeyID      string           `json:"signer_key_id"`
	IssuedAt         string           `json:"issued_at"`
	ExpiresAt        string           `json:"expires_at"`
	RevokedAt        string           `json:"revoked_at,omitempty"`
}

type canonicalScope struct {
	Database string `json:"database"`
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	Column   string `json:"column"`
}

// canonical returns the exact bytes that are signed and stored. Times are
// normalised to UTC and RFC3339Nano so equal instants always encode identically;
// a payload that does not re-encode to itself is rejected on load.
func (r Record) canonical() ([]byte, error) {
	scopes := make([]canonicalScope, 0, len(r.Scopes))
	for _, scope := range r.Scopes {
		scopes = append(scopes, canonicalScope{
			Database: scope.Table.Database, Schema: scope.Table.Schema,
			Table: scope.Table.Table, Column: scope.Column,
		})
	}
	wire := canonicalRecord{
		Version: r.Version, PlanID: r.PlanID,
		SourceSnapshotID: r.SourceSnapshotID, TargetID: r.TargetID,
		SchemaDigest: r.SchemaDigest, ConfigDigest: r.ConfigDigest,
		StrategyID: r.StrategyID, StrategyVersion: r.StrategyVersion,
		KeyID: r.KeyID, Scopes: scopes, ReviewerID: r.ReviewerID,
		SignerKeyID: r.SignerKeyID,
		IssuedAt:    formatTimestamp(r.IssuedAt), ExpiresAt: formatTimestamp(r.ExpiresAt),
	}
	if r.RevokedAt != nil {
		wire.RevokedAt = formatTimestamp(*r.RevokedAt)
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, unsafeError(errCorrupt)
	}
	return payload, nil
}

func formatTimestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, unsafeError(errCorrupt)
	}
	return parsed, nil
}

// decodePayload accepts only the canonical form: unknown fields, duplicate keys,
// non-UTC timestamps, reordered keys or any other re-encoding difference are
// refused, so a record has exactly one valid byte representation.
func decodePayload(payload []byte) (Record, error) {
	var wire canonicalRecord
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Record{}, unsafeError(errCorrupt)
	}
	issuedAt, err := parseTimestamp(wire.IssuedAt)
	if err != nil {
		return Record{}, err
	}
	expiresAt, err := parseTimestamp(wire.ExpiresAt)
	if err != nil {
		return Record{}, err
	}
	record := Record{
		Version: wire.Version, PlanID: wire.PlanID,
		SourceSnapshotID: wire.SourceSnapshotID, TargetID: wire.TargetID,
		SchemaDigest: wire.SchemaDigest, ConfigDigest: wire.ConfigDigest,
		StrategyID: wire.StrategyID, StrategyVersion: wire.StrategyVersion,
		KeyID: wire.KeyID, ReviewerID: wire.ReviewerID,
		SignerKeyID: wire.SignerKeyID, IssuedAt: issuedAt, ExpiresAt: expiresAt,
	}
	for _, scope := range wire.Scopes {
		record.Scopes = append(record.Scopes, Scope{
			Table: contracts.TableRef{
				Database: scope.Database, Schema: scope.Schema, Table: scope.Table,
			},
			Column: scope.Column,
		})
	}
	if wire.RevokedAt != "" {
		revokedAt, err := parseTimestamp(wire.RevokedAt)
		if err != nil {
			return Record{}, err
		}
		record.RevokedAt = &revokedAt
	}
	if err := record.checkShape(); err != nil {
		return Record{}, err
	}
	reencoded, err := record.canonical()
	if err != nil {
		return Record{}, err
	}
	if !bytes.Equal(reencoded, payload) {
		return Record{}, unsafeError(errCorrupt)
	}
	return record, nil
}

// checkShape refuses a decoded record that is missing a binding field or carries
// no scope. It cannot make a record trustworthy; it only rejects records that are
// structurally unable to bind a plan.
func (r Record) checkShape() error {
	if r.Version != RecordVersion || r.PlanID == "" || r.SourceSnapshotID == "" ||
		r.TargetID == "" || r.SchemaDigest == "" || r.ConfigDigest == "" ||
		r.StrategyID == "" || r.StrategyVersion == "" || r.KeyID == "" ||
		r.ReviewerID == "" || len(r.ReviewerID) > maxIdentityBytes ||
		r.SignerKeyID == "" || len(r.SignerKeyID) > maxIdentityBytes ||
		len(r.Scopes) == 0 {
		return unsafeError(errCorrupt)
	}
	for _, scope := range r.Scopes {
		if scope.Table.Database == "" || scope.Table.Schema == "" ||
			scope.Table.Table == "" || scope.Column == "" {
			return unsafeError(errCorrupt)
		}
	}
	return nil
}

// newRecord derives a record for exactly this plan. Every binding field comes
// from the plan, so a caller cannot approve a different scope set or digest than
// the plan it passes.
func newRecord(plan contracts.Plan, reviewerID string, issuedAt, expiresAt time.Time, signerKeyID string) (Record, error) {
	if err := plan.ValidateShape(); err != nil {
		return Record{}, err
	}
	if reviewerID == "" || len(reviewerID) > maxIdentityBytes ||
		signerKeyID == "" || len(signerKeyID) > maxIdentityBytes {
		return Record{}, contracts.ErrInvalid
	}
	if issuedAt.IsZero() || expiresAt.IsZero() || !expiresAt.After(issuedAt) {
		return Record{}, contracts.ErrInvalid
	}
	record := Record{
		Version: RecordVersion, PlanID: plan.ID,
		SourceSnapshotID: plan.SourceSnapshotID, TargetID: plan.TargetID,
		SchemaDigest: plan.SchemaDigest, ConfigDigest: plan.ConfigDigest,
		ReviewerID: reviewerID, SignerKeyID: signerKeyID,
		IssuedAt: issuedAt.UTC(), ExpiresAt: expiresAt.UTC(),
	}
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			// The first column seeds the strategy fields; checkBinding then
			// refuses a plan whose columns disagree about strategy or key.
			if record.StrategyID == "" {
				record.StrategyID = column.Strategy.ID
				record.StrategyVersion = column.Strategy.Version
				record.KeyID = column.KeyID
			}
			record.Scopes = append(record.Scopes, Scope{Table: table.Table, Column: column.Column.Column})
		}
	}
	if err := record.checkBinding(plan); err != nil {
		return Record{}, err
	}
	return record, nil
}

// checkBinding requires the record and the live plan to agree on every bound
// field and on the exact ordered scope list. Any difference - including a
// reordered scope list or a subset of a multi-table plan - is a conflict.
func (r Record) checkBinding(plan contracts.Plan) error {
	if err := plan.ValidateShape(); err != nil {
		return err
	}
	if r.Version != RecordVersion {
		return unsafeError(errCorrupt)
	}
	if r.PlanID != plan.ID || r.SourceSnapshotID != plan.SourceSnapshotID ||
		r.TargetID != plan.TargetID || r.SchemaDigest != plan.SchemaDigest ||
		r.ConfigDigest != plan.ConfigDigest {
		return conflictError(errBinding)
	}
	index := 0
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if column.Strategy.ID != r.StrategyID ||
				column.Strategy.Version != r.StrategyVersion || column.KeyID != r.KeyID {
				return conflictError(errBinding)
			}
			if index >= len(r.Scopes) {
				return conflictError(errBinding)
			}
			if r.Scopes[index] != (Scope{Table: table.Table, Column: column.Column.Column}) {
				return conflictError(errBinding)
			}
			index++
		}
	}
	if index != len(r.Scopes) {
		return conflictError(errBinding)
	}
	return nil
}

// checkActive refuses a revoked, expired or future-issued record. A record is
// valid for now < ExpiresAt, with expiry inclusive of the expiry instant, and
// only while IssuedAt is not later than now plus maxFutureIssuedSkew. The
// future check is what keeps a record the reviewer dated ahead of the current
// clock from being treated as an approval that already exists.
func (r Record) checkActive(now time.Time) error {
	if r.RevokedAt != nil {
		return unsafeError(errRevoked)
	}
	if r.IssuedAt.After(now.Add(maxFutureIssuedSkew)) {
		return unsafeError(errFutureIssued)
	}
	if !now.Before(r.ExpiresAt) {
		return unsafeError(errExpired)
	}
	return nil
}

// encodeRecord signs the canonical payload and wraps it in the stored file form:
// the magic line, the hex detached signature, then the payload.
func encodeRecord(record Record, private ed25519.PrivateKey) ([]byte, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, unsafeError(errNoSigner)
	}
	payload, err := record.canonical()
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(private, payload)
	raw := make([]byte, 0, len(magicLine)+2*ed25519.SignatureSize+len(payload)+2)
	raw = append(raw, magicLine...)
	raw = append(raw, '\n')
	raw = append(raw, hex.EncodeToString(signature)...)
	raw = append(raw, '\n')
	raw = append(raw, payload...)
	return raw, nil
}

// decodeRecord splits the stored file and enforces the canonical form without
// verifying the signature: the signer key ID lives inside the unverified payload,
// so it is needed to choose which trusted key to verify against.
func decodeRecord(raw []byte) (Record, []byte, []byte, error) {
	if len(raw) == 0 || len(raw) > maxRecordBytes {
		return Record{}, nil, nil, unsafeError(errCorrupt)
	}
	header, body, ok := bytes.Cut(raw, []byte("\n"))
	if !ok || string(header) != magicLine {
		return Record{}, nil, nil, unsafeError(errCorrupt)
	}
	encodedSignature, payload, ok := bytes.Cut(body, []byte("\n"))
	if !ok || len(payload) == 0 {
		return Record{}, nil, nil, unsafeError(errCorrupt)
	}
	signature, err := hex.DecodeString(string(encodedSignature))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Record{}, nil, nil, unsafeError(errCorrupt)
	}
	record, err := decodePayload(payload)
	if err != nil {
		return Record{}, nil, nil, err
	}
	return record, payload, signature, nil
}

// claimedPlanID reports the plan a stored file declares, without verifying its
// signature. It is only used to detect two files claiming the same plan; a file
// that does not declare a plan cannot be loaded as a record at all.
func claimedPlanID(raw []byte) (string, bool) {
	_, body, ok := bytes.Cut(raw, []byte("\n"))
	if !ok {
		return "", false
	}
	_, payload, ok := bytes.Cut(body, []byte("\n"))
	if !ok {
		return "", false
	}
	var claim struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.Unmarshal(payload, &claim); err != nil || claim.PlanID == "" {
		return "", false
	}
	return claim.PlanID, true
}

// Refusal sentinels. Every exported error wraps both a contracts category and one
// of these, so callers can branch on the category and tests on the cause. Their
// text never includes record contents, paths or key material.
var (
	errCorrupt        = errors.New("history record is not a canonical record")
	errSignature      = errors.New("history record signature does not verify")
	errSignerMismatch = errors.New("review signing key does not match the trusted key it names")
	errUnknownSigner  = errors.New("history record signer is not trusted")
	errForeignSigner  = errors.New("history record was signed by another key")
	errNoTrustedKey   = errors.New("no trusted reviewer key configured")
	errNoSigner       = errors.New("no review signing key configured")
	errExpired        = errors.New("history record has expired")
	errFutureIssued   = errors.New("history record was issued in the future")
	errRevoked        = errors.New("history record is revoked")
	errAlreadyRevoked = errors.New("history record is already revoked")
	errBinding        = errors.New("history record does not bind this plan")
	errAmbiguous      = errors.New("history store holds ambiguous records for this plan")
	errRead           = errors.New("history record read failed")
	errWrite          = errors.New("history record write failed")
)

func unsafeError(cause error) error {
	return fmt.Errorf("%w: %w", contracts.ErrUnsafe, cause)
}

func conflictError(cause error) error {
	return fmt.Errorf("%w: %w", contracts.ErrConflict, cause)
}
