// Package workspace manages WUPHF's on-disk runtime state.
//
//   - Reset: narrow. Clears broker runtime state and task worktrees so a stuck
//     office can restart clean. Preserves team, company, office history, and
//     workflows.
//
//   - Shred: legacy compatibility entry point. It no longer deletes local
//     office state because channels, agents, messages, and task receipts are
//     user data.
//
// Preserved in both cases: logs/, sessions/, codex-headless/, providers/,
// calendar.json, config.json, and in-flight task worktrees. Branches and local
// changes inside those worktrees remain intact.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nex-crm/wuphf/internal/config"
)

// Result reports which paths the operation actually removed and collects any
// non-fatal errors. A path is "removed" only if it existed before the call.
type Result struct {
	Removed []string `json:"removed"`
	Errors  []string `json:"errors,omitempty"`
}

// ClearRuntime performs a narrow reset: deletes the broker state file and the
// task-worktrees registry (the worktrees themselves stay on disk). Safe to
// call when no broker is running. Callers that need to stop a live broker or
// tmux session must do so separately — this package only touches the disk.
func ClearRuntime() (Result, error) {
	home, err := wuphfHome()
	if err != nil {
		return Result{}, err
	}
	var res Result
	// ~/.wuphf/team/ holds broker-state.json, office.pid, and the snapshot.
	// Wiping the whole dir is simpler than enumerating and matches what
	// the broker rebuilds on next boot.
	res.removeIfPresent(filepath.Join(home, "team"))
	return res, nil
}

// Shred is intentionally non-destructive. Older builds used this as a full
// workspace wipe, but that made it too easy to destroy office topology and
// history. Keep the function for API/CLI compatibility while preserving data.
func Shred() (Result, error) {
	if _, err := wuphfHome(); err != nil {
		return Result{}, err
	}
	return Result{}, nil
}

// wuphfHome returns the absolute path to ~/.wuphf, honoring WUPHF_RUNTIME_HOME
// so tests and sandboxed runs stay isolated from the real user directory.
func wuphfHome() (string, error) {
	home := config.RuntimeHomeDir()
	if home == "" {
		return "", errors.New("workspace: could not resolve home directory")
	}
	return filepath.Join(home, ".wuphf"), nil
}

func (r *Result) removeIfPresent(path string) {
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		r.Errors = append(r.Errors, fmt.Sprintf("stat %s: %v", path, err))
		return
	}
	var rmErr error
	if info.IsDir() {
		rmErr = os.RemoveAll(path)
	} else {
		rmErr = os.Remove(path)
	}
	if rmErr != nil {
		r.Errors = append(r.Errors, fmt.Sprintf("remove %s: %v", path, rmErr))
		return
	}
	r.Removed = append(r.Removed, path)
}
