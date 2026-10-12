package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/haoyuehx/maskriver/pkg/contracts"
)

// Runtime describes one explicitly scoped SQLite table/column operation.
// Source and Target must be distinct pre-existing disposable database files.
// No configuration option is capable of authorizing a write.
type Runtime struct {
	Source, Target string
	Table, Column  string
	Strategy       contracts.StrategyRef
	KeyID          string
	Approved       bool
	MinSamples     int
	SampleLimit    int
	MinRatio       float64
	BatchSize      int
}

func (c Runtime) Validate() error {
	if c.Source == "" || c.Target == "" || c.Table == "" || c.Column == "" ||
		c.MinSamples < 1 || c.SampleLimit < c.MinSamples || c.SampleLimit > 10000 ||
		c.MinRatio <= 0 || c.MinRatio > 1 || c.BatchSize < 1 || c.BatchSize > 1000 {
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
