//go:build unix

package web

import "syscall"

// readDisk reports usage for the filesystem backing path.
func readDisk(path string) (usedGB, totalGB int, pct int, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, false
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	if total == 0 {
		return 0, 0, 0, false
	}
	used := total - free
	const gb = 1024 * 1024 * 1024
	return int(used / gb), int(total / gb), int(used * 100 / total), true
}
