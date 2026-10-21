package main

// Explicit recovery management and read-only status.
//
// `init-recovery` is the only command that may install the recovery schema; no
// apply/status path executes DDL. `status` opens the target read-only and never
// recovers, writes, installs or repairs anything.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/haoyuehx/maskriver/internal/db"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// runInitRecovery installs the additive recovery schema on a database that
// already has the outbox v1 schema. It never creates the outbox itself; an
// exact version/shape is a no-op and anything else fails closed.
func runInitRecovery(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init-recovery", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var target string
	flags.StringVar(&target, "target", "", "existing absolute SQLite target database path")
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
	opts := db.OutboxOptions{
		DatabaseOptions: contracts.DatabaseOptions{
			Dialect: contracts.SQLite, DSN: value, DatasetID: "maskriver",
			Schemas: []string{"main"}, MaxOpenConns: 2, ConnectTimeout: 5 * time.Second,
		},
		Protocol: contracts.StreamingProtocolRevision, JournalMode: "delete", MaxReceiptPage: 1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// Recovery is additive to an already-provisioned outbox v1 schema. Requiring
	// it prevents `init-recovery --target source.db` from silently adding
	// metadata tables to a source of truth.
	if err := db.InstallRecoverySchema(ctx, opts); err != nil {
		return fail(stderr, 1)
	}
	fmt.Fprintf(stdout, "init-recovery: outbox=%s recovery=%s\n",
		db.OutboxSchemaVersion, db.RecoverySchemaVersion)
	return 0
}

// runStatus prints the sanitized recovery state of one run. It uses a read-only
// SQLite connection and a fixed set of aggregate fields.
func runStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var target, runID string
	flags.StringVar(&target, "target", "", "existing absolute SQLite target database path")
	flags.StringVar(&runID, "run-id", "", "32-hex run identity")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(stderr)
			flags.Usage()
			return 0
		}
		return usageError(stderr)
	}
	if flags.NArg() != 0 || target == "" || runID == "" || !contracts.ValidRunID(runID) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader, err := db.OpenRecoveryReader(ctx, contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: value, DatasetID: "maskriver",
		Schemas: []string{"main"}, MaxOpenConns: 1, ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		reason := "recovery_schema"
		if errors.Is(err, db.ErrHotJournal) {
			reason = "hot_journal"
		}
		fmt.Fprintf(stderr, "status: unavailable=true reason=%s\n", reason)
		return statusExitCode(err)
	}
	defer reader.Close()
	manifest, err := reader.Manifest(ctx, runID)
	if err != nil {
		fmt.Fprintln(stderr, "status: unavailable=true reason=manifest")
		return statusExitCode(err)
	}
	progress, hasProgress, err := reader.LatestProgress(ctx, runID)
	if err != nil {
		fmt.Fprintln(stderr, "status: unavailable=true reason=progress")
		return statusExitCode(err)
	}
	receipts, err := reader.Receipts(ctx, runID)
	if err != nil {
		fmt.Fprintln(stderr, "status: unavailable=true reason=receipts")
		return statusExitCode(err)
	}
	eligible := manifest.State != db.RecoveryStateConflict &&
		manifest.State != db.RecoveryStateAborted &&
		manifest.State != db.RecoveryStateValidationFailed
	fmt.Fprintf(stdout, "status: run=%s protocol=%s state=%s validation=%s\n",
		manifest.RunID, string(manifest.Protocol), string(manifest.State), string(manifest.Validation))
	fmt.Fprintf(stdout, "status: checkpoint_sequence=%d committed_batches=%d rows_committed=%d\n",
		manifest.CheckpointSequence, progress.CommittedBatches, progress.RowsCommitted)
	fmt.Fprintf(stdout, "status: table_ordinal=%d rows_processed=%d business_complete=%t validation_complete=%t\n",
		progress.TableOrdinal, progress.RowsProcessed, manifest.BusinessComplete,
		manifest.Validation == db.ValidationPassed)
	fmt.Fprintf(stdout, "status: receipts=%d receipt_rows=%d audit=%s\n",
		receipts.Count, receipts.Rows, auditLabel(manifest))
	fmt.Fprintf(stdout, "status: resume_eligible=%t commit_unknown=%t conflict=%t has_progress=%t\n",
		eligible, manifest.State == db.RecoveryStateCommitUnknown,
		manifest.State == db.RecoveryStateConflict, hasProgress)
	return 0
}

func auditLabel(m db.RecoveryManifest) string {
	if m.Validation == db.ValidationPassed && m.BusinessComplete {
		return "complete"
	}
	if m.BusinessComplete {
		return "business_complete_validation_pending"
	}
	return "incomplete"
}

func statusExitCode(err error) int {
	switch {
	case errors.Is(err, db.ErrHotJournal):
		return 3
	case errors.Is(err, contracts.ErrUnsupported):
		return 6
	case errors.Is(err, contracts.ErrNotFound), errors.Is(err, contracts.ErrConflict),
		errors.Is(err, contracts.ErrInvalid):
		return 7
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 3
	default:
		return 1
	}
}
