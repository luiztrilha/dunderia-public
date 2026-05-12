package provider

import (
	"strings"
	"testing"
)

func TestGeminiCLIEnvRemovesAPIKeyProviders(t *testing.T) {
	got := geminiCLIEnv([]string{
		"GEMINI_API_KEY=gemini-key",
		"GOOGLE_API_KEY=google-key",
		"WUPHF_GEMINI_API_KEY=wuphf-key",
		"GOOGLE_APPLICATION_CREDENTIALS=C:/secret.json",
		"GOOGLE_GENAI_USE_VERTEXAI=true",
		"PATH=C:/bin",
	})

	joined := strings.Join(got, "\n")
	for _, blocked := range []string{
		"GEMINI_API_KEY=",
		"GOOGLE_API_KEY=",
		"WUPHF_GEMINI_API_KEY=",
		"GOOGLE_APPLICATION_CREDENTIALS=",
		"GOOGLE_GENAI_USE_VERTEXAI=",
	} {
		if strings.Contains(joined, blocked) {
			t.Fatalf("gemini cli env leaked %s in %q", blocked, joined)
		}
	}
	if !strings.Contains(joined, "PATH=C:/bin") {
		t.Fatalf("gemini cli env should preserve unrelated variables, got %q", joined)
	}
}

func TestBuildGeminiCLIPromptIncludesSystemPrompt(t *testing.T) {
	got := buildGeminiCLIPrompt("system", "user")
	if got != "system\n\nuser" {
		t.Fatalf("buildGeminiCLIPrompt = %q", got)
	}
}

func TestRedactGeminiCLIOutputHidesGoogleAPIKeys(t *testing.T) {
	got := redactGeminiCLIOutput("failed AIzaXXXXXXXXXXXXXXXXXXXXXXXXXXXX api_key=secret")
	if strings.Contains(got, "AIza") || strings.Contains(got, "secret") {
		t.Fatalf("redacted output leaked secret-like content: %q", got)
	}
}
