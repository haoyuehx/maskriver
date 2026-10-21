//go:build linux

package db

import (
	"fmt"
	"os"
	"syscall"
)

// TargetIdentity returns a stable, non-sensitive identity for an opened target
// file: device and inode numbers. It is used to notice that the target path was
// replaced between the run and its recovery. The path is still the operator's
// responsibility; this is not a TOCTOU-proof file-handle binding.
func TargetIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrTargetIdentity
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ErrTargetIdentity
	}
	return fmt.Sprintf("%d:%d", uint64(stat.Dev), uint64(stat.Ino)), nil
}
