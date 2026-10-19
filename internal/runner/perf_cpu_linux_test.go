//go:build linux

package runner

import "syscall"

// cpuNS reports process CPU time consumed so far. On platforms without
// getrusage the companion file reports ok=false so the artifacts say UNVERIFIED
// instead of zero.
func cpuNS() (user, system int64, ok bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, 0, false
	}
	return usage.Utime.Nano(), usage.Stime.Nano(), true
}
