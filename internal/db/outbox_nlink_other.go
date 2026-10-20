//go:build !linux

package db

import "os"

// linkCount is unavailable outside the supported Linux platform. Its false
// result makes the preflight fail closed instead of assuming a safe link count.
func linkCount(os.FileInfo) (uint64, bool) { return 0, false }
