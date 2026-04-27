//go:build !windows

package atomicfile

import "os"

// Replace promotes a temp file into place in the same directory.
func Replace(tmp, path string) error {
	return os.Rename(tmp, path)
}
