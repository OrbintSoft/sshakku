//go:build linux

package diagnose

import "syscall"

// realTmpfsSize statfs's path and returns its total size in bytes, or 0 on
// any error (a missing mount point, a race with an unmount).
func realTmpfsSize(path string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0
	}
	return int64(st.Bsize) * int64(st.Blocks) //nolint:unconvert // Bsize's width varies by arch
}
