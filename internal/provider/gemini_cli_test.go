package provider

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/nex-crm/wuphf/internal/agent"
)

func TestResolveGeminiCLIPathUsesGeminiExecutable(t *testing.T) {
	path, name, err := resolveGeminiCLIPath(func(file string) (string, error) {
		if file == "gemini" {
			return "/usr/local/bin/gemini", nil
		}
		return "", exec.ErrNotFound
	})
	if err != nil {
		t.Fatalf("resolveGeminiCLIPath: %v", err)
	}
	if name != "gemini" {
		t.Fatalf("expected gemini, got %q", name)
	}
	if path != "/usr/local/bin/gemini" {
		t.Fatalf("expected /usr/local/bin/gemini, got %q", path)
	}
}

func TestResolveGeminiCLIPathAcceptsWindowsExe(t *testing.T) {
	path, name, err := resolveGeminiCLIPath(func(file string) (string, error) {
		if file == "gemini.exe" {
			return `C:\Tools\gemini.exe`, nil
		}
		return "", exec.ErrNotFound
	})
	if err != nil {
		t.Fatalf("resolveGeminiCLIPath: %v", err)
	}
	if name != "gemini.exe" {
		t.Fatalf("expected gemini.exe, got %q", name)
	}
	if path != `C:\Tools\gemini.exe` {
		t.Fatalf("expected C:\\Tools\\gemini.exe, got %q", path)
	}
}

func TestResolveGeminiCLIPathAcceptsWindowsCmd(t *testing.T) {
	path, name, err := resolveGeminiCLIPath(func(file string) (string, error) {
		if file == "gemini.cmd" {
			return `C:\Tools\gemini.cmd`, nil
		}
		return "", exec.ErrNotFound
	})
	if err != nil {
		t.Fatalf("resolveGeminiCLIPath: %v", err)
	}
	if name != "gemini.cmd" {
		t.Fatalf("expected gemini.cmd, got %q", name)
	}
	if path != `C:\Tools\gemini.cmd` {
		t.Fatalf("expected C:\\Tools\\gemini.cmd, got %q", path)
	}
}

func TestResolveGeminiCLIPathReturnsNotFoundWhenMissing(t *testing.T) {
	_, _, err := resolveGeminiCLIPath(func(_ string) (string, error) {
		return "", exec.ErrNotFound
	})
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}
}

func TestBuildGeminiCLIArgsIncludesRequiredFlags(t *testing.T) {
	args := buildGeminiCLIArgs("do the thing")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--model") {
		t.Fatalf("expected --model flag, got %q", joined)
	}
	if !strings.Contains(joined, GeminiCLIDefaultModel) {
		t.Fatalf("expected model %q in args, got %q", GeminiCLIDefaultModel, joined)
	}
	if !strings.Contains(joined, "--yolo") {
		t.Fatalf("expected --yolo flag, got %q", joined)
	}
	if !strings.Contains(joined, "-p") {
		t.Fatalf("expected -p flag, got %q", joined)
	}
	if !strings.Contains(joined, "do the thing") {
		t.Fatalf("expected prompt in args, got %q", joined)
	}
}

func TestCreateGeminiCLIStreamFnErrorsWhenBinaryMissing(t *testing.T) {
	old := geminiCLILookPath
	geminiCLILookPath = func(_ string) (string, error) { return "", exec.ErrNotFound }
	defer func() { geminiCLILookPath = old }()

	fn := CreateGeminiCLIStreamFn("ceo")
	chunks := collectStreamChunks(fn([]agent.Message{{Role: "user", Content: "hello"}}, nil))

	if !hasErrorChunkContaining(chunks, "Gemini CLI not found") {
		t.Fatalf("expected missing-binary error, got %#v", chunks)
	}
}

func TestCreateGeminiCLIStreamFnStreamsText(t *testing.T) {
	restore := stubGeminiCLIRuntime(t, "success")
	defer restore()

	fn := CreateGeminiCLIStreamFn("ceo")
	chunks := collectStreamChunks(fn([]agent.Message{
		{Role: "system", Content: "You are the CEO."},
		{Role: "user", Content: "Ship it."},
	}, nil))

	if joinedChunkText(chunks) == "" {
		t.Fatalf("expected non-empty text from gemini CLI, got %#v", chunks)
	}
}

func TestCreateGeminiCLIStreamFnReportsStderrOnFailure(t *testing.T) {
	restore := stubGeminiCLIRuntime(t, "auth-error")
	defer restore()

	fn := CreateGeminiCLIStreamFn("ceo")
	chunks := collectStreamChunks(fn([]agent.Message{{Role: "user", Content: "hello"}}, nil))

	if !hasErrorChunkContaining(chunks, "Gemini CLI error") {
		t.Fatalf("expected Gemini CLI error chunk, got %#v", chunks)
	}
}

// stubGeminiCLIRuntime replaces geminiCLILookPath and geminiCLICommand with a
// subprocess stub that reads GEMINI_CLI_TEST_SCENARIO. Returns a restore func.
func stubGeminiCLIRuntime(t *testing.T, scenario string) func() {
	t.Helper()

	oldLookPath := geminiCLILookPath
	oldCommand := geminiCLICommand
	t.Setenv("GO_WANT_GEMINI_CLI_HELPER_PROCESS", "1")
	t.Setenv("GEMINI_CLI_TEST_SCENARIO", scenario)

	geminiCLILookPath = func(_ string) (string, error) {
		return "/usr/bin/gemini", nil
	}
	geminiCLICommand = func(name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestGeminiCLIHelperProcess", "--"}
		cmdArgs = append(cmdArgs, args...)
		return exec.Command(os.Args[0], cmdArgs...)
	}

	return func() {
		geminiCLILookPath = oldLookPath
		geminiCLICommand = oldCommand
	}
}

func TestGeminiCLIHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_GEMINI_CLI_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	doubleDash := 0
	for i, arg := range args {
		if arg == "--" {
			doubleDash = i
			break
		}
	}
	_ = args[doubleDash+1:]
	_, _ = io.ReadAll(os.Stdin)

	switch os.Getenv("GEMINI_CLI_TEST_SCENARIO") {
	case "success":
		_, _ = os.Stdout.WriteString("Here is the answer.\n")
		_, _ = os.Stdout.WriteString("Done.\n")
		os.Exit(0)
	case "auth-error":
		_, _ = os.Stderr.WriteString("authentication failed: no credentials found\n")
		os.Exit(1)
	default:
		t.Fatalf("unknown gemini CLI helper scenario: %s", os.Getenv("GEMINI_CLI_TEST_SCENARIO"))
	}
}
