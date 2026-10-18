package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// scopedRuntime returns a valid single-scope Runtime over two disposable files.
// Validate needs real regular files, so tests may not build paths by hand.
func scopedRuntime(t *testing.T) Runtime {
	t.Helper()
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.db"), filepath.Join(dir, "target.db")
	for _, path := range []string{source, target} {
		if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return Runtime{
		Source: source, Target: target, Table: "people", Column: "email",
		MinSamples: 1, SampleLimit: 10, MinRatio: .9, BatchSize: 2, Workers: 1,
	}
}

func TestResolvedScopes(t *testing.T) {
	single := scopedRuntime(t)
	if got, want := single.ResolvedScopes(), []Scope{{Table: "people", Column: "email"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("single scope resolved to %+v, want %+v", got, want)
	}
	declared := []Scope{{Table: "people", Column: "email"}, {Table: "contacts", Column: "email"}}
	multiple := single
	multiple.Table, multiple.Column = "", ""
	multiple.Scopes = append([]Scope(nil), declared...)
	if got := multiple.ResolvedScopes(); !reflect.DeepEqual(got, declared) {
		t.Fatalf("declared order not preserved: %+v", got)
	}
	// ResolvedScopes hands out a copy, so a caller can not mutate the config.
	multiple.ResolvedScopes()[0].Table = "mutated"
	if multiple.Scopes[0].Table != "people" {
		t.Fatal("ResolvedScopes exposed the configured slice")
	}
}

func TestValidateScopeDeclarations(t *testing.T) {
	clear := func(c *Runtime) { c.Table, c.Column = "", "" }
	tests := []struct {
		name    string
		mutate  func(*Runtime)
		wantErr bool
	}{
		{"single scope", func(*Runtime) {}, false},
		{"declared scopes", func(c *Runtime) {
			clear(c)
			c.Scopes = []Scope{{Table: "people", Column: "email"}, {Table: "contacts", Column: "email"}}
		}, false},
		{"same table two columns", func(c *Runtime) {
			clear(c)
			c.Scopes = []Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "backup_email"}}
		}, false},
		{"duplicate scope", func(c *Runtime) {
			clear(c)
			c.Scopes = []Scope{{Table: "people", Column: "email"}, {Table: "people", Column: "email"}}
		}, true},
		{"empty table", func(c *Runtime) {
			clear(c)
			c.Scopes = []Scope{{Table: "", Column: "email"}}
		}, true},
		{"empty column", func(c *Runtime) {
			clear(c)
			c.Scopes = []Scope{{Table: "people", Column: ""}}
		}, true},
		{"scopes with table", func(c *Runtime) {
			c.Scopes = []Scope{{Table: "contacts", Column: "email"}}
		}, true},
		{"scopes with column", func(c *Runtime) {
			c.Table = ""
			c.Scopes = []Scope{{Table: "contacts", Column: "email"}}
		}, true},
		{"no scope at all", func(c *Runtime) { clear(c) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := scopedRuntime(t)
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantErr {
				if !errors.Is(err, contracts.ErrInvalid) {
					t.Fatalf("Validate error=%v, want ErrInvalid", err)
				}
				if _, err := cfg.DatabaseOptions(true); !errors.Is(err, contracts.ErrInvalid) {
					t.Fatalf("DatabaseOptions error=%v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate error=%v, want nil", err)
			}
		})
	}
}
