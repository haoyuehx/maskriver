package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// Scope is one explicitly declared table/column operation scope.
type Scope struct{ Table, Column string }

// Runtime describes the explicitly scoped SQLite table/column operations.
// Source and Target must be distinct pre-existing disposable database files.
// Table/Column is the single-scope form kept for compatibility; Scopes is the
// multi-scope form. Exactly one of the two forms may be used.
// No configuration option is capable of authorizing a write.
type Runtime struct {
	Source, Target string
	Table, Column  string
	Scopes         []Scope
	Strategy       contracts.StrategyRef
	KeyID          string
	Approved       bool
	MinSamples     int
	SampleLimit    int
	MinRatio       float64
	BatchSize      int
	Workers        int
}

// ResolvedScopes returns the declared scopes in declared order: the Scopes list
// when it is used, otherwise the single Table/Column pair. Declared order is
// the deterministic plan order and is never sorted.
func (c Runtime) ResolvedScopes() []Scope {
	if len(c.Scopes) > 0 {
		return append([]Scope(nil), c.Scopes...)
	}
	return []Scope{{Table: c.Table, Column: c.Column}}
}

// checkedScopes rejects an ambiguous or malformed scope declaration. It is the
// single place the two configuration forms are reconciled.
func (c Runtime) checkedScopes() ([]Scope, error) {
	if len(c.Scopes) > 0 && (c.Table != "" || c.Column != "") {
		return nil, contracts.ErrInvalid
	}
	scopes := c.ResolvedScopes()
	seen := make(map[Scope]bool, len(scopes))
	for _, scope := range scopes {
		if scope.Table == "" || scope.Column == "" || seen[scope] {
			return nil, contracts.ErrInvalid
		}
		seen[scope] = true
	}
	return scopes, nil
}

func (c Runtime) Validate() error {
	scopes, err := c.checkedScopes()
	if err != nil {
		return err
	}
	if len(scopes) == 0 || c.Source == "" || c.Target == "" ||
		c.MinSamples < 1 || c.SampleLimit < c.MinSamples || c.SampleLimit > 10000 ||
		c.MinRatio <= 0 || c.MinRatio > 1 || c.BatchSize < 1 || c.BatchSize > 1000 ||
		c.Workers < 1 || c.Workers > 16 {
		return contracts.ErrInvalid
	}
	source, err := os.Stat(c.Source)
	if err != nil || !source.Mode().IsRegular() {
		return contracts.ErrInvalid
	}
	target, err := os.Stat(c.Target)
	if err != nil || !target.Mode().IsRegular() || os.SameFile(source, target) {
		return contracts.ErrInvalid
	}
	if !filepath.IsAbs(c.Source) || !filepath.IsAbs(c.Target) {
		return contracts.ErrInvalid
	}
	return nil
}

func (c Runtime) DatabaseOptions(source bool) (contracts.DatabaseOptions, error) {
	if err := c.Validate(); err != nil {
		return contracts.DatabaseOptions{}, err
	}
	path := c.Target
	if source {
		path = c.Source
	}
	value, err := contracts.NewValue(contracts.Text, path)
	if err != nil {
		return contracts.DatabaseOptions{}, err
	}
	return contracts.DatabaseOptions{
		Dialect: contracts.SQLite, DSN: value, DatasetID: "maskriver",
		Schemas: []string{"main"}, MaxOpenConns: 1, ConnectTimeout: 5 * time.Second,
	}, nil
}
