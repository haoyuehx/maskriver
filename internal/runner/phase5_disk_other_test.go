//go:build !linux

package runner

func phase5DiskAvailable() (uint64, string) {
	return 0, "UNVERIFIED: native disk budget sampler is Linux-only"
}
