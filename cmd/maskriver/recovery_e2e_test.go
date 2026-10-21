package main

// Process-level recovery acceptance for Issue #14.
//
// Every command runs as a real child process through the same main entry point.
// Faults are injected by a test-only hook installed only in the child test
// binary: the production binary has no environment-controlled fault path. The
// child exits at a precise stage, so an interruption point is deterministic
// rather than a random kill.

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/internal/testhooks"
)

const cliFaultEnv = "MASKRIVER_TEST_FAULT"

// installChildFault installs a deterministic process exit in the CLI child.
// It is compiled only into the test binary.
func installChildFault(spec string) {
	stage, value, hasValue := strings.Cut(spec, "=")
	limit := 1
	if hasValue {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			os.Exit(2)
		}
		limit = parsed
	}
	count := 0
	switch stage {
	case "after_commit":
		testhooks.Set(func(current string) error {
			if current == "after_business_commit" {
				count++
				if count >= limit {
					os.Exit(70)
				}
			}
			return nil
		})
	case "before_commit":
		testhooks.Set(func(current string) error {
			if current == "before_commit" {
				count++
				if count >= limit {
					os.Exit(71)
				}
			}
			return nil
		})
	case "after_validate":
		testhooks.Set(func(current string) error {
			if current == "after_validate_before_complete" {
				os.Exit(72)
			}
			return nil
		})
	default:
		os.Exit(2)
	}
}

// runCLIProcessFault runs one CLI invocation with a fault specification.
func runCLIProcessFault(t *testing.T, spec string, args ...string) (int, string) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcessChild$", "--"}, args...)...)
	command.Env = append(os.Environ(), cliChildEnv+"=1", cliFaultEnv+"="+spec)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if err == nil {
		return 0, output.String()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), output.String()
	}
	t.Fatalf("child process %v did not start: %v", args, err)
	return 0, ""
}

// recoveryStatements builds a synthetic table with a fixed number of rows as
// batched INSERT statements.
func recoveryStatements(rows int) []string {
	statements := []string{`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`}
	for start := 1; start <= rows; start += 100 {
		end := start + 99
		if end > rows {
			end = rows
		}
		var values []string
		for id := start; id <= end; id++ {
			values = append(values, fmt.Sprintf("(%d,'recovery%04d@example.invalid')", id, id))
		}
		statements = append(statements, "INSERT INTO people(id,email) VALUES "+strings.Join(values, ","))
	}
	return statements
}

func (f *e2eFixture) addRecoverySecrets(rows int) {
	f.secrets = append(f.secrets,
		"recovery0001@example.invalid",
		fmt.Sprintf("recovery%04d@example.invalid", rows))
}

func (f *e2eFixture) recoveryApproveArgs(scopes ...string) []string {
	return append(f.streamingApproveArgs(scopes...), "--sample-limit", "10000")
}

func (f *e2eFixture) recoveryApplyArgs(budget string, scopes ...string) []string {
	return append(f.streamingApplyArgs(budget, scopes...), "--sample-limit", "10000", "--batch-size", "2")
}

func (f *e2eFixture) recoveryResumeArgs(budget string, scopes ...string) []string {
	return append(f.streamingResumeArgs(budget, scopes...), "--sample-limit", "10000", "--batch-size", "2")
}

func (f *e2eFixture) recoveryValidateArgs(scopes ...string) []string {
	return append(f.validateArgs(scopes...), "--sample-limit", "10000")
}

func (f *e2eFixture) initRecovery(t *testing.T) {
	t.Helper()
	f.initOutbox(t)
	f.run(t, 0, "init-recovery", "--target", f.target)
}

func (f *e2eFixture) streamingResumeArgs(budget string, scopes ...string) []string {
	args := f.streamingApplyArgs(budget, scopes...)
	for i, arg := range args {
		if arg == "--apply" {
			return append(args[:i+1], append([]string{"--resume"}, args[i+1:]...)...)
		}
	}
	return append(args, "--resume")
}

func targetEmails(t *testing.T, path string) []string {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	rows, err := database.Query(`SELECT id, email FROM people ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var id int
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			t.Fatal(err)
		}
		result = append(result, email)
	}
	sort.Strings(result)
	return result
}

func TestE2ERecoveryCLIInterruptionPoints(t *testing.T) {
	requireLinuxOutbox(t)
	for _, committed := range []int{1, 10, 100, 500} {
		committed := committed
		t.Run(fmt.Sprintf("after-%d-batches", committed), func(t *testing.T) {
			rows := committed*2 + 2
			scopes := []string{"people.email"}

			// A: uninterrupted baseline on an identical synthetic dataset.
			baseline := newE2EFixture(t, recoveryStatements(rows)...)
			baseline.addRecoverySecrets(rows)
			baselineBudget := baseline.budgetFile(t)
			baseline.initRecovery(t)
			baseline.run(t, 0, baseline.recoveryApproveArgs(scopes...)...)
			baseline.run(t, 0, baseline.recoveryApplyArgs(baselineBudget, scopes...)...)
			want := targetEmails(t, baseline.target)

			// B: the same dataset, deterministically interrupted after N commits.
			f := newE2EFixture(t, recoveryStatements(rows)...)
			f.addRecoverySecrets(rows)
			budget := f.budgetFile(t)
			f.initRecovery(t)
			f.run(t, 0, f.recoveryApproveArgs(scopes...)...)
			code, output := runCLIProcessFault(t, fmt.Sprintf("after_commit=%d", committed),
				f.recoveryApplyArgs(budget, scopes...)...)
			if code != 70 {
				t.Fatalf("interrupted apply exit=%d, want 70; output=%q", code, output)
			}

			// The read-only status observes the interrupted run and writes nothing.
			beforeStatus := f.targetDigest(t)
			status := f.run(t, 0, "status", "--target", f.target, "--run-id", e2eRunID)
			for _, wantText := range []string{
				"state=running", "resume_eligible=true", "commit_unknown=false",
				"conflict=false",
			} {
				if !strings.Contains(status, wantText) {
					t.Fatalf("status missing %q: %q", wantText, status)
				}
			}
			if f.targetDigest(t) != beforeStatus {
				t.Fatal("status changed the target")
			}

			resumed := f.run(t, 0, f.recoveryResumeArgs(budget, scopes...)...)
			for _, wantText := range []string{"state=complete", "audit=consistent", "resume_eligible=true"} {
				if !strings.Contains(resumed, wantText) {
					t.Fatalf("resume missing %q: %q", wantText, resumed)
				}
			}
			if got := f.run(t, 0, f.recoveryValidateArgs(scopes...)...); !strings.Contains(got, "pass=true") {
				t.Fatalf("strict validate failed: %q", got)
			}
			if got := targetEmails(t, f.target); !equalStrings(want, got) {
				t.Fatalf("resumed target differs from the uninterrupted baseline")
			}

			// A second resume is idempotent and read-only.
			digest := f.targetDigest(t)
			f.run(t, 0, f.recoveryResumeArgs(budget, scopes...)...)
			if f.targetDigest(t) != digest {
				t.Fatal("idempotent resume changed the target")
			}
		})
	}
}

func TestE2ERecoveryCLIValidateInterruptionAndPreCommitCrash(t *testing.T) {
	requireLinuxOutbox(t)
	scopes := []string{"people.email"}

	t.Run("after-validate", func(t *testing.T) {
		f := newE2EFixture(t, recoveryStatements(6)...)
		budget := f.budgetFile(t)
		f.initRecovery(t)
		f.run(t, 0, f.recoveryApproveArgs(scopes...)...)
		code, output := runCLIProcessFault(t, "after_validate", f.recoveryApplyArgs(budget, scopes...)...)
		if code != 72 {
			t.Fatalf("exit=%d want 72; output=%q", code, output)
		}
		status := f.run(t, 0, "status", "--target", f.target, "--run-id", e2eRunID)
		if !strings.Contains(status, "business_complete=true") {
			t.Fatalf("status=%q", status)
		}
		f.run(t, 0, f.recoveryResumeArgs(budget, scopes...)...)
		if got := f.run(t, 0, f.recoveryValidateArgs(scopes...)...); !strings.Contains(got, "pass=true") {
			t.Fatalf("validate=%q", got)
		}
	})

	t.Run("before-commit", func(t *testing.T) {
		f := newE2EFixture(t, recoveryStatements(6)...)
		budget := f.budgetFile(t)
		f.initRecovery(t)
		f.run(t, 0, f.recoveryApproveArgs(scopes...)...)
		code, output := runCLIProcessFault(t, "before_commit=1", f.recoveryApplyArgs(budget, scopes...)...)
		if code != 71 {
			t.Fatalf("exit=%d want 71; output=%q", code, output)
		}
		f.run(t, 0, f.recoveryResumeArgs(budget, scopes...)...)
		if got := f.run(t, 0, f.recoveryValidateArgs(scopes...)...); !strings.Contains(got, "pass=true") {
			t.Fatalf("validate=%q", got)
		}
	})
}

func TestE2ERecoveryCLIRefusals(t *testing.T) {
	requireLinuxOutbox(t)
	scopes := []string{"people.email"}
	f := newE2EFixture(t, recoveryStatements(6)...)

	// Status on a database without the recovery schema is unsupported.
	f.initOutbox(t)
	if code, _ := runCLIProcess(t, "status", "--target", f.target, "--run-id", e2eRunID); code != 6 {
		t.Fatalf("status without recovery schema exit=%d, want 6", code)
	}
	f.initRecovery(t)
	f.run(t, 0, f.recoveryApproveArgs(scopes...)...)
	budget := f.budgetFile(t)

	// Create a genuine interrupted run.
	code, output := runCLIProcessFault(t, "after_commit=1", f.recoveryApplyArgs(budget, scopes...)...)
	if code != 70 {
		t.Fatalf("interrupted apply exit=%d; output=%q", code, output)
	}
	before := f.targetDigest(t)

	base := append([]string{"mask"}, f.planArgs(scopes...)...)
	base = append(base, "--key-file", f.keyFile)
	base = append(base, f.review.applyFlags()...)
	base = append(base, "--approved", "--budget-file", budget, "--receipt-page-size", "8",
		"--sample-limit", "10000", "--batch-size", "2")

	// A resume must be explicit: --streaming, --apply, --run-id.
	if code, _ := runCLIProcess(t, append(base, "--resume", "--run-id", e2eRunID)...); code != 2 {
		t.Fatalf("resume without --streaming exit=%d, want 2", code)
	}
	if code, _ := runCLIProcess(t, append(base, "--apply", "--streaming", "--resume")...); code != 2 {
		t.Fatalf("resume without --run-id exit=%d, want 2", code)
	}
	withoutApply := append([]string{}, base...)
	withoutResume := append(withoutApply, "--streaming", "--resume", "--run-id", e2eRunID)
	if code, _ := runCLIProcess(t, withoutResume...); code != 2 {
		t.Fatalf("resume without --apply exit=%d, want 2", code)
	}
	if code, _ := runCLIProcess(t, append(base, "--apply", "--streaming", "--resume", "--run-id", "not-a-run-id")...); code != 2 {
		t.Fatalf("resume with malformed run id exit=%d, want 2", code)
	}
	// A missing run is invalid, never a new run.
	other := "ffffffffffffffffffffffffffffffff"
	if code, _ := runCLIProcess(t, append(base, "--apply", "--streaming", "--resume", "--run-id", other)...); code != 7 {
		t.Fatalf("resume of a missing run exit=%d, want 7", code)
	}
	if f.targetDigest(t) != before {
		t.Fatal("refused resumes changed the target")
	}

	// Revoked approval must refuse the resume before any write.
	revoke := append([]string{"revoke"}, f.planArgs(scopes...)...)
	revoke = append(revoke, f.review.reviewFlags()...)
	revoke = append(revoke, "--streaming")
	f.run(t, 0, revoke...)
	if code, _ := runCLIProcess(t, append(base, "--apply", "--streaming", "--resume", "--run-id", e2eRunID)...); code != 8 {
		t.Fatalf("revoked resume exit=%d, want 8", code)
	}
	if f.targetDigest(t) != before {
		t.Fatal("revoked resume changed the target")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
