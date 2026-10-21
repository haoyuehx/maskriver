// Package testhooks is an inert, test-only deterministic fault-injection seam.
//
// Production code calls Fire at a few precise execution stages. With no hook
// installed Fire is a no-op and allocates nothing; the hook can only be
// installed from tests (or from a test binary acting as a CLI child process) and
// never changes a safety default. It is not a public API and it is not wired to
// environment variables by production code.
package testhooks

import "sync/atomic"

// Hook receives a stage name and may return an error to abort the current
// operation. A hook that wants to simulate process death calls os.Exit itself.
type Hook func(stage string) error

var installed atomic.Pointer[Hook]

// Set installs a hook and returns a function that restores the previous state.
func Set(hook Hook) func() {
	var next *Hook
	if hook != nil {
		next = &hook
	}
	previous := installed.Swap(next)
	return func() { installed.Store(previous) }
}

// Fire runs the installed hook for stage, if any.
func Fire(stage string) error {
	hook := installed.Load()
	if hook == nil || *hook == nil {
		return nil
	}
	return (*hook)(stage)
}

// Enabled reports whether a hook is installed.
func Enabled() bool {
	hook := installed.Load()
	return hook != nil && *hook != nil
}
