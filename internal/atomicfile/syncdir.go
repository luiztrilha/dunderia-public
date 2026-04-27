//go:build !windows

package atomicfile

import "os"

// SyncDir flushes the parent directory metadata after an atomic replace so the
// rename itself is durable across power loss on POSIX filesystems.
func SyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
