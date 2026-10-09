// Package config owns validated runtime options. File loading is not implemented.
package config

// Options permits writes only on explicit CLI intent. The zero value is safe.
// Apply alone is not authorization to execute: the runner must validate a plan.
type Options struct {
	Apply bool
}

// DryRun reports whether write intent is absent.
func (o Options) DryRun() bool { return !o.Apply }
