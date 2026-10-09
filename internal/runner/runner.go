// Package runner will coordinate scan, plan, apply, and verification.
package runner

import "errors"

// ErrNotImplemented prevents placeholder commands from claiming success.
var ErrNotImplemented = errors.New("not implemented in this scaffold; no database operation performed")
