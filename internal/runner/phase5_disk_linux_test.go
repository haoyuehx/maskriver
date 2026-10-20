//go:build linux

package runner

import "syscall"

func phase5DiskAvailable() (uint64, string) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(".", &stat); err != nil {
		return 0, "UNVERIFIED: statfs unavailable"
	}
	return stat.Bavail * uint64(stat.Bsize), "Linux statfs available blocks"
}
