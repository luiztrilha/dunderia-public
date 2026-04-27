//go:build windows

package atomicfile

// SyncDir is a no-op on Windows because Replace already uses write-through
// move semantics for the rename path.
func SyncDir(string) error {
	return nil
}
