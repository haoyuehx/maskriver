//go:build linux

package db

import (
	"os"
	"syscall"
)

// linkCount reports the hard-link count of a regular file. It is used only by
// the Linux local-SQLite preflight; other platforms are unsupported for the
// streaming outbox in this version.
func linkCount(info os.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Nlink), true
}
