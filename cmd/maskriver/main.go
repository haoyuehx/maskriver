// Command maskriver runs the bounded, SQLite-only M1 workflow.
// Inputs must be pre-existing, separate disposable database snapshots.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/haoyuehx/maskriver/internal/config"
	"github.com/haoyuehx/maskriver/internal/runner"
	"github.com/haoyuehx/maskriver/pkg/contracts"
)

const version = "0.1.0-dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stdout)
		return 0
	}
	switch args[0] {
	case "help", "-h", "--help":
		if len(args) != 1 {
			return usageError(stderr)
		}
		usage(stdout)
		return 0
	case "version", "--version":
		if len(args) != 1 {
			return usageError(stderr)
		}
		fmt.Fprintln(stdout, "MaskRiver", version)
		return 0
	case "scan", "mask", "validate":
	default:
		return usageError(stderr)
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	cfg := config.Runtime{}
	var source, target, keyFile string
	var apply, strict bool
	var strategy, keyID string
	flags.StringVar(&source, "source", "", "existing absolute SQLite source-snapshot path")
	flags.StringVar(&target, "target", "", "existing absolute SQLite target-copy path")
	flags.StringVar(&cfg.Table, "table", "", "explicit table allowlist (one table)")
	flags.StringVar(&cfg.Column, "column", "", "explicit sensitive column (one column)")
	flags.IntVar(&cfg.MinSamples, "min-samples", 20, "minimum distinct non-NULL evidence")
	flags.IntVar(&cfg.SampleLimit, "sample-limit", 128, "maximum distinct sampled inputs")
	flags.Float64Var(&cfg.MinRatio, "min-ratio", 0.9, "required rule hit ratio")
	flags.IntVar(&cfg.BatchSize, "batch-size", 64, "transaction batch size (1..1000)")
	if command != "scan" {
		flags.StringVar(&strategy, "strategy", "redact", "supported strategy identifier")
		flags.StringVar(&keyID, "key-id", "", "opaque ID for external Bytes key")
		flags.StringVar(&keyFile, "key-file", "", "local key bytes file, never echoed")
		flags.BoolVar(&cfg.Approved, "approved", false, "affirm a separately reviewed column decision")
	}
	if command == "mask" {
		flags.BoolVar(&apply, "apply", false, "explicitly request writes after full preview")
	}
	if command == "validate" {
		flags.BoolVar(&strict, "strict", true, "refuse any incomplete or warning coverage")
	}
	flags.Usage = func() {
		fmt.Fprintf(stderr, "Usage: maskriver %s [flags]\n", command)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return usageError(stderr)
	}
	var err error
	cfg.Source, err = filepath.Abs(source)
	if err != nil || source == "" {
		return fail(stderr, 2)
	}
	cfg.Target, err = filepath.Abs(target)
	if err != nil || target == "" {
		return fail(stderr, 2)
	}
	if err := cfg.Validate(); err != nil {
		return fail(stderr, 2)
	}
	cfg.Strategy = contracts.StrategyRef{ID: strategy, Version: "1"}
	cfg.KeyID = keyID
	if command == "mask" && apply && !cfg.Approved {
		return fail(stderr, 2) // rule classification alone never grants writes
	}
	var key contracts.Value
	if command != "scan" {
		if keyFile == "" || keyID == "" {
			return fail(stderr, 2)
		}
		info, err := os.Stat(keyFile)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 32 || info.Size() > 4096 {
			return fail(stderr, 2)
		}
		content, err := os.ReadFile(keyFile)
		if err != nil {
			return fail(stderr, 2)
		}
		key, err = contracts.NewValue(contracts.Bytes, string(content))
		if err != nil {
			return fail(stderr, 2)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	engine, err := runner.Open(ctx, cfg, key)
	if err != nil {
		return fail(stderr, 1)
	}
	defer engine.Close()
	if command == "scan" {
		report, err := engine.Scan(ctx, contracts.ScanRequest{})
		if err != nil {
			return fail(stderr, 1)
		}
		fmt.Fprintf(stdout, "scan: complete=%t columns=%d issues=%d\n",
			report.Coverage.Complete, report.Coverage.Columns, len(report.Issues))
		if !report.Coverage.Complete {
			return 1
		}
		return 0
	}
	plan, err := engine.Plan(ctx)
	if err != nil {
		return fail(stderr, 1)
	}
	if command == "validate" {
		report, err := engine.Validate(ctx, contracts.ValidateRequest{Plan: plan, Strict: strict})
		if err != nil {
			return fail(stderr, 1)
		}
		passed := report.Passed(strict)
		fmt.Fprintf(stdout, "validate: pass=%t tables=%d rows=%d issues=%d\n",
			passed, report.Coverage.Tables, report.Coverage.Rows, len(report.Issues))
		if !passed {
			return 1
		}
		return 0
	}
	if !apply {
		report, err := engine.Preview(ctx, contracts.PreviewRequest{Plan: plan})
		if err != nil {
			return fail(stderr, 1)
		}
		fmt.Fprintf(stdout, "preview: dry-run=true rows=%d complete=%t (no row values shown)\n",
			report.RowsRead, report.Coverage.Complete)
		return 0
	}
	report, err := engine.Apply(ctx, contracts.ApplyRequest{
		Plan: plan, ExplicitApply: true, BatchSize: cfg.BatchSize,
	})
	if err != nil {
		fmt.Fprintf(stderr, "apply: complete=false committed_batches=%d outcome_unknown=%t\n",
			len(report.Committed), report.CommitOutcomeUnknown)
		return 1
	}
	fmt.Fprintf(stdout, "apply: complete=%t committed_batches=%d\n",
		report.Complete, len(report.Committed))
	if !report.Complete {
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "MaskRiver — bounded SQLite data masking (M1 in development)")
	fmt.Fprintln(w, "Usage: maskriver <scan|mask|validate|version> [flags]")
	fmt.Fprintln(w, "Two independent pre-existing SQLite files are required; mask defaults to dry run.")
	fmt.Fprintln(w, "Only mask --apply --approved grants write intent. Use authorized synthetic copies.")
}

func usageError(w io.Writer) int {
	fmt.Fprintln(w, "invalid arguments; use maskriver --help")
	return 2
}

func fail(w io.Writer, code int) int {
	fmt.Fprintln(w, "operation refused or incomplete (sanitized; no database values or DSN)")
	return code
}
