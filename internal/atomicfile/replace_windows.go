//go:build windows

package atomicfile

import (
	"os"

	"golang.org/x/sys/windows"
)

// Replace promotes a temp file into place using the Windows replace-existing
// move semantics so existing files are overwritten in one step.
func Replace(tmp, path string) error {
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return &os.LinkError{Op: "replace", Old: tmp, New: path, Err: err}
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return &os.LinkError{Op: "replace", Old: tmp, New: path, Err: err}
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "replace", Old: tmp, New: path, Err: err}
	}
	return nil
}
