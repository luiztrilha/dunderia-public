package provider

import (
	"os/exec"
)

var codexCLICandidates = []string{
	"codex",
	"codex.exe",
	"codex.cmd",
	"codex.bat",
	"codex-lb",
	"codex-lb.exe",
	"codex-lb.cmd",
	"codex-lb.bat",
}

func resolveCodexCLIPath(lookPath func(string) (string, error)) (string, string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	for _, candidate := range codexCLICandidates {
		if path, err := lookPath(candidate); err == nil {
			return path, candidate, nil
		}
	}
	return "", "", exec.ErrNotFound
}

func ResolveCodexCLIPath(lookPath func(string) (string, error)) (string, string, error) {
	return resolveCodexCLIPath(lookPath)
}
