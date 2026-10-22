package main

// Explicit streaming (m2-stream-v1) CLI surface.
//
// This is additive: the default `mask` flow and its m1a-v1 semantics are
// unchanged. `mask --streaming --apply` requires its own budget file, its own
// protocol-bound approval and an already provisioned outbox schema. No flag,
// environment variable or config file enables writes on its own, and the
// streaming path never falls back to the legacy path.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/internal/history"
	"github.com/haoyuehx/maskriver/internal/runner"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const maxBudgetFileBytes = 64 << 10

// loadResourceBudget reads an explicit, complete ResourceBudget from a local
// file. There are no defaults: every field must be present, and unknown fields
// are refused. This keeps an unapproved experimental budget from masquerading as
// a production default.
func loadResourceBudget(path string) (contracts.ResourceBudget, error) {
	var budget contracts.ResourceBudget
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxBudgetFileBytes {
		return budget, contracts.ErrInvalid
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return budget, contracts.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&budget); err != nil {
		return contracts.ResourceBudget{}, contracts.ErrInvalid
	}
	if err := budget.Validate(); err != nil {
		return contracts.ResourceBudget{}, err
	}
	return budget, nil
}

// runIDFlag returns the caller-supplied 32-hex run identity or a fresh random
// one. The identity is bounded and contains no business data.
func resolveRunID(value string) (string, error) {
	if value != "" {
		if !contracts.ValidRunID(value) {
			return "", contracts.ErrInvalid
		}
		return value, nil
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", contracts.ErrUnsafe
	}
	return hex.EncodeToString(raw[:]), nil
}

// streamingApplyOrResume dispatches the explicit streaming entry points. A
// resume never generates a run identity and never falls back to a new run.
func streamingApplyOrResume(ctx context.Context, engine *runner.Engine, store *history.Store, plan contracts.Plan,
	cfg config.Runtime, budget contracts.ResourceBudget, runID string, receiptPage int, resume bool,
	stdout, stderr io.Writer) int {
	if resume {
		return streamingResume(ctx, engine, store, plan, cfg, budget, runID, receiptPage, stdout, stderr)
	}
	return streamingApply(ctx, engine, store, plan, cfg, budget, runID, receiptPage, stdout, stderr)
}

// streamingResume loads the stored streaming approval and continues an existing
// run. It prints only aggregate state; no value, key or DSN is ever included.
func streamingResume(ctx context.Context, engine *runner.Engine, store *history.Store, plan contracts.Plan,
	cfg config.Runtime, budget contracts.ResourceBudget, runID string, receiptPage int,
	stdout, stderr io.Writer) int {
	approval, err := store.LoadStreamingApproval(ctx, plan)
	if err != nil {
		fmt.Fprintln(stderr, "resume: refused=true reason=approval")
		return resumeExitCode(err, contracts.StreamingApplySummary{}, runner.RecoveryReport{})
	}
	engine.SetStreamingApprovalQuery(store)
	summary, report, err := engine.StreamingResume(ctx, runner.StreamingRecoveryRequest{
		Protocol: contracts.StreamingProtocolRevision, Plan: plan, Approval: approval,
		ExplicitApply: true, RunID: runID, BatchSize: cfg.BatchSize,
		ReceiptPageSize: receiptPage, Budget: budget,
	})
	write := stdout
	if err != nil {
		write = stderr
	}
	fmt.Fprintf(write, "resume: protocol=%s run=%s plan=%s\n", summary.Protocol, summary.RunID, summary.PlanID)
	fmt.Fprintf(write, "resume: state=%s validation=%s rows_committed=%d batches_total=%d batches_committed=%d batches_failed=%d batches_unknown=%d\n",
		runStateName(summary.State), string(report.Validation), summary.RowsCommitted,
		summary.BatchesTotal, summary.BatchesCommitted, summary.BatchesFailed, summary.BatchesUnknown)
	fmt.Fprintf(write, "resume: checkpoint_sequence=%d table_ordinal=%d tables=%d rows_processed=%d business_complete=%t\n",
		report.CheckpointSequence, report.TableOrdinal, report.Tables, report.RowsProcessed, report.BusinessComplete)
	fmt.Fprintf(write, "resume: audit=%s resume_eligible=%t receipts_bounded=true\n",
		auditStateName(summary.Audit), report.ResumeEligible)
	if summary.State == contracts.RunStateCommitUnknown || report.State == db.RecoveryStateCommitUnknown {
		fmt.Fprintln(write, "resume: commit_unknown=true retry=never")
	}
	if report.State == db.RecoveryStateConflict {
		fmt.Fprintln(write, "resume: conflict=true overwrite=never")
	}
	return resumeExitCode(err, summary, report)
}

// resumeExitCode maps the result to the documented CLI exit codes. A partial or
// failed resume is never reported as success.
func resumeExitCode(err error, summary contracts.StreamingApplySummary, report runner.RecoveryReport) int {
	if err == nil && summary.State == contracts.RunStateComplete {
		return 0
	}
	switch {
	case errors.Is(err, contracts.ErrCommitUnknown), report.State == db.RecoveryStateCommitUnknown:
		return 5
	case errors.Is(err, contracts.ErrConflict), report.State == db.RecoveryStateConflict:
		return 4
	case errors.Is(err, contracts.ErrUnsupported):
		return 6
	case errors.Is(err, contracts.ErrNotFound):
		return 7
	case errors.Is(err, contracts.ErrUnsafe):
		return 8
	case errors.Is(err, contracts.ErrIncomplete), report.State == db.RecoveryStateValidationFailed:
		return 9
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 3
	case err == nil && (summary.State == contracts.RunStateRunning ||
		report.State == db.RecoveryStateRecoverable || report.State == db.RecoveryStateRecoveryRequired):
		return 3
	default:
		return 1
	}
}

// streamingApply runs the explicit streaming path. The approval is loaded from
// the store through the same signed binding the verifier checks, so a
// hand-written approval struct can not authorize a write.
func streamingApply(ctx context.Context, engine *runner.Engine, store *history.Store, plan contracts.Plan,
	cfg config.Runtime, budget contracts.ResourceBudget, runID string, receiptPage int,
	stdout, stderr io.Writer) int {
	runID, err := resolveRunID(runID)
	if err != nil {
		return fail(stderr, 2)
	}
	approval, err := store.LoadStreamingApproval(ctx, plan)
	if err != nil {
		return fail(stderr, 1)
	}
	engine.SetStreamingApprovalQuery(store)
	summary, err := engine.StreamingApply(ctx, contracts.StreamingApplyRequest{
		Protocol: contracts.StreamingProtocolRevision,
		Plan:     plan, Approval: approval, ExplicitApply: true,
		RunID: runID, BatchSize: cfg.BatchSize, ReceiptPageSize: receiptPage,
		Budget: budget,
	})
	// Every emitted line is an aggregate: no value, replacement, key, DSN or
	// host path is included. An unknown commit is displayed as unknown and is
	// never presented as retryable success.
	pages := int64(0)
	if summary.ReceiptPageSize > 0 {
		pages = (summary.BatchesTotal + int64(summary.ReceiptPageSize) - 1) / int64(summary.ReceiptPageSize)
	}
	lines := []string{
		fmt.Sprintf("streaming: protocol=%s run=%s plan=%s", summary.Protocol, summary.RunID, summary.PlanID),
		fmt.Sprintf("streaming: state=%s audit=%s rows_committed=%d batches_total=%d batches_committed=%d batches_failed=%d batches_unknown=%d",
			runStateName(summary.State), auditStateName(summary.Audit), summary.RowsCommitted,
			summary.BatchesTotal, summary.BatchesCommitted, summary.BatchesFailed, summary.BatchesUnknown),
		fmt.Sprintf("streaming: receipts page_size=%d pages=%d bounded=true", summary.ReceiptPageSize, pages),
		fmt.Sprintf("streaming: budget_status=experimental issue_categories=%d", len(summary.IssueCounts)),
	}
	if summary.State == contracts.RunStateCommitUnknown {
		lines = append(lines, "streaming: commit_unknown=true retry=never")
	}
	for _, line := range lines {
		if err != nil {
			fmt.Fprintln(stderr, line)
			continue
		}
		fmt.Fprintln(stdout, line)
	}
	if err != nil {
		return 1
	}
	if summary.State != contracts.RunStateComplete || summary.Audit != contracts.AuditStateConsistent {
		return 1
	}
	return 0
}

func runStateName(state contracts.RunState) string {
	switch state {
	case contracts.RunStateRunning:
		return "running"
	case contracts.RunStateComplete:
		return "complete"
	case contracts.RunStateIncomplete:
		return "incomplete"
	case contracts.RunStateCommitUnknown:
		return "commit_unknown"
	case contracts.RunStateAuditIncomplete:
		return "audit_incomplete"
	default:
		return "unknown"
	}
}

func auditStateName(state contracts.AuditState) string {
	switch state {
	case contracts.AuditStateConsistent:
		return "consistent"
	case contracts.AuditStateIncomplete:
		return "incomplete"
	default:
		return "unknown"
	}
}

// runInitOutbox is a separate, explicit management entry point for the outbox
// schema. It is never reachable from preview or apply and requires the operator
// to name this command.
func runInitOutbox(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init-outbox", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var target string
	flags.StringVar(&target, "target", "", "existing absolute SQLite target database path")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: maskriver init-outbox [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(stderr)
			flags.Usage()
			return 0
		}
		return usageError(stderr)
	}
	if flags.NArg() != 0 || target == "" {
		return usageError(stderr)
	}
	path, err := filepath.Abs(target)
	if err != nil {
		return fail(stderr, 2)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fail(stderr, 2)
	}
	value, err := contracts.NewValue(contracts.Text, path)
	if err != nil {
		return fail(stderr, 2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err = db.InstallOutboxSchema(ctx, db.OutboxOptions{
		DatabaseOptions: contracts.DatabaseOptions{
			Dialect: contracts.SQLite, DSN: value, DatasetID: "maskriver",
			Schemas: []string{"main"}, MaxOpenConns: 2, ConnectTimeout: 5 * time.Second,
		},
		Protocol: contracts.StreamingProtocolRevision, JournalMode: "delete", MaxReceiptPage: 1,
	})
	if err != nil {
		return fail(stderr, 1)
	}
	fmt.Fprintf(stdout, "init-outbox: installed=true schema=%s protocol=%s\n",
		db.OutboxSchemaVersion, contracts.StreamingProtocolRevision)
	return 0
}
