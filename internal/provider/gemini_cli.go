package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var secretLikePattern = regexp.MustCompile(`AIza[0-9A-Za-z_\-]{20,}|(?i)(api[_-]?key|token|secret)=\S+`)

// RunGeminiCLIWithModelContext uses the installed Gemini CLI so it can reuse
// the user's local Google OAuth login instead of requiring Gemini API keys.
func RunGeminiCLIWithModelContext(ctx context.Context, model, systemPrompt, prompt string) (string, error) {
	bin, err := resolveGeminiCLIPath()
	if err != nil {
		return "", err
	}

	model = resolveGeminiModel(model)
	args := []string{
		"--skip-trust",
		"--model", model,
		"--prompt", buildGeminiCLIPrompt(systemPrompt, prompt),
		"--output-format", "text",
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = geminiCLIWorkingDir()
	cmd.Env = geminiCLIEnv(os.Environ())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("gemini cli timed out or was cancelled via local OAuth path: %w", ctxErr)
		}
		if msg != "" {
			return "", fmt.Errorf("gemini cli failed via local OAuth path: %v: %s", err, redactGeminiCLIOutput(msg))
		}
		return "", fmt.Errorf("gemini cli failed via local OAuth path: %w", err)
	}

	out := strings.TrimSpace(stripANSI(stdout.String()))
	if out == "" {
		return "", fmt.Errorf("gemini cli returned empty output via local OAuth path: %s", redactGeminiCLIOutput(stderr.String()))
	}
	return out, nil
}

func resolveGeminiCLIPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("WUPHF_GEMINI_CLI_PATH")); p != "" {
		return p, nil
	}
	candidates := []string{"gemini"}
	if runtime.GOOS == "windows" {
		candidates = []string{"gemini.cmd", "gemini.exe", "gemini"}
	}
	for _, candidate := range candidates {
		if p, err := exec.LookPath(candidate); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("gemini cli not found on PATH; install and authenticate it with your Google account")
}

func GeminiCLILocalOAuthAvailable() bool {
	_, err := resolveGeminiCLIPath()
	return err == nil
}

func buildGeminiCLIPrompt(systemPrompt, prompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	prompt = strings.TrimSpace(prompt)
	if systemPrompt == "" {
		return prompt
	}
	return systemPrompt + "\n\n" + prompt
}

func geminiCLIWorkingDir() string {
	if dir := strings.TrimSpace(os.Getenv("WUPHF_GEMINI_CLI_CWD")); dir != "" {
		return dir
	}
	dir := filepath.Join(os.TempDir(), "wuphf-gemini-cli")
	if err := os.MkdirAll(dir, 0o700); err == nil {
		return dir
	}
	return os.TempDir()
}

func geminiCLIEnv(base []string) []string {
	blocked := map[string]struct{}{
		"GEMINI_API_KEY":                 {},
		"GOOGLE_API_KEY":                 {},
		"WUPHF_GEMINI_API_KEY":           {},
		"GOOGLE_APPLICATION_CREDENTIALS": {},
		"GOOGLE_GENAI_USE_VERTEXAI":      {},
	}
	out := make([]string, 0, len(base)+2)
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, found := blocked[strings.ToUpper(key)]; found {
			continue
		}
		out = append(out, entry)
	}
	out = append(out, "NO_COLOR=1", "TERM=xterm-256color")
	return out
}

func stripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}

func redactGeminiCLIOutput(s string) string {
	s = stripANSI(s)
	s = secretLikePattern.ReplaceAllString(s, "[redacted]")
	s = strings.TrimSpace(s)
	if len(s) > 2000 {
		return s[:2000] + "..."
	}
	return s
}
