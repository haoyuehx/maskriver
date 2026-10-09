package contracts

import (
	"context"
	"errors"
	"time"
)

// Export only sanitized category errors. Do not wrap raw driver errors in reports.
var (
	ErrInvalid       = errors.New("invalid contract input")
	ErrUnsupported   = errors.New("unsupported capability")
	ErrIncomplete    = errors.New("incomplete coverage")
	ErrUnsafe        = errors.New("unsafe operation refused")
	ErrConflict      = errors.New("conflicting state")
	ErrNotFound      = errors.New("object not found")
	ErrClosed        = errors.New("resource closed")
	ErrDatabase      = errors.New("database operation failed")
	ErrCommitUnknown = errors.New("transaction commit outcome unknown")
)

// DatabaseOptions is provided by Main; DSN must be a Text Value and never logged.
// DatasetID is the logical TableRef.Database identity shared by snapshot and target.
type DatabaseOptions struct {
	Dialect        Dialect
	DSN            Value `json:"-"`
	DatasetID      string
	Schemas        []string
	MaxOpenConns   int
	ConnectTimeout time.Duration
}
type Dialect uint8

const (
	DialectUnknown Dialect = iota
	SQLite
	MySQL
)

type TableRef struct{ Database, Schema, Table string }
type ColumnRef struct {
	TableRef
	Column string
}

// Type describes declared SQL semantics; Kind is the required Value encoding.
// Length/Precision/Scale zero means unavailable unless the adapter documents it.
type Type struct {
	Kind                     Kind
	Native                   string
	Nullable                 bool
	Length, Precision, Scale int
	Collation                string
}
type Column struct {
	Ref  ColumnRef
	Type Type
}
type Index struct {
	Name    string
	Columns []string
	Unique  bool
}
type ForeignKey struct {
	Columns            []string
	Target             TableRef
	TargetColumns      []string
	OnUpdate, OnDelete string
}

// Known=false is not equivalent to an empty list of constraints.
type SchemaCoverage struct{ Columns, PrimaryKey, Indexes, ForeignKeys, Unique, Checks bool }
type TableSchema struct {
	Ref         TableRef
	Columns     []Column
	PrimaryKey  []string
	Indexes     []Index
	ForeignKeys []ForeignKey
	Unique      [][]string
	Checks      []string
	Coverage    SchemaCoverage
}

type Row map[string]Value
type SampleRequest struct {
	Column ColumnRef
	Limit  int
}
type Sample struct {
	Values    []Value `json:"-"`
	Basis     SampleBasis
	Truncated bool
}
type SampleBasis uint8

const (
	BasisUnknown SampleBasis = iota
	DistinctNonNull
)

type PageRequest struct {
	Table      TableRef
	Columns    []string
	KeyColumns []string
	After      []Value `json:"-"`
	Limit      int
}
type Page struct {
	Rows []Row   `json:"-"`
	Next []Value `json:"-"`
	Done bool
}

// Reader never creates a database, mapping store, audit file, or transaction write.
// Page returns a fully materialized bounded page with its SQL cursor closed.
type Reader interface {
	Tables(context.Context, []string) ([]TableRef, error)
	Describe(context.Context, TableRef) (TableSchema, error)
	Sample(context.Context, SampleRequest) (Sample, error)
	Page(context.Context, PageRequest) (Page, error)
	Count(context.Context, TableRef) (int64, error)
	Close() error
}

type Sensitivity uint8

const (
	Unknown Sensitivity = iota
	NotSensitive
	Sensitive
)

type DecisionSource uint8

const (
	SourceUnknown DecisionSource = iota
	Manual
	ReviewedHistory
	Rule
	Skip
)

type Evidence struct {
	RuleID      string
	Hits, Total int
	Threshold   float64
	Eligible    bool
	Reasons     []IssueCode
}
type Decision struct {
	Column      ColumnRef
	Type        Type
	Sensitivity Sensitivity
	Source      DecisionSource
	RuleID      string
	Strategy    StrategyRef
	SampleBasis SampleBasis
	Evidence    []Evidence
}
type DetectionRequest struct {
	Column     Column
	Sample     Sample
	MinSamples int
	MinRatio   float64
	DateOrder  string // Exactly MDY or DMY.
	Override   *Decision
	Reviewed   *Decision // Main/history has already checked approval, expiry and type.
	Skip       bool
}
type Detector interface {
	Detect(context.Context, DetectionRequest) (Decision, error)
}

type StrategyRef struct{ ID, Version string }
type ColumnPlan struct {
	Column               ColumnRef
	Type                 Type
	Strategy             StrategyRef
	Scope                string
	NormalizationVersion string
	KeyID                string // Opaque ID, never the secret key itself.
}
type TablePlan struct {
	Table      TableRef
	PrimaryKey []string
	Columns    []ColumnPlan
}

// Plan is Main-owned and immutable after publication by convention.
// It is not an authorization token; runner must validate/recheck every field.
type Plan struct {
	Revision                                               string
	ID                                                     string // Canonical plan digest computed by Main, not a user-supplied label.
	SourceSnapshotID, TargetID, SchemaDigest, ConfigDigest string
	ScanComplete                                           bool
	Unresolved                                             []ColumnRef
	Tables                                                 []TablePlan
}
type MaskContext struct {
	Plan ColumnPlan
	Key  Value `json:"-"` // Bytes with >=32 bytes; never string/JSON serialized.
}

// Strategy must be deterministic for equal typed input and context, with no I/O.
type Strategy interface {
	Ref() StrategyRef
	Mask(context.Context, Value, MaskContext) (Value, error)
}

// Mapping keys are HMAC fingerprints, never source values or plain hashes.
type MappingKey struct {
	Strategy                           StrategyRef
	Scope, NormalizationVersion, KeyID string
	Fingerprint                        [32]byte
}
type MappingReader interface {
	Lookup(context.Context, MappingKey) (Value, bool, error)
}
type MappingWriter interface {
	// GetOrCreate is atomic; return the stored winner, which may differ from candidate.
	GetOrCreate(context.Context, MappingKey, Value) (Value, error)
}

type RowChange struct {
	Key         []Value `json:"-"`
	Expected    Row     `json:"-"` // Original values for all changed columns (optimistic guard).
	Replacement Row     `json:"-"`
}

// Writer is only supplied to runner's explicit apply path, never scan/preview.
type Writer interface {
	Begin(context.Context, TablePlan) (WriteTx, error)
	Close() error
}
type WriteTx interface {
	// Update checks exactly one row per key and all Expected values, else ErrConflict.
	Update(context.Context, []RowChange) (int64, error)
	Commit(context.Context) error
	// Rollback must remain callable after cancellation; use a separate bounded context.
	Rollback(context.Context) error
}

type IssueCode string

const (
	InvalidInput         IssueCode = "invalid_input"
	Unsupported          IssueCode = "unsupported"
	InsufficientEvidence IssueCode = "insufficient_evidence"
	ConflictingEvidence  IssueCode = "conflicting_evidence"
	ScanFailed           IssueCode = "scan_failed"
	UnresolvedColumn     IssueCode = "unresolved_column"
	MissingKey           IssueCode = "missing_key"
	KeyMismatch          IssueCode = "key_mismatch"
	SchemaMismatch       IssueCode = "schema_mismatch"
	RowCountMismatch     IssueCode = "row_count_mismatch"
	UnchangedValue       IssueCode = "unchanged_value"
	PartialCoverage      IssueCode = "partial_coverage"
	DatabaseFailure      IssueCode = "database_failure"
	Cancelled            IssueCode = "cancelled"
	CommitUnknown        IssueCode = "commit_unknown"
)

type Status uint8

const (
	StatusUnknown Status = iota
	Pass
	Fail
	Warning
	Error
)

// Issue has no arbitrary message/sample/key field, to limit accidental disclosure.
type Issue struct {
	Status Status
	Code   IssueCode
	Column ColumnRef
	Count  int64
}
type Coverage struct {
	Complete              bool
	Tables, Columns, Rows int64
}
type ValidationReport struct {
	PlanID   string
	Coverage Coverage
	Issues   []Issue
}

func (r ValidationReport) Passed(strict bool) bool {
	if !r.Coverage.Complete || r.PlanID == "" || r.Coverage.Tables <= 0 || r.Coverage.Columns <= 0 || r.Coverage.Rows < 0 {
		return false
	}
	for _, issue := range r.Issues {
		if issue.Status != Pass && !(issue.Status == Warning && !strict) {
			return false
		}
	}
	return true
}

type ValidateRequest struct {
	Plan     Plan
	RowLimit int64
	Strict   bool
}
type Validator interface {
	Validate(context.Context, Reader, Reader, ValidateRequest) (ValidationReport, error)
}

// Workflow interfaces are runner targets, not implementations supplied in M1-A.
type ScanRequest struct {
	Tables                  []TableRef
	SampleLimit, MinSamples int
	MinRatio                float64
	DateOrder               string
}
type ScanReport struct {
	Decisions []Decision
	Coverage  Coverage
	Issues    []Issue
}
type PreviewRequest struct {
	Plan          Plan
	LimitPerTable int
}
type PreviewReport struct {
	PlanID   string
	RowsRead int64
	Coverage Coverage
	Issues   []Issue
}
type ApplyRequest struct {
	Plan          Plan
	ExplicitApply bool
	BatchSize     int
}
type BatchReceipt struct {
	Table         TableRef
	Ordinal, Rows int64
}
type ApplyReport struct {
	PlanID               string
	Committed            []BatchReceipt
	CommitOutcomeUnknown bool
	Complete             bool
	Issues               []Issue
}

// Workflow receives opened dependencies at construction, owned/closed by Main.
// Preview deliberately returns no original or transformed row payloads in v1.
type Workflow interface {
	Scan(context.Context, ScanRequest) (ScanReport, error)
	Preview(context.Context, PreviewRequest) (PreviewReport, error)
	Apply(context.Context, ApplyRequest) (ApplyReport, error)
	Validate(context.Context, ValidateRequest) (ValidationReport, error)
}
