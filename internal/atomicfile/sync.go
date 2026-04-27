//go:build !windows

package atomicfile

import "os"

// SyncDir flushes directory metadata after an atomic rename.
func SyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
