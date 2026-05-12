package provider

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunGeminiOneShotLiveSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv("WUPHF_LIVE_GEMINI_SMOKE")) != "1" {
		t.Skip("set WUPHF_LIVE_GEMINI_SMOKE=1 to call the live Gemini CLI/API path")
	}
	if !useGeminiAPIForOneShot() && !GeminiCLILocalOAuthAvailable() {
		t.Skip("gemini cli not found on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	out, err := RunGeminiOneShotWithModelContext(ctx, GeminiDefaultModel, "Reply with exactly: ok", "Return ok.")
	if err != nil {
		t.Fatalf("live gemini smoke failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "ok") {
		t.Fatalf("live gemini smoke returned %q, want ok", out)
	}
}
