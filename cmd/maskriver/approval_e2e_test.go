package main

// End-to-end tests for work package C2: a plan is issued in one process, loaded
// by a NEW process, and only then can Apply write. Every database here is
// synthetic SQLite under t.TempDir(); the review commands really run as child
// processes through the same main entry point, so exit codes, sanitized output
// and file effects are observed from outside the process.

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const cliChildEnv = "MASKRIVER_CLI_CHILD"

// TestCLIProcessChild is the re-exec entry point for the child-process tests. It
// runs only when the parent set cliChildEnv, and it invokes exactly the same run
// function main uses.
func TestCLIProcessChild(t *testing.T) {
	if os.Getenv(cliChildEnv) != "1" {
		t.Skip("helper process only")
	}
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(2)
	}
	if spec := os.Getenv(cliFaultEnv); spec != "" {
		installChildFault(spec)
	}
	os.Exit(run(os.Args[separator+1:], os.Stdout, os.Stderr))
}

// runCLIProcess runs one CLI invocation in a new process and returns its exit
// code and combined output.
func runCLIProcess(t *testing.T, args ...string) (int, string) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProcessChild$", "--"}, args...)...)
	command.Env = append(os.Environ(), cliChildEnv+"=1")
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

// e2eFixture is one synthetic source/target pair, the masking key file, and the
// reviewer provisioning. It also carries the secrets every CLI path must keep
// out of its output.
type e2eFixture struct {
	dir     string
	source  string
	target  string
	keyFile string
	review  reviewProvisioning
	secrets []string
}

var e2eOneTable = []string{
	`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
	`INSERT INTO people VALUES (1, 'e2e-alpha@example.invalid'), (2, 'e2e-beta@example.invalid'), (3, 'e2e-gamma@example.invalid')`,
}

var e2eTwoTables = []string{
	`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
	`INSERT INTO people VALUES (1, 'e2e-alpha@example.invalid'), (2, 'e2e-beta@example.invalid')`,
	`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
	`INSERT INTO contacts VALUES (1, 'e2e-delta@example.invalid'), (2, 'e2e-epsilon@example.invalid')`,
}

// e2eSyntheticValues are the fixture cell values that must never appear in CLI
// output.
var e2eSyntheticValues = []string{
	"e2e-alpha@example.invalid", "e2e-beta@example.invalid",
	"e2e-gamma@example.invalid", "e2e-delta@example.invalid", "e2e-epsilon@example.invalid",
}

func newE2EFixture(t *testing.T, statements ...string) *e2eFixture {
	t.Helper()
	dir := t.TempDir()
	f := &e2eFixture{
		dir:     dir,
		source:  filepath.Join(dir, "source.db"),
		target:  filepath.Join(dir, "target.db"),
		keyFile: filepath.Join(dir, "masking.key"),
	}
	cliExec(t, f.source, statements...)
	raw, err := os.ReadFile(f.source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.target, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.keyFile, []byte(strings.Repeat("K", 40)), 0o600); err != nil {
		t.Fatal(err)
	}
	f.review = newReviewProvisioning(t)
	f.secrets = []string{
		f.source, f.target, f.keyFile,
		f.review.historyDir, f.review.trustFile, f.review.reviewerKeyFile,
		strings.Repeat("K", 40),
		string(f.review.private), hex.EncodeToString(f.review.private),
		hex.EncodeToString(f.review.private.Seed()),
	}
	f.secrets = append(f.secrets, e2eSyntheticValues...)
	return f
}

// planArgs are the flags every command needs to rebuild exactly the same plan.
func (f *e2eFixture) planArgs(scopes ...string) []string {
	args := []string{"--source", f.source, "--target", f.target,
		"--strategy", "fake_email", "--key-id", "synthetic-key", "--min-samples", "1"}
	for _, scope := range scopes {
		args = append(args, "--scope", scope)
	}
	return args
}

func (f *e2eFixture) previewArgs(scopes ...string) []string {
	args := append([]string{"mask"}, f.planArgs(scopes...)...)
	return append(args, "--key-file", f.keyFile)
}

func (f *e2eFixture) validateArgs(scopes ...string) []string {
	args := append([]string{"validate"}, f.planArgs(scopes...)...)
	return append(args, "--key-file", f.keyFile, "--strict")
}

func (f *e2eFixture) approveArgs(scopes ...string) []string {
	args := append([]string{"approve"}, f.planArgs(scopes...)...)
	args = append(args, f.review.reviewFlags()...)
	return append(args, "--reviewer-id", "reviewer@synthetic.example")
}

func (f *e2eFixture) revokeArgs(scopes ...string) []string {
	args := append([]string{"revoke"}, f.planArgs(scopes...)...)
	return append(args, f.review.reviewFlags()...)
}

// applyArgs is the authorized write path: masking key, read-only store, the
// deployment trust file, explicit --approved and explicit --apply. It carries no
// reviewer private key.
func (f *e2eFixture) applyArgs(scopes ...string) []string {
	args := append([]string{"mask"}, f.planArgs(scopes...)...)
	args = append(args, "--key-file", f.keyFile)
	args = append(args, f.review.applyFlags()...)
	return append(args, "--approved", "--apply")
}

// run executes one CLI child process, requires the expected exit code and
// refuses any output that names a fixture value, path or key.
func (f *e2eFixture) run(t *testing.T, want int, args ...string) string {
	t.Helper()
	code, output := runCLIProcess(t, args...)
	if code != want {
		t.Fatalf("%s exit=%d want=%d; output=%q", args[0], code, want, output)
	}
	for _, secret := range f.secrets {
		if secret != "" && strings.Contains(output, secret) {
			t.Fatalf("%s leaked %q: %q", args[0], secret, output)
		}
	}
	return output
}

// targetDigest is the byte identity of the write destination.
func (f *e2eFixture) targetDigest(t *testing.T) [32]byte {
	t.Helper()
	raw, err := os.ReadFile(f.target)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(raw)
}

// TestE2EPlanIssueNewProcessApply is PART 3 item 1: a plan is previewed, issued
// in an independent process, then a NEW process loads that decision for preview,
// another new process applies, and a final new process validates strictly.
func TestE2EPlanIssueNewProcessApply(t *testing.T) {
	f := newE2EFixture(t, e2eOneTable...)
	scopes := []string{"people.email"}

	// Plan and preflight before any decision exists: this process only reads.
	if got := f.run(t, 0, f.previewArgs(scopes...)...); !strings.Contains(got, "preview: dry-run=true") {
		t.Fatalf("pre-approval preview output=%q", got)
	}
	// Independent issuance.
	if got := f.run(t, 0, f.approveArgs(scopes...)...); !strings.Contains(got, "approve: stored=true") {
		t.Fatalf("approve output=%q", got)
	}
	// A new process loads the approval for preview and must not write.
	before := f.targetDigest(t)
	if got := f.run(t, 0, f.previewArgs(scopes...)...); !strings.Contains(got, "preview: dry-run=true") {
		t.Fatalf("post-approval preview output=%q", got)
	}
	if f.targetDigest(t) != before {
		t.Fatal("preview after approval changed the target")
	}
	// A new process loads the approval and writes.
	applied := f.run(t, 0, f.applyArgs(scopes...)...)
	if !strings.Contains(applied, "apply: complete=true committed_batches=1 committed_rows=3") {
		t.Fatalf("apply summary=%q", applied)
	}
	// A new process validates the durable result.
	if got := f.run(t, 0, f.validateArgs(scopes...)...); !strings.Contains(got, "pass=true") {
		t.Fatalf("strict validation output=%q", got)
	}
	if f.targetDigest(t) == before {
		t.Fatal("apply did not change the target")
	}
	var rows int
	database, err := sql.Open("sqlite", "file:"+f.target+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if err := database.QueryRow(`SELECT COUNT(DISTINCT email) FROM people`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("masked column lost distinct values: %d", rows)
	}
}

// TestE2ETwoTableDecisionCoversWholePlan is PART 3 item 2.
func TestE2ETwoTableDecisionCoversWholePlan(t *testing.T) {
	f := newE2EFixture(t, e2eTwoTables...)
	scopes := []string{"people.email", "contacts.email"}
	f.run(t, 0, f.approveArgs(scopes...)...)
	applied := f.run(t, 0, f.applyArgs(scopes...)...)
	for _, want := range []string{
		"apply: complete=true",
		"apply: table 1 batches=1 rows=2",
		"apply: table 2 batches=1 rows=2",
	} {
		if !strings.Contains(applied, want) {
			t.Fatalf("apply summary missing %q: %q", want, applied)
		}
	}
	if got := f.run(t, 0, f.validateArgs(scopes...)...); !strings.Contains(got, "pass=true") {
		t.Fatalf("strict validation output=%q", got)
	}
}

// TestE2EPartialDecisionDoesNotAuthorizeWholePlan is PART 3 item 3: a decision
// issued for one table does not authorize a two-table plan.
func TestE2EPartialDecisionDoesNotAuthorizeWholePlan(t *testing.T) {
	f := newE2EFixture(t, e2eTwoTables...)
	f.run(t, 0, f.approveArgs("people.email")...)
	before := f.targetDigest(t)
	f.run(t, 1, f.applyArgs("people.email", "contacts.email")...)
	if f.targetDigest(t) != before {
		t.Fatal("partial authorization wrote to the target")
	}
}

// TestE2EDecisionBoundFieldChangesRefused is PART 3 item 4.
func TestE2EDecisionBoundFieldChangesRefused(t *testing.T) {
	t.Run("reordered scopes", func(t *testing.T) {
		f := newE2EFixture(t, e2eTwoTables...)
		f.run(t, 0, f.approveArgs("people.email", "contacts.email")...)
		before := f.targetDigest(t)
		f.run(t, 1, f.applyArgs("contacts.email", "people.email")...)
		if f.targetDigest(t) != before {
			t.Fatal("reordered scope plan was written")
		}
	})
	t.Run("changed strategy", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		f.run(t, 0, f.approveArgs("people.email")...)
		args := f.applyArgs("people.email")
		for index := range args {
			if args[index] == "fake_email" {
				args[index] = "redact"
			}
		}
		before := f.targetDigest(t)
		f.run(t, 1, args...)
		if f.targetDigest(t) != before {
			t.Fatal("changed-strategy plan was written")
		}
	})
	t.Run("changed schema digest", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		f.run(t, 0, f.approveArgs("people.email")...)
		// The same DDL lands on both snapshots, so only the schema (and, for the
		// source, the file digest) moved: the decision no longer binds this plan.
		for _, path := range []string{f.source, f.target} {
			cliExec(t, path, `ALTER TABLE people ADD COLUMN note TEXT NULL`)
		}
		before := f.targetDigest(t)
		f.run(t, 1, f.applyArgs("people.email")...)
		if f.targetDigest(t) != before {
			t.Fatal("changed-schema plan was written")
		}
	})
	t.Run("changed source snapshot", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		f.run(t, 0, f.approveArgs("people.email")...)
		for _, path := range []string{f.source, f.target} {
			cliExec(t, path, `INSERT INTO people VALUES (4, 'e2e-zeta@example.invalid')`)
		}
		before := f.targetDigest(t)
		f.run(t, 1, f.applyArgs("people.email")...)
		if f.targetDigest(t) != before {
			t.Fatal("changed-source plan was written")
		}
	})
	t.Run("changed target binding", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		f.run(t, 0, f.approveArgs("people.email")...)
		other := filepath.Join(f.dir, "other-target.db")
		raw, err := os.ReadFile(f.target)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(other, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		args := f.applyArgs("people.email")
		for index := range args {
			if args[index] == f.target {
				args[index] = other
			}
		}
		before := sha256.Sum256(raw)
		f.run(t, 1, args...)
		after, err := os.ReadFile(other)
		if err != nil {
			t.Fatal(err)
		}
		if sha256.Sum256(after) != before {
			t.Fatal("changed-target plan was written")
		}
	})
}

// TestE2EInvalidDecisionsRefused is PART 3 item 5 for every decision failure the
// CLI can produce. A future-issued decision has no CLI flag by design, so it is
// covered by the history regression test and the runner integration test.
func TestE2EInvalidDecisionsRefused(t *testing.T) {
	scopes := []string{"people.email"}
	t.Run("missing record", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		before := f.targetDigest(t)
		f.run(t, 1, f.applyArgs(scopes...)...)
		if f.targetDigest(t) != before {
			t.Fatal("apply without a decision wrote to the target")
		}
		if _, err := os.Stat(f.review.historyDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("a refused apply created a history store")
		}
	})
	t.Run("tampered record", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		f.run(t, 0, f.approveArgs(scopes...)...)
		entries, err := os.ReadDir(f.review.historyDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".decision") {
			t.Fatalf("unexpected decision store contents")
		}
		path := filepath.Join(f.review.historyDir, entries[0].Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw[len(raw)/2] ^= 0x01
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		before := f.targetDigest(t)
		f.run(t, 1, f.applyArgs(scopes...)...)
		if f.targetDigest(t) != before {
			t.Fatal("tampered decision authorized a write")
		}
	})
	t.Run("forged signature", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		attacker := newReviewProvisioningWithSeed(t, 0x5e)
		// The attacker signs a decision for exactly this plan with a key the
		// deployment trust file does not name.
		args := append([]string{"approve"}, f.planArgs(scopes...)...)
		args = append(args, attacker.reviewFlags()...)
		args = append(args, "--reviewer-id", "attacker@synthetic.example")
		f.run(t, 0, args...)

		applyArgs := append([]string{"mask"}, f.planArgs(scopes...)...)
		applyArgs = append(applyArgs, "--key-file", f.keyFile,
			"--history-dir", attacker.historyDir, "--trust-file", f.review.trustFile,
			"--approved", "--apply")
		before := f.targetDigest(t)
		f.run(t, 1, applyArgs...)
		if f.targetDigest(t) != before {
			t.Fatal("forged decision authorized a write")
		}
	})
	t.Run("expired record", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		args := append(f.approveArgs(scopes...), "--valid-for", "60ms")
		f.run(t, 0, args...)
		time.Sleep(200 * time.Millisecond)
		before := f.targetDigest(t)
		f.run(t, 1, f.applyArgs(scopes...)...)
		if f.targetDigest(t) != before {
			t.Fatal("expired decision authorized a write")
		}
	})
	t.Run("revoked record", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		f.run(t, 0, f.approveArgs(scopes...)...)
		if got := f.run(t, 0, f.revokeArgs(scopes...)...); !strings.Contains(got, "revoke: stored=true") {
			t.Fatalf("revoke output=%q", got)
		}
		before := f.targetDigest(t)
		f.run(t, 1, f.applyArgs(scopes...)...)
		if f.targetDigest(t) != before {
			t.Fatal("revoked decision authorized a write")
		}
	})
	t.Run("wrong private key", func(t *testing.T) {
		f := newE2EFixture(t, e2eOneTable...)
		attacker := newReviewProvisioningWithSeed(t, 0x5e)
		args := append([]string{"approve"}, f.planArgs(scopes...)...)
		args = append(args, "--history-dir", f.review.historyDir, "--trust-file", f.review.trustFile,
			"--reviewer-key-id", "reviewer-1", "--reviewer-key-file", attacker.reviewerKeyFile,
			"--reviewer-id", "reviewer@synthetic.example")
		// The key id is trusted but the private key is a different key: the store
		// refuses to sign a decision no verifier could accept.
		f.run(t, 2, args...)
		if _, err := os.Stat(f.review.historyDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("mismatched reviewer key still created a history store")
		}
	})
}

// TestE2ERefusedApplyHasNoWriteSideEffects is PART 3 item 6 at the CLI level:
// with no valid decision the target is byte-identical and no store appears. The
// Runner-level counterpart asserts the Writer counters.
func TestE2ERefusedApplyHasNoWriteSideEffects(t *testing.T) {
	f := newE2EFixture(t, e2eOneTable...)
	scopes := []string{"people.email"}
	before := f.targetDigest(t)
	treeBefore := e2eTreeDigest(t, f.dir)
	f.run(t, 1, f.applyArgs(scopes...)...)
	if f.targetDigest(t) != before {
		t.Fatal("refused apply changed the target bytes")
	}
	if treeAfter := e2eTreeDigest(t, f.dir); treeAfter != treeBefore {
		t.Fatalf("refused apply changed the fixture tree: %s -> %s", treeBefore, treeAfter)
	}
}

// TestE2EReadOnlyCommandsDoNotTouchHistory is PART 3 item 7.
func TestE2EReadOnlyCommandsDoNotTouchHistory(t *testing.T) {
	f := newE2EFixture(t, e2eOneTable...)
	scopes := []string{"people.email"}
	treeBefore := e2eTreeDigest(t, f.dir)

	f.run(t, 0, "scan", "--source", f.source, "--target", f.target, "--scope", "people.email", "--min-samples", "1")
	// Preview gets the apply-only store flags and must still ignore them.
	previewArgs := append(f.previewArgs(scopes...), f.review.applyFlags()...)
	if got := f.run(t, 0, previewArgs...); !strings.Contains(got, "preview: dry-run=true") {
		t.Fatalf("preview output=%q", got)
	}
	// Validation before masking fails, but it must fail without touching history.
	f.run(t, 1, f.validateArgs(scopes...)...)

	if _, err := os.Stat(f.review.historyDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only commands created the history store")
	}
	if treeAfter := e2eTreeDigest(t, f.dir); treeAfter != treeBefore {
		t.Fatalf("read-only commands changed the fixture tree: %s -> %s", treeBefore, treeAfter)
	}
}

// TestE2EApprovalDoesNotBypassExistingGates is PART 3 item 8: a valid decision
// changes nothing about the scan, key, constraint or journal gates.
func TestE2EApprovalDoesNotBypassExistingGates(t *testing.T) {
	cases := []struct {
		name        string
		statements  []string
		scopes      []string
		wantApprove int
		walSource   bool
	}{
		{
			name: "UNKNOWN scan",
			statements: []string{
				`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
				`INSERT INTO people VALUES (1, 'alpha'), (2, 'beta')`,
			},
			scopes: []string{"people.email"}, wantApprove: 1,
		},
		{
			name: "missing primary key",
			statements: []string{
				`CREATE TABLE people (email TEXT NOT NULL)`,
				`INSERT INTO people VALUES ('e2e-alpha@example.invalid'), ('e2e-beta@example.invalid')`,
			},
			scopes: []string{"people.email"}, wantApprove: 1,
		},
		{
			name: "foreign key column",
			statements: []string{
				`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE)`,
				`INSERT INTO people VALUES (1, 'e2e-alpha@example.invalid'), (2, 'e2e-beta@example.invalid')`,
				`CREATE TABLE orders (id INTEGER PRIMARY KEY, person_email TEXT NOT NULL REFERENCES people(email))`,
				`INSERT INTO orders VALUES (1, 'e2e-alpha@example.invalid'), (2, 'e2e-beta@example.invalid')`,
			},
			scopes: []string{"orders.person_email"}, wantApprove: 0,
		},
		{
			name: "UNIQUE constraint",
			statements: []string{
				`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE)`,
				`INSERT INTO people VALUES (1, 'e2e-alpha@example.invalid'), (2, 'e2e-beta@example.invalid')`,
			},
			scopes: []string{"people.email"}, wantApprove: 0,
		},
		{
			name: "CHECK constraint",
			statements: []string{
				`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL CHECK (length(email) > 4))`,
				`INSERT INTO people VALUES (1, 'e2e-alpha@example.invalid'), (2, 'e2e-beta@example.invalid')`,
			},
			scopes: []string{"people.email"}, wantApprove: 0,
		},
		{
			name: "trigger",
			statements: []string{
				`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
				`INSERT INTO people VALUES (1, 'e2e-alpha@example.invalid'), (2, 'e2e-beta@example.invalid')`,
				`CREATE TRIGGER people_touch AFTER UPDATE ON people BEGIN SELECT 1; END`,
			},
			scopes: []string{"people.email"}, wantApprove: 0,
		},
		{
			name: "WAL source",
			statements: []string{
				`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
				`INSERT INTO people VALUES (1, 'e2e-alpha@example.invalid'), (2, 'e2e-beta@example.invalid')`,
			},
			scopes: []string{"people.email"}, wantApprove: 1, walSource: true,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newE2EFixture(t, test.statements...)
			if test.walSource {
				var mode string
				database, err := sql.Open("sqlite", "file:"+f.source+"?mode=rw")
				if err != nil {
					t.Fatal(err)
				}
				if err := database.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil {
					_ = database.Close()
					t.Fatal(err)
				}
				if err := database.Close(); err != nil {
					t.Fatal(err)
				}
				if !strings.EqualFold(mode, "wal") {
					t.Fatalf("fixture journal mode = %q, want wal", mode)
				}
			}
			f.run(t, test.wantApprove, f.approveArgs(test.scopes...)...)
			before, err := os.ReadFile(f.target)
			if err != nil {
				t.Fatal(err)
			}
			f.run(t, 1, f.applyArgs(test.scopes...)...)
			after, err := os.ReadFile(f.target)
			if err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatal("a refused gate changed the target")
			}
		})
	}
}

// e2eTreeDigest hashes every file under a directory so a read-only flow can be
// shown to leave no file behind.
func e2eTreeDigest(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		entries = append(entries, path+"|"+info.Mode().String()+"|"+strconv.FormatInt(info.Size(), 10))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

// TestApplyPathTakesNoReviewerPrivateKey pins the trust boundary: the apply
// invocation accepts a deployment trust file, and a reviewer private key flag is
// not part of its surface at all.
func TestApplyPathTakesNoReviewerPrivateKey(t *testing.T) {
	f := newE2EFixture(t, e2eOneTable...)
	args := append(f.applyArgs("people.email"), "--reviewer-key-file", f.review.reviewerKeyFile)
	f.run(t, 2, args...)
}
