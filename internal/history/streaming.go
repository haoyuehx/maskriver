package history

// Streaming (m2) approval records.
//
// A streaming record is a separate signed file, a separate version string and a
// separate file suffix. It never reinterprets, upgrades or validates a legacy
// m1a-v1 record as an m2 authorization, and a streaming record can not
// authorize the legacy path: the two lookups are disjoint by suffix and both
// bind their own protocol/binding revision.
//
// The record adds the missing content binding the frozen contracts can not
// express: the ordered full column plans (including Scope and
// NormalizationVersion) are reduced to one deterministic PlanContentDigest that
// is signed. Plan.ID alone is not sufficient because it does not cover those
// fields.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const (
	// StreamingRecordVersion versions the signed streaming wire form. It is not
	// contracts.ApprovalBindingRevision and not the legacy RecordVersion.
	StreamingRecordVersion = "history-streaming-record-v1"
	// streamingMagicLine opens every streaming record file.
	streamingMagicLine = "maskriver-history-streaming-record-v1"
	// streamingSuffix keeps streaming approvals in separate files so a legacy
	// decision can never be loaded by the streaming path.
	streamingSuffix = ".streaming-decision"
)

// StreamingScope is one reviewed column plan in canonical plan order. The
// per-column fields that Plan.ID does not cover are signed explicitly.
type StreamingScope struct {
	Table                contracts.TableRef
	Column               string
	Scope                string
	NormalizationVersion string
	Type                 contracts.Type
}

// StreamingRecord is one verified streaming approval. It carries identifiers,
// digests and review metadata only: never values, keys or DSNs.
type StreamingRecord struct {
	Version           string
	Protocol          string
	BindingRevision   string
	PlanID            string
	SourceSnapshotID  string
	TargetID          string
	SchemaDigest      string
	ConfigDigest      string
	PlanContentDigest string
	StrategyID        string
	StrategyVersion   string
	KeyID             string
	Scopes            []StreamingScope
	ReviewerID        string
	SignerKeyID       string
	IssuedAt          time.Time
	ExpiresAt         time.Time
	RevokedAt         *time.Time
}

type canonicalStreamingRecord struct {
	Version           string                    `json:"version"`
	Protocol          string                    `json:"protocol"`
	BindingRevision   string                    `json:"binding_revision"`
	PlanID            string                    `json:"plan_id"`
	SourceSnapshotID  string                    `json:"source_snapshot_id"`
	TargetID          string                    `json:"target_id"`
	SchemaDigest      string                    `json:"schema_digest"`
	ConfigDigest      string                    `json:"config_digest"`
	PlanContentDigest string                    `json:"plan_content_digest"`
	StrategyID        string                    `json:"strategy_id"`
	StrategyVersion   string                    `json:"strategy_version"`
	KeyID             string                    `json:"key_id"`
	Scopes            []canonicalStreamingScope `json:"scopes"`
	ReviewerID        string                    `json:"reviewer_id"`
	SignerKeyID       string                    `json:"signer_key_id"`
	IssuedAt          string                    `json:"issued_at"`
	ExpiresAt         string                    `json:"expires_at"`
	RevokedAt         string                    `json:"revoked_at,omitempty"`
}

type canonicalStreamingScope struct {
	Database             string `json:"database"`
	Schema               string `json:"schema"`
	Table                string `json:"table"`
	Column               string `json:"column"`
	Scope                string `json:"scope"`
	NormalizationVersion string `json:"normalization_version"`
	TypeKind             int    `json:"type_kind"`
	TypeNative           string `json:"type_native"`
	TypeNullable         bool   `json:"type_nullable"`
	TypeLength           int    `json:"type_length"`
	TypePrecision        int    `json:"type_precision"`
	TypeScale            int    `json:"type_scale"`
	TypeCollation        string `json:"type_collation"`
}

// canonicalPlanContent reduces the execution-sensitive plan content to one
// deterministic, signed view. Field order is part of the encoding.
type canonicalPlanContent struct {
	Revision         string                  `json:"revision"`
	PlanID           string                  `json:"plan_id"`
	SourceSnapshotID string                  `json:"source_snapshot_id"`
	TargetID         string                  `json:"target_id"`
	SchemaDigest     string                  `json:"schema_digest"`
	ConfigDigest     string                  `json:"config_digest"`
	Tables           []canonicalContentTable `json:"tables"`
}

type canonicalContentTable struct {
	Database   string                   `json:"database"`
	Schema     string                   `json:"schema"`
	Table      string                   `json:"table"`
	PrimaryKey []string                 `json:"primary_key"`
	Columns    []canonicalContentColumn `json:"columns"`
}

type canonicalContentColumn struct {
	Database             string `json:"database"`
	Schema               string `json:"schema"`
	Table                string `json:"table"`
	Column               string `json:"column"`
	StrategyID           string `json:"strategy_id"`
	StrategyVersion      string `json:"strategy_version"`
	Scope                string `json:"scope"`
	NormalizationVersion string `json:"normalization_version"`
	KeyID                string `json:"key_id"`
	TypeKind             int    `json:"type_kind"`
	TypeNative           string `json:"type_native"`
	TypeNullable         bool   `json:"type_nullable"`
	TypeLength           int    `json:"type_length"`
	TypePrecision        int    `json:"type_precision"`
	TypeScale            int    `json:"type_scale"`
	TypeCollation        string `json:"type_collation"`
}

// CanonicalPlanDigest returns the SHA-256 of the canonical plan content. It
// covers every table, primary key, column reference, strategy, per-column Scope,
// NormalizationVersion, KeyID and declared type in plan order. Two plans with
// the same Plan.ID but any different content produce different digests.
func CanonicalPlanDigest(plan contracts.Plan) (string, error) {
	if err := plan.ValidateShape(); err != nil {
		return "", err
	}
	content := canonicalPlanContent{
		Revision: plan.Revision, PlanID: plan.ID,
		SourceSnapshotID: plan.SourceSnapshotID, TargetID: plan.TargetID,
		SchemaDigest: plan.SchemaDigest, ConfigDigest: plan.ConfigDigest,
		Tables: make([]canonicalContentTable, 0, len(plan.Tables)),
	}
	for _, table := range plan.Tables {
		ct := canonicalContentTable{
			Database: table.Table.Database, Schema: table.Table.Schema,
			Table:      table.Table.Table,
			PrimaryKey: append(make([]string, 0, len(table.PrimaryKey)), table.PrimaryKey...),
			Columns:    make([]canonicalContentColumn, 0, len(table.Columns)),
		}
		for _, col := range table.Columns {
			ct.Columns = append(ct.Columns, canonicalContentColumn{
				Database: col.Column.Database, Schema: col.Column.Schema,
				Table: col.Column.Table, Column: col.Column.Column,
				StrategyID: col.Strategy.ID, StrategyVersion: col.Strategy.Version,
				Scope: col.Scope, NormalizationVersion: col.NormalizationVersion,
				KeyID: col.KeyID, TypeKind: int(col.Type.Kind),
				TypeNative: col.Type.Native, TypeNullable: col.Type.Nullable,
				TypeLength: col.Type.Length, TypePrecision: col.Type.Precision,
				TypeScale: col.Type.Scale, TypeCollation: col.Type.Collation,
			})
		}
		content.Tables = append(content.Tables, ct)
	}
	payload, err := json.Marshal(content)
	if err != nil {
		return "", unsafeError(errCorrupt)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (r StreamingRecord) canonical() ([]byte, error) {
	scopes := make([]canonicalStreamingScope, 0, len(r.Scopes))
	for _, scope := range r.Scopes {
		scopes = append(scopes, canonicalStreamingScope{
			Database: scope.Table.Database, Schema: scope.Table.Schema,
			Table: scope.Table.Table, Column: scope.Column,
			Scope: scope.Scope, NormalizationVersion: scope.NormalizationVersion,
			TypeKind: int(scope.Type.Kind), TypeNative: scope.Type.Native,
			TypeNullable: scope.Type.Nullable, TypeLength: scope.Type.Length,
			TypePrecision: scope.Type.Precision, TypeScale: scope.Type.Scale,
			TypeCollation: scope.Type.Collation,
		})
	}
	wire := canonicalStreamingRecord{
		Version: r.Version, Protocol: r.Protocol, BindingRevision: r.BindingRevision,
		PlanID: r.PlanID, SourceSnapshotID: r.SourceSnapshotID, TargetID: r.TargetID,
		SchemaDigest: r.SchemaDigest, ConfigDigest: r.ConfigDigest,
		PlanContentDigest: r.PlanContentDigest,
		StrategyID:        r.StrategyID, StrategyVersion: r.StrategyVersion,
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

func (r StreamingRecord) checkShape() error {
	if r.Version != StreamingRecordVersion ||
		r.Protocol != contracts.StreamingProtocolRevision ||
		r.BindingRevision != contracts.ApprovalBindingRevision ||
		r.PlanID == "" || r.SourceSnapshotID == "" || r.TargetID == "" ||
		r.SchemaDigest == "" || r.ConfigDigest == "" || r.PlanContentDigest == "" ||
		r.StrategyID == "" || r.StrategyVersion == "" || r.KeyID == "" ||
		r.ReviewerID == "" || len(r.ReviewerID) > maxIdentityBytes ||
		r.SignerKeyID == "" || len(r.SignerKeyID) > maxIdentityBytes ||
		len(r.Scopes) == 0 {
		return unsafeError(errCorrupt)
	}
	for _, scope := range r.Scopes {
		if scope.Table.Database == "" || scope.Table.Schema == "" ||
			scope.Table.Table == "" || scope.Column == "" ||
			scope.Scope == "" || scope.NormalizationVersion == "" ||
			scope.Type.Kind == contracts.Null {
			return unsafeError(errCorrupt)
		}
	}
	return nil
}

// Binding compares the record to a live plan. The canonical content digest is
// recomputed here, so a plan whose Scope, NormalizationVersion, type or column
// order changed can not reuse a signature even if Plan.ID collides.
func (r StreamingRecord) checkBinding(plan contracts.Plan) error {
	if err := r.checkShape(); err != nil {
		return err
	}
	if r.PlanID != plan.ID || r.SourceSnapshotID != plan.SourceSnapshotID ||
		r.TargetID != plan.TargetID || r.SchemaDigest != plan.SchemaDigest ||
		r.ConfigDigest != plan.ConfigDigest {
		return conflictError(errBinding)
	}
	digest, err := CanonicalPlanDigest(plan)
	if err != nil {
		return err
	}
	if digest != r.PlanContentDigest {
		return conflictError(errBinding)
	}
	index := 0
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if index >= len(r.Scopes) {
				return conflictError(errBinding)
			}
			scope := r.Scopes[index]
			if scope.Table != table.Table || scope.Column != column.Column.Column ||
				scope.Scope != column.Scope ||
				scope.NormalizationVersion != column.NormalizationVersion ||
				scope.Table.Table != column.Column.Table ||
				scope.Type != column.Type ||
				column.Strategy.ID != r.StrategyID ||
				column.Strategy.Version != r.StrategyVersion ||
				column.KeyID != r.KeyID {
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

func (r StreamingRecord) StreamingApproval() contracts.StreamingApproval {
	approval := contracts.StreamingApproval{
		BindingRevision: r.BindingRevision, PlanID: r.PlanID,
		SourceSnapshotID: r.SourceSnapshotID, TargetID: r.TargetID,
		SchemaDigest: r.SchemaDigest, ConfigDigest: r.ConfigDigest,
		StrategyID: r.StrategyID, StrategyVersion: r.StrategyVersion, KeyID: r.KeyID,
		IssuedAt: r.IssuedAt, ExpiresAt: r.ExpiresAt, Revoked: r.RevokedAt != nil,
	}
	for _, scope := range r.Scopes {
		approval.Scopes = append(approval.Scopes, contracts.StreamingScope{
			Table: scope.Table, Column: scope.Column,
		})
	}
	return approval
}

// StreamingApproveRequest asks to persist one streaming approval. Every binding
// field is derived from Plan; the caller supplies only the review metadata.
type StreamingApproveRequest struct {
	Plan       contracts.Plan
	ReviewerID string
	IssuedAt   time.Time
	ExpiresAt  time.Time
}

// ApproveStreaming signs and persists a streaming approval for exactly this
// plan. It never writes a legacy Decision file and never upgrades a legacy
// record.
func (s *Store) ApproveStreaming(ctx context.Context, req StreamingApproveRequest) (StreamingRecord, error) {
	if ctx == nil {
		return StreamingRecord{}, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return StreamingRecord{}, err
	}
	if s.signer == nil {
		return StreamingRecord{}, unsafeError(errNoSigner)
	}
	if !signerMatchesTrust(s.signer, s.trust) {
		return StreamingRecord{}, unsafeError(errSignerMismatch)
	}
	record, err := newStreamingRecord(req.Plan, req.ReviewerID, req.IssuedAt, req.ExpiresAt, s.signer.SignerKeyID)
	if err != nil {
		return StreamingRecord{}, err
	}
	if !s.now().Before(record.ExpiresAt) {
		return StreamingRecord{}, unsafeError(errExpired)
	}
	if err := s.requireUnambiguousStreaming(req.Plan.ID); err != nil {
		return StreamingRecord{}, err
	}
	if err := s.writeStreaming(ctx, record); err != nil {
		return StreamingRecord{}, err
	}
	return record, nil
}

// RevokeStreaming re-signs an existing streaming record with a revocation time.
func (s *Store) RevokeStreaming(ctx context.Context, plan contracts.Plan, revokedAt time.Time) (StreamingRecord, error) {
	if ctx == nil {
		return StreamingRecord{}, contracts.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return StreamingRecord{}, err
	}
	if s.signer == nil {
		return StreamingRecord{}, unsafeError(errNoSigner)
	}
	if revokedAt.IsZero() {
		return StreamingRecord{}, contracts.ErrInvalid
	}
	record, err := s.loadStreaming(ctx, plan)
	if err != nil {
		return StreamingRecord{}, err
	}
	if record.SignerKeyID != s.signer.SignerKeyID {
		return StreamingRecord{}, unsafeError(errForeignSigner)
	}
	if record.RevokedAt != nil {
		return StreamingRecord{}, conflictError(errAlreadyRevoked)
	}
	at := revokedAt.UTC()
	record.RevokedAt = &at
	if err := s.writeStreaming(ctx, record); err != nil {
		return StreamingRecord{}, err
	}
	return record, nil
}

// VerifyStreaming authenticates the stored streaming record for plan and proves
// that approval is exactly the verified record's binding. It never falls back
// to a legacy record.
func (s *Store) VerifyStreaming(ctx context.Context, plan contracts.Plan, approval contracts.StreamingApproval) (StreamingRecord, error) {
	record, err := s.loadStreaming(ctx, plan)
	if err != nil {
		return StreamingRecord{}, err
	}
	if err := record.checkActive(s.now()); err != nil {
		return StreamingRecord{}, err
	}
	if !streamingApprovalEqual(record.StreamingApproval(), approval) {
		return StreamingRecord{}, unsafeError(errBinding)
	}
	return record, nil
}

// LoadStreamingApproval loads, authenticates and returns the protocol-bound
// approval for plan. It is the explicit issuance/read surface; it never writes.
func (s *Store) LoadStreamingApproval(ctx context.Context, plan contracts.Plan) (contracts.StreamingApproval, error) {
	record, err := s.loadStreaming(ctx, plan)
	if err != nil {
		return contracts.StreamingApproval{}, err
	}
	if err := record.checkActive(s.now()); err != nil {
		return contracts.StreamingApproval{}, err
	}
	return record.StreamingApproval(), nil
}

func streamingApprovalEqual(a, b contracts.StreamingApproval) bool {
	if a.BindingRevision != b.BindingRevision || a.PlanID != b.PlanID ||
		a.SourceSnapshotID != b.SourceSnapshotID || a.TargetID != b.TargetID ||
		a.SchemaDigest != b.SchemaDigest || a.ConfigDigest != b.ConfigDigest ||
		a.StrategyID != b.StrategyID || a.StrategyVersion != b.StrategyVersion ||
		a.KeyID != b.KeyID || !a.IssuedAt.Equal(b.IssuedAt) ||
		!a.ExpiresAt.Equal(b.ExpiresAt) || a.Revoked != b.Revoked ||
		len(a.Scopes) != len(b.Scopes) {
		return false
	}
	for i := range a.Scopes {
		if a.Scopes[i] != b.Scopes[i] {
			return false
		}
	}
	return true
}

func newStreamingRecord(plan contracts.Plan, reviewerID string, issuedAt, expiresAt time.Time, signerKeyID string) (StreamingRecord, error) {
	if err := plan.ValidateShape(); err != nil {
		return StreamingRecord{}, err
	}
	if err := contracts.ValidatePlanIdentity(plan); err != nil {
		return StreamingRecord{}, err
	}
	if reviewerID == "" || len(reviewerID) > maxIdentityBytes ||
		signerKeyID == "" || len(signerKeyID) > maxIdentityBytes {
		return StreamingRecord{}, contracts.ErrInvalid
	}
	if issuedAt.IsZero() || expiresAt.IsZero() || !expiresAt.After(issuedAt) {
		return StreamingRecord{}, contracts.ErrInvalid
	}
	digest, err := CanonicalPlanDigest(plan)
	if err != nil {
		return StreamingRecord{}, err
	}
	record := StreamingRecord{
		Version:         StreamingRecordVersion,
		Protocol:        contracts.StreamingProtocolRevision,
		BindingRevision: contracts.ApprovalBindingRevision,
		PlanID:          plan.ID, SourceSnapshotID: plan.SourceSnapshotID,
		TargetID: plan.TargetID, SchemaDigest: plan.SchemaDigest,
		ConfigDigest: plan.ConfigDigest, PlanContentDigest: digest,
		ReviewerID: reviewerID, SignerKeyID: signerKeyID,
		IssuedAt: issuedAt.UTC(), ExpiresAt: expiresAt.UTC(),
	}
	for _, table := range plan.Tables {
		for _, column := range table.Columns {
			if record.StrategyID == "" {
				record.StrategyID = column.Strategy.ID
				record.StrategyVersion = column.Strategy.Version
				record.KeyID = column.KeyID
			}
			record.Scopes = append(record.Scopes, StreamingScope{
				Table: table.Table, Column: column.Column.Column,
				Scope: column.Scope, NormalizationVersion: column.NormalizationVersion,
				Type: column.Type,
			})
		}
	}
	if err := record.checkBinding(plan); err != nil {
		return StreamingRecord{}, err
	}
	return record, nil
}

func encodeStreamingRecord(record StreamingRecord, private ed25519.PrivateKey) ([]byte, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, unsafeError(errNoSigner)
	}
	payload, err := record.canonical()
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(private, payload)
	raw := make([]byte, 0, len(streamingMagicLine)+2*ed25519.SignatureSize+len(payload)+2)
	raw = append(raw, streamingMagicLine...)
	raw = append(raw, '\n')
	raw = append(raw, hex.EncodeToString(signature)...)
	raw = append(raw, '\n')
	raw = append(raw, payload...)
	return raw, nil
}

func decodeStreamingRecord(raw []byte) (StreamingRecord, []byte, []byte, error) {
	if len(raw) == 0 || len(raw) > maxRecordBytes {
		return StreamingRecord{}, nil, nil, unsafeError(errCorrupt)
	}
	header, body, ok := bytes.Cut(raw, []byte("\n"))
	if !ok || string(header) != streamingMagicLine {
		return StreamingRecord{}, nil, nil, unsafeError(errCorrupt)
	}
	encodedSignature, payload, ok := bytes.Cut(body, []byte("\n"))
	if !ok || len(payload) == 0 {
		return StreamingRecord{}, nil, nil, unsafeError(errCorrupt)
	}
	signature, err := hex.DecodeString(string(encodedSignature))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return StreamingRecord{}, nil, nil, unsafeError(errCorrupt)
	}
	var wire canonicalStreamingRecord
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return StreamingRecord{}, nil, nil, unsafeError(errCorrupt)
	}
	issuedAt, err := parseTimestamp(wire.IssuedAt)
	if err != nil {
		return StreamingRecord{}, nil, nil, err
	}
	expiresAt, err := parseTimestamp(wire.ExpiresAt)
	if err != nil {
		return StreamingRecord{}, nil, nil, err
	}
	record := StreamingRecord{
		Version: wire.Version, Protocol: wire.Protocol, BindingRevision: wire.BindingRevision,
		PlanID: wire.PlanID, SourceSnapshotID: wire.SourceSnapshotID,
		TargetID: wire.TargetID, SchemaDigest: wire.SchemaDigest,
		ConfigDigest: wire.ConfigDigest, PlanContentDigest: wire.PlanContentDigest,
		StrategyID: wire.StrategyID, StrategyVersion: wire.StrategyVersion,
		KeyID: wire.KeyID, ReviewerID: wire.ReviewerID,
		SignerKeyID: wire.SignerKeyID, IssuedAt: issuedAt, ExpiresAt: expiresAt,
	}
	for _, scope := range wire.Scopes {
		record.Scopes = append(record.Scopes, StreamingScope{
			Table: contracts.TableRef{
				Database: scope.Database, Schema: scope.Schema, Table: scope.Table,
			},
			Column: scope.Column, Scope: scope.Scope,
			NormalizationVersion: scope.NormalizationVersion,
			Type: contracts.Type{
				Kind: contracts.Kind(scope.TypeKind), Native: scope.TypeNative,
				Nullable: scope.TypeNullable, Length: scope.TypeLength,
				Precision: scope.TypePrecision, Scale: scope.TypeScale,
				Collation: scope.TypeCollation,
			},
		})
	}
	if wire.RevokedAt != "" {
		revokedAt, err := parseTimestamp(wire.RevokedAt)
		if err != nil {
			return StreamingRecord{}, nil, nil, err
		}
		record.RevokedAt = &revokedAt
	}
	if err := record.checkShape(); err != nil {
		return StreamingRecord{}, nil, nil, err
	}
	reencoded, err := record.canonical()
	if err != nil {
		return StreamingRecord{}, nil, nil, err
	}
	if !bytes.Equal(reencoded, payload) {
		return StreamingRecord{}, nil, nil, unsafeError(errCorrupt)
	}
	return record, payload, signature, nil
}

func (s *Store) loadStreaming(ctx context.Context, plan contracts.Plan) (StreamingRecord, error) {
	if err := plan.ValidateShape(); err != nil {
		return StreamingRecord{}, err
	}
	path, err := streamingRecordPath(s.dir, plan.ID)
	if err != nil {
		return StreamingRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return StreamingRecord{}, err
	}
	if err := s.requireUnambiguousStreaming(plan.ID); err != nil {
		return StreamingRecord{}, err
	}
	raw, err := readCapped(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return StreamingRecord{}, contracts.ErrNotFound
		}
		if errors.Is(err, contracts.ErrUnsafe) {
			return StreamingRecord{}, err
		}
		return StreamingRecord{}, unsafeError(errRead)
	}
	record, payload, signature, err := decodeStreamingRecord(raw)
	if err != nil {
		return StreamingRecord{}, err
	}
	public, trusted := s.trust.lookup(record.SignerKeyID)
	if !trusted {
		return StreamingRecord{}, unsafeError(errUnknownSigner)
	}
	if !ed25519.Verify(public, payload, signature) {
		return StreamingRecord{}, unsafeError(errSignature)
	}
	if err := record.checkBinding(plan); err != nil {
		return StreamingRecord{}, err
	}
	return record, nil
}

func (s *Store) writeStreaming(ctx context.Context, record StreamingRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := encodeStreamingRecord(record, s.signer.Private)
	if err != nil {
		return err
	}
	target, err := streamingRecordPath(s.dir, record.PlanID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, storeDirPerm); err != nil {
		return unsafeError(errWrite)
	}
	staged, err := os.CreateTemp(s.dir, "."+filepath.Base(target)+".tmp-")
	if err != nil {
		return unsafeError(errWrite)
	}
	stagedName := staged.Name()
	discard := func() {
		_ = staged.Close()
		_ = os.Remove(stagedName)
	}
	if err := staged.Chmod(storeFilePerm); err != nil {
		discard()
		return unsafeError(errWrite)
	}
	if _, err := staged.Write(raw); err != nil {
		discard()
		return unsafeError(errWrite)
	}
	if err := staged.Sync(); err != nil {
		discard()
		return unsafeError(errWrite)
	}
	if err := staged.Close(); err != nil {
		_ = os.Remove(stagedName)
		return unsafeError(errWrite)
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(stagedName)
		return err
	}
	if err := s.renameIntoPlace(stagedName, target, raw); err != nil {
		_ = os.Remove(stagedName)
		return unsafeError(errWrite)
	}
	return s.syncDir()
}

func streamingRecordPath(dir, planID string) (string, error) {
	if !safePlanID(planID) {
		return "", contracts.ErrInvalid
	}
	return filepath.Join(dir, planID+streamingSuffix), nil
}

func claimedStreamingPlanID(raw []byte) (string, bool) {
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

// requireUnambiguousStreaming is the streaming counterpart of
// requireUnambiguous: it scans the whole directory through the bounded
// enumerator, and any other streaming file claiming the same plan is ambiguous.
func (s *Store) requireUnambiguousStreaming(planID string) error {
	canonical := planID + streamingSuffix
	return s.eachEntry(context.Background(), func(entry fs.DirEntry) error {
		if entry.IsDir() || entry.Name() == canonical ||
			!strings.HasSuffix(entry.Name(), streamingSuffix) {
			return nil
		}
		raw, err := readCapped(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil
		}
		if claimed, ok := claimedStreamingPlanID(raw); ok && claimed == planID {
			return conflictError(errAmbiguous)
		}
		return nil
	})
}

// Bounded directory enumeration.
//
// os.ReadDir materialises every entry of the directory before returning, so a
// hostile directory could make memory grow with entry count. eachEntry reads
// at most maxHistoryChunk entries at a time and fails closed when the entry
// count or a candidate record name exceeds the configured bound. Context
// cancellation ends the enumeration.
const (
	maxHistoryChunk          = 64
	defaultMaxHistoryEntries = 4096
	defaultMaxHistoryNameLen = 4096
)

func (s *Store) eachEntry(ctx context.Context, fn func(fs.DirEntry) error) error {
	maxEntries := s.maxEntries
	if maxEntries <= 0 {
		maxEntries = defaultMaxHistoryEntries
	}
	maxNameLen := s.maxNameBytes
	if maxNameLen <= 0 {
		maxNameLen = defaultMaxHistoryNameLen
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return unsafeError(errRead)
	}
	defer func() { _ = dir.Close() }()
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := dir.ReadDir(maxHistoryChunk)
		if err != nil && !errors.Is(err, io.EOF) {
			return unsafeError(errRead)
		}
		for _, entry := range chunk {
			total++
			if total > maxEntries {
				return unsafeError(errRead)
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(entry.Name()) > maxNameLen &&
				(strings.HasSuffix(entry.Name(), recordSuffix) ||
					strings.HasSuffix(entry.Name(), streamingSuffix)) {
				return unsafeError(errRead)
			}
			if err := fn(entry); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) || len(chunk) < maxHistoryChunk {
			return nil
		}
	}
}

// errNoStreamingRecord keeps not-found distinguishable from a malformed store.
var errNoStreamingRecord = errors.New("no streaming approval record")

func streamingNotFound() error {
	return fmt.Errorf("%w: %w", contracts.ErrNotFound, errNoStreamingRecord)
}

// checkActive applies the same clock rules as a legacy record: a revoked,
// expired or implausibly future-issued streaming record is refused.
func (r StreamingRecord) checkActive(now time.Time) error {
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
