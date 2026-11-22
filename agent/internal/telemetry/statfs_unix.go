//go:build unix

package telemetry

import "golang.org/x/sys/unix"

func diskUsage(path string) (total, free uint64) {
	if path == "" {
		return 0, 0
	}
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0
	}
	return st.Blocks * uint64(st.Bsize), st.Bavail * uint64(st.Bsize)
}
