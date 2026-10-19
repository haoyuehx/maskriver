//go:build !linux

package runner

// cpuNS is unavailable here; callers record UNVERIFIED rather than a zero that
// would look like a measurement.
func cpuNS() (user, system int64, ok bool) { return 0, 0, false }
