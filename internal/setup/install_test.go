package setup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallLatestCLI(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "args.log")
	npmPath := fakeInstallerPath(dir)
	script := fakeInstallerScript(logFile, "")
	if err := os.WriteFile(npmPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake npm: %v", err)
	}

	t.Setenv("WUPHF_CLI_INSTALL_BIN", npmPath)
	t.Setenv("WUPHF_CLI_PACKAGE", "@example/wuphf")

	notice, err := InstallLatestCLI()
	if err != nil {
		t.Fatalf("InstallLatestCLI returned error: %v", err)
	}
	if !strings.Contains(notice, "@example/wuphf") {
		t.Fatalf("expected package name in notice, got %q", notice)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"install", "-g", "@example/wuphf@latest"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("expected args %v, got %v", want, got)
	}
}

func TestInstallLatestCLIReturnsHelpfulFailure(t *testing.T) {
	dir := t.TempDir()
	npmPath := fakeInstallerPath(dir)
	script := fakeInstallerScript("", "boom")
	if err := os.WriteFile(npmPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake npm: %v", err)
	}

	t.Setenv("WUPHF_CLI_INSTALL_BIN", npmPath)
	t.Setenv("WUPHF_CLI_PACKAGE", "@example/wuphf")

	_, err := InstallLatestCLI()
	if err == nil {
		t.Fatal("expected install failure")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected stderr in error, got %v", err)
	}
}

func fakeInstallerPath(dir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "npm.cmd")
	}
	return filepath.Join(dir, "npm")
}

func fakeInstallerScript(logFile, stderr string) string {
	if runtime.GOOS == "windows" {
		if strings.TrimSpace(stderr) != "" {
			return "@echo off\r\necho " + stderr + " 1>&2\r\nexit /b 1\r\n"
		}
		return "@echo off\r\necho %* > \"" + logFile + "\"\r\n"
	}
	if strings.TrimSpace(stderr) != "" {
		return "#!/bin/sh\necho " + shellQuote(stderr) + " >&2\nexit 1\n"
	}
	return "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(logFile) + "\n"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
