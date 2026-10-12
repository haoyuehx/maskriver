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
		{"unknown flag", []string{"mask", "--unknown"}, 2, "flag provided but not defined"},
		{"positional rejected", []string{"mask", "unexpected"}, 2, "invalid arguments"},
		{"scan apply rejected", []string{"scan", "--apply"}, 2, "flag provided but not defined"},
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
	// Re-running Apply would try to mask already-modified data; must refuse.
	runCmd(1, "mask", append(append([]string{}, common...), "--apply", "--approved")...)
	// Same inode is never a source + target pair.
	args := []string{"scan", "--source", source, "--target", source,
		"--table", "people", "--column", "email"}
	if got := run(args, &bytes.Buffer{}, &bytes.Buffer{}); got != 2 {
		t.Fatal("same source and target must be rejected")
	}
}
