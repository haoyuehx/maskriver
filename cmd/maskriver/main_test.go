package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haoyuehx/maskriver/internal/testenv"
)

func TestCLIArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		text string
	}{
		{"help", nil, 0, "SQLite"},
		{"explicit help", []string{"--help"}, 0, "Usage:"},
		{"version", []string{"--version"}, 0, version},
		{"scan missing paths", []string{"scan"}, 2, "operation refused"},
		{"mask missing paths", []string{"mask"}, 2, "operation refused"},
		{"apply missing paths", []string{"mask", "--apply"}, 2, "operation refused"},
		{"validate missing paths", []string{"validate"}, 2, "operation refused"},
		{"command help", []string{"mask", "--help"}, 0, "Usage:"},
		{"unknown", []string{"unknown"}, 2, "invalid arguments"},
		{"unknown flag", []string{"mask", "--unknown"}, 2, "invalid arguments"},
		{"positional rejected", []string{"mask", "unexpected"}, 2, "invalid arguments"},
		{"scan apply rejected", []string{"scan", "--apply"}, 2, "invalid arguments"},
		{"scan scope without paths", []string{"scan", "--scope", "people.email"}, 2, "operation refused"},
		{"scope without separator", []string{"scan", "--scope", "people"}, 2, "invalid arguments"},
		{"scope empty column", []string{"mask", "--scope", "people."}, 2, "invalid arguments"},
		{"scope empty table", []string{"validate", "--scope", ".email"}, 2, "invalid arguments"},
		{"scope extra separator", []string{"mask", "--scope", "a.b.c"}, 2, "invalid arguments"},
		{"extra version args", []string{"version", "extra"}, 2, "invalid arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run(tt.args, &out, &errOut); got != tt.code {
				t.Fatalf("exit=%d, want %d", got, tt.code)
			}
			if !strings.Contains(out.String()+errOut.String(), tt.text) {
				t.Fatalf("missing %q in safe output", tt.text)
			}
		})
	}
}

func TestCLIFlagParseErrorsAreSanitized(t *testing.T) {
	const marker = "SYNTHETIC-DSN-DO-NOT-ECHO"
	tests := []struct {
		name string
		args []string
	}{
		{"workers integer", []string{"mask", "--workers", marker}},
		{"min samples integer", []string{"scan", "--source", marker, "--min-samples", marker}},
		{"sample limit integer", []string{"scan", "--sample-limit=" + marker}},
		{"batch size integer", []string{"mask", "--target", marker, "--batch-size=" + marker}},
		{"min ratio float", []string{"scan", "--min-ratio", marker}},
		{"approved bool", []string{"validate", "--key-file", marker, "--approved=" + marker}},
		{"apply bool", []string{"mask", "--source=" + marker, "--apply=" + marker}},
		{"strict bool", []string{"validate", "--strict=" + marker}},
		{"scope format", []string{"scan", "--scope", marker}},
		{"unknown flag name", []string{"mask", "--" + marker}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(tt.args, &out, &errOut); code != 2 {
				t.Fatalf("exit=%d, want 2", code)
			}
			for streamName, stream := range map[string]string{"stdout": out.String(), "stderr": errOut.String()} {
				if strings.Contains(stream, marker) {
					t.Fatalf("synthetic marker leaked to %s: %q", streamName, stream)
				}
			}
			if !strings.Contains(errOut.String(), "invalid arguments") {
				t.Fatalf("missing sanitized parse error: %q", errOut.String())
			}
			// The upstream flag package wording also embeds user-supplied values.
			for _, leaked := range []string{"invalid value", "parse error", "flag provided but not defined"} {
				if strings.Contains(errOut.String(), leaked) {
					t.Fatalf("raw flag parser detail %q reached stderr: %q", leaked, errOut.String())
				}
			}
			if out.Len() != 0 {
				t.Fatalf("parse failure wrote to stdout: %q", out.String())
			}
		})
	}
}

func TestCLIHelpPrintsCompleteStaticFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"mask long help", []string{"mask", "--help"}, []string{"Usage: maskriver mask [flags]", "-source", "-target", "-scope", "-workers", "-batch-size", "-strategy", "-key-file", "-approved", "-apply"}},
		{"mask short help", []string{"mask", "-h"}, []string{"Usage: maskriver mask [flags]", "-source", "-target", "-scope", "-workers", "-batch-size", "-key-file", "-approved", "-apply"}},
		{"validate help", []string{"validate", "--help"}, []string{"Usage: maskriver validate [flags]", "-source", "-target", "-scope", "-strict"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(tt.args, &out, &errOut); code != 0 {
				t.Fatalf("exit=%d, want 0", code)
			}
			text := out.String() + errOut.String()
			for _, item := range tt.want {
				if !strings.Contains(text, item) {
					t.Errorf("help missing %q", item)
				}
			}
			if out.Len() != 0 {
				t.Errorf("help wrote to stdout: %q", out.String())
			}
			// Help must be emitted once: the parse-time banner is discarded.
			if n := strings.Count(text, tt.want[0]); n != 1 {
				t.Errorf("usage banner printed %d times, want 1: %q", n, text)
			}
		})
	}
}

func TestCLIRealSQLiteWorkflow(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	target := filepath.Join(dir, "target.db")
	keyFile := filepath.Join(dir, "key.bin")
	seed, err := sql.Open("sqlite", "file:"+source+"?mode=rwc&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if err = testenv.Seed(context.Background(), seed, "sqlite"); err != nil {
		t.Fatal("seed fixture failed", err)
	}
	if err = seed.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, before, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("K", 40)), 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--source", source, "--target", target,
		"--table", "people", "--column", "email"}
	runCmd := func(code int, command string, extras ...string) string {
		t.Helper()
		args := append([]string{command}, base...)
		args = append(args, extras...)
		var out, errOut bytes.Buffer
		if got := run(args, &out, &errOut); got != code {
			t.Fatalf("%s exit=%d wanted=%d; sanitized output=%s", command, got, code, errOut.String())
		}
		all := out.String() + errOut.String()
		if strings.Contains(all, "fixture-01@example.invalid") ||
			strings.Contains(all, strings.Repeat("K", 40)) ||
			strings.Contains(all, source) || strings.Contains(all, target) {
			t.Fatal("CLI leaked secret or path")
		}
		return all
	}
	scan := runCmd(0, "scan")
	if !strings.Contains(scan, "complete=true") {
		t.Fatal("scan did not recognize synthetic email samples")
	}
	common := []string{"--strategy", "fake_email", "--key-id", "synthetic-key", "--key-file", keyFile}
	runCmd(2, "mask", append(append([]string{}, common...), "--apply")...)
	beforeTarget, _ := os.ReadFile(target)
	if got := runCmd(0, "mask", common...); !strings.Contains(got, "dry-run=true") {
		t.Fatal("preview was not read-only")
	}
	afterPreview, _ := os.ReadFile(target)
	if sha256.Sum256(beforeTarget) != sha256.Sum256(afterPreview) {
		t.Fatal("dry run changed target file")
	}
	if got := runCmd(0, "mask", append(append([]string{}, common...), "--apply", "--approved")...); !strings.Contains(got, "complete=true") {
		t.Fatal("authorized apply incomplete")
	}
	afterSource, _ := os.ReadFile(source)
	if sha256.Sum256(before) != sha256.Sum256(afterSource) {
		t.Fatal("source snapshot changed after apply")
	}
	result := runCmd(0, "validate", append(append([]string{}, common...), "--approved", "--strict")...)
	if !strings.Contains(result, "pass=true") {
		t.Fatal("strict post-write validation failed")
	}
	query, err := sql.Open("sqlite", "file:"+target+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()
	var distinct, total int
	if err := query.QueryRow("SELECT COUNT(DISTINCT email), COUNT(*) FROM people").Scan(&distinct, &total); err != nil {
		t.Fatal(err)
	}
	if distinct != 25 || total != 25 {
		t.Fatal("unique email constraint or row count was not preserved")
	}
	if err := query.Close(); err != nil {
		t.Fatal(err)
	}
	serialMasked, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, before, 0600); err != nil {
		t.Fatal(err)
	}
	parallel := append(append([]string{}, common...), "--workers", "4")
	runCmd(0, "mask", parallel...)
	runCmd(0, "mask", append(append([]string{}, parallel...), "--apply", "--approved")...)
	runCmd(0, "validate", append(append([]string{}, parallel...), "--approved", "--strict")...)
	parallelMasked, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(serialMasked) != sha256.Sum256(parallelMasked) {
		t.Fatal("serial and parallel mask runs differ on durable SQLite results")
	}
	// Re-running Apply would try to mask already-modified data; must refuse.
	runCmd(1, "mask", append(append([]string{}, common...), "--apply", "--approved")...)
	// Same inode is never a source + target pair.
	args := []string{"scan", "--source", source, "--target", source,
		"--table", "people", "--column", "email"}
	if got := run(args, &bytes.Buffer{}, &bytes.Buffer{}); got != 2 {
		t.Fatal("same source and target must be rejected")
	}
}

// scopePair writes identical synthetic source/target copies holding two tables
// with one sensitive column each, so a repeated -scope can be exercised end to
// end without depending on the single-table fixture seeder.
func scopePair(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	for _, path := range []string{source, target} {
		database, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range []string{
			`CREATE TABLE people (id INTEGER PRIMARY KEY, email TEXT NULL)`,
			`INSERT INTO people VALUES (1, 'alpha@example.invalid'), (2, 'beta@example.invalid')`,
			`CREATE TABLE contacts (id INTEGER PRIMARY KEY, email TEXT NULL)`,
			`INSERT INTO contacts VALUES (1, 'gamma@example.invalid'), (2, 'delta@example.invalid')`,
		} {
			if _, err = database.Exec(statement); err != nil {
				_ = database.Close()
				t.Fatalf("fixture statement failed: %v", err)
			}
		}
		if err = database.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return source, target
}

// TestCLIRepeatedScopeFlag covers the multi-scope declaration form: repeated
// -scope values scan every declared scope, while mask and validate stay
// fail-closed with ErrUnsupported until multi-scope execution exists.
func TestCLIRepeatedScopeFlag(t *testing.T) {
	source, target := scopePair(t)
	keyFile := filepath.Join(filepath.Dir(source), "key.bin")
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("K", 40)), 0600); err != nil {
		t.Fatal(err)
	}
	sanitized := func(code int, args ...string) string {
		t.Helper()
		var out, errOut bytes.Buffer
		if got := run(args, &out, &errOut); got != code {
			t.Fatalf("%v exit=%d wanted=%d; sanitized output=%s", args, got, code, errOut.String())
		}
		all := out.String() + errOut.String()
		for _, secret := range []string{"alpha@example.invalid", source, target} {
			if strings.Contains(all, secret) {
				t.Fatalf("CLI leaked %q: %q", secret, all)
			}
		}
		return all
	}
	if got := sanitized(0, "scan", "--source", source, "--target", target, "--scope", "people.email", "--min-samples", "1"); !strings.Contains(got, "complete=true columns=1") {
		t.Fatalf("single scope scan output=%q", got)
	}
	if got := sanitized(0, "scan", "--source", source, "--target", target,
		"--scope", "people.email", "--scope", "contacts.email", "--min-samples", "1"); !strings.Contains(got, "complete=true columns=2") {
		t.Fatalf("repeated scope scan output=%q", got)
	}
	common := []string{"--strategy", "fake_email", "--key-id", "synthetic-key", "--key-file", keyFile}
	// Multi-scope Preview is supported since work package B1, so it must really
	// run and report both tables instead of being refused. --min-samples 1 keeps
	// detection deterministic so the assertion cannot pass by accident through
	// an UNKNOWN verdict.
	previewArgs := append([]string{"mask", "--source", source, "--target", target}, common...)
	previewArgs = append(previewArgs, "--scope", "people.email", "--scope", "contacts.email", "--min-samples", "1")
	if got := sanitized(0, previewArgs...); !strings.Contains(got, "preview: dry-run=true") {
		t.Fatalf("multi-scope preview output=%q", got)
	}
	// Apply is still fail-closed for multi-scope until work package B2.
	applyArgs := append([]string{}, previewArgs...)
	applyArgs = append(applyArgs, "--approved", "--apply")
	if got := sanitized(1, applyArgs...); !strings.Contains(got, "operation refused") && !strings.Contains(got, "apply: complete=false") {
		t.Fatalf("multi-scope apply output=%q", got)
	}
	validateArgs := append([]string{"validate", "--source", source, "--target", target}, common...)
	validateArgs = append(validateArgs, "--scope", "people.email", "--scope", "contacts.email", "--min-samples", "1")
	sanitized(1, validateArgs...)
	sanitized(2, "scan", "--source", source, "--target", target, "--scope", "people.email", "--table", "people")
	sanitized(2, "scan", "--source", source, "--target", target, "--scope", "people.email", "--column", "email")
	sanitized(2, "scan", "--source", source, "--target", target, "--scope", "people.email", "--scope", "people.email")
}
