package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty_allowed", "", false},
		{"claude_code", "claude-code", false},
		{"codex", "codex", false},
		{"gemini", "gemini", false},
		{"gemini_vertex", "gemini-vertex", false},
		{"ollama", "ollama", false},
		{"openclaude", "openclaude", false},
		{"unknown", "not-a-runtime", true},
		{"typo", "claud-code", true},
		{"uppercase_rejected", "Codex", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKind(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateKind(%q) err=%v wantErr=%v", tt.in, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "claude-code") {
				t.Fatalf("ValidateKind(%q) err=%v should list valid values", tt.in, err)
			}
		})
	}
}

func TestBindingJSONRoundTrip_Empty(t *testing.T) {
	t.Parallel()
	var b ProviderBinding
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Fatalf("empty binding should marshal to {}, got %s", data)
	}
	var got ProviderBinding
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Kind != "" || got.Model != "" {
		t.Fatalf("empty round-trip lost zero value: %+v", got)
	}
}

func TestBindingJSONRoundTrip_Claude(t *testing.T) {
	t.Parallel()
	in := ProviderBinding{Kind: "claude-code", Model: "claude-sonnet-4.6"}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ProviderBinding
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != in {
		t.Fatalf("round-trip mismatch: got=%+v want=%+v", got, in)
	}
}

func TestBindingJSONRoundTrip_RuntimeKinds(t *testing.T) {
	t.Parallel()
	tests := []ProviderBinding{
		{Kind: "codex", Model: "gpt-5.4"},
		{Kind: "gemini", Model: "gemini-2.5-pro"},
		{Kind: "gemini-vertex", Model: "gemini-3.1-pro-preview"},
		{Kind: "ollama", Model: "qwen2.5-coder:7b"},
		{Kind: "openclaude", Model: "claude-sonnet-4-6"},
	}
	for _, in := range tests {
		in := in
		t.Run(in.Kind, func(t *testing.T) {
			data, err := json.Marshal(in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got ProviderBinding
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != in {
				t.Fatalf("round-trip mismatch: got=%+v want=%+v", got, in)
			}
		})
	}
}

func TestResolveKindFallsBackToGlobal(t *testing.T) {
	t.Parallel()
	global := func() string { return "codex" }
	if got := ResolveKind(ProviderBinding{Kind: ""}, global); got != "codex" {
		t.Fatalf("empty Kind should fall back to global, got %q", got)
	}
	if got := ResolveKind(ProviderBinding{Kind: "claude-code"}, global); got != "claude-code" {
		t.Fatalf("explicit Kind should win, got %q", got)
	}
}
