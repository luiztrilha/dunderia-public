package provider

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nex-crm/wuphf/internal/agent"
)

func TestResolveGeminiVertexProjectUsesEnvOverride(t *testing.T) {
	t.Setenv("WUPHF_VERTEX_PROJECT", "vertex-env-project")

	orig := geminiVertexLookupGcloudValue
	geminiVertexLookupGcloudValue = func(context.Context, ...string) (string, error) {
		t.Fatal("gcloud lookup should not run when env override is set")
		return "", nil
	}
	defer func() { geminiVertexLookupGcloudValue = orig }()

	got, err := resolveGeminiVertexProject(context.Background())
	if err != nil {
		t.Fatalf("resolve gemini-vertex project: %v", err)
	}
	if got != "vertex-env-project" {
		t.Fatalf("expected env project, got %q", got)
	}
}

func TestResolveGeminiVertexProjectFallsBackToGcloud(t *testing.T) {
	t.Setenv("WUPHF_VERTEX_PROJECT", "")
	t.Setenv("VERTEX_AI_PROJECT", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GCP_PROJECT", "")

	orig := geminiVertexLookupGcloudValue
	geminiVertexLookupGcloudValue = func(_ context.Context, args ...string) (string, error) {
		if len(args) != 3 || args[0] != "config" || args[1] != "get-value" || args[2] != "project" {
			t.Fatalf("unexpected gcloud args: %#v", args)
		}
		return "vertex-gcloud-project", nil
	}
	defer func() { geminiVertexLookupGcloudValue = orig }()

	got, err := resolveGeminiVertexProject(context.Background())
	if err != nil {
		t.Fatalf("resolve gemini-vertex project via gcloud: %v", err)
	}
	if got != "vertex-gcloud-project" {
		t.Fatalf("expected gcloud project, got %q", got)
	}
}

func TestResolveGeminiVertexLocationDefaultsToGlobal(t *testing.T) {
	if got := resolveGeminiVertexLocation(); got != "global" {
		t.Fatalf("expected default vertex location global, got %q", got)
	}
}

func TestGeminiVertexStreamFnBuilds(t *testing.T) {
	fn := CreateGeminiVertexStreamFn()
	if fn == nil {
		t.Fatal("expected non-nil StreamFn")
	}
}

func TestSplitSystemInstructionRemovesSystemMessagesFromContents(t *testing.T) {
	systemInstruction, contents := splitSystemInstruction([]agent.Message{
		{Role: "system", Content: "Act like a precise assistant."},
		{Role: "user", Content: "ping"},
		{Role: "assistant", Content: "pong"},
	})

	if systemInstruction == nil {
		t.Fatal("expected system instruction content")
	}
	if got := systemInstruction.Parts[0].Text; got != "Act like a precise assistant." {
		t.Fatalf("unexpected system instruction text %q", got)
	}
	if len(contents) != 2 {
		t.Fatalf("expected only non-system contents, got %d", len(contents))
	}
	if contents[0].Role != "user" || contents[1].Role != "model" {
		t.Fatalf("unexpected content roles: %q, %q", contents[0].Role, contents[1].Role)
	}
}

func TestGeminiVertexSmoke(t *testing.T) {
	if os.Getenv("WUPHF_RUN_GEMINI_VERTEX_SMOKE") != "1" {
		t.Skip("set WUPHF_RUN_GEMINI_VERTEX_SMOKE=1 to run the live Vertex smoke test")
	}

	out, err := RunGeminiVertexOneShot(
		"Respond with exactly GEMINI_VERTEX_OK.",
		"Return exactly GEMINI_VERTEX_OK.",
	)
	if err != nil {
		t.Fatalf("gemini-vertex smoke failed: %v", err)
	}
	if !strings.Contains(out, "GEMINI_VERTEX_OK") {
		t.Fatalf("unexpected gemini-vertex smoke output %q", out)
	}
}
