package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		text string
	}{
		{"default help", nil, 0, "scaffold"},
		{"help", []string{"--help"}, 0, "Usage:"},
		{"version", []string{"--version"}, 0, version},
		{"scan", []string{"scan"}, 2, "not implemented"},
		{"mask dry run", []string{"mask"}, 2, "dry-run=true"},
		{"apply refused", []string{"mask", "--apply"}, 2, "not implemented"},
		{"explicit false", []string{"mask", "--apply=false"}, 2, "dry-run=true"},
		{"validate", []string{"validate"}, 2, "not implemented"},
		{"command help", []string{"mask", "--help"}, 0, "not implemented"},
		{"unknown", []string{"unknown"}, 2, "invalid arguments"},
		{"unknown flag", []string{"mask", "--unknown"}, 2, "flag provided but not defined"},
		{"positional rejected", []string{"mask", "unexpected", "--apply"}, 2, "invalid arguments"},
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
				t.Fatalf("missing %q in output", tt.text)
			}
		})
	}
}
