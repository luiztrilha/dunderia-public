package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/nex-crm/wuphf/internal/agent"
	"github.com/nex-crm/wuphf/internal/config"
)

// RunConfiguredOneShot runs a single-shot generation using the configured LLM provider.
// Providers without a dedicated one-shot path fall back to Claude for now.
func RunConfiguredOneShot(systemPrompt, prompt, cwd string) (string, error) {
	switch config.ResolveLLMProvider("") {
	case "codex":
		return RunCodexOneShot(systemPrompt, prompt, cwd)
	case "gemini":
		return RunGeminiOneShot(systemPrompt, prompt)
	case "gemini-vertex":
		return RunGeminiVertexOneShot(systemPrompt, prompt)
	case "ollama":
		return RunOllamaOneShot(systemPrompt, prompt)
	default:
		return RunClaudeOneShot(systemPrompt, prompt, cwd)
	}
}

func RunGeminiOneShot(systemPrompt, prompt string) (string, error) {
	return RunGeminiOneShotContext(context.Background(), systemPrompt, prompt)
}

func RunGeminiOneShotContext(ctx context.Context, systemPrompt, prompt string) (string, error) {
	apiKey := config.ResolveGeminiAPIKey()
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("gemini api key not configured")
	}
	fn := CreateGeminiStreamFnContext(ctx, apiKey)
	msgs := make([]agent.Message, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		msgs = append(msgs, agent.Message{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, agent.Message{Role: "user", Content: prompt})

	var parts []string
	for chunk := range fn(msgs, nil) {
		switch chunk.Type {
		case "text":
			if strings.TrimSpace(chunk.Content) != "" {
				parts = append(parts, chunk.Content)
			}
		case "error":
			return "", fmt.Errorf("%s", chunk.Content)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "")), nil
}

func RunOllamaOneShot(systemPrompt, prompt string) (string, error) {
	fn := CreateOllamaStreamFn(config.ResolveOllamaBaseURL(), OllamaDefaultModel)
	msgs := make([]agent.Message, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		msgs = append(msgs, agent.Message{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, agent.Message{Role: "user", Content: prompt})

	var parts []string
	for chunk := range fn(msgs, nil) {
		switch chunk.Type {
		case "text":
			if strings.TrimSpace(chunk.Content) != "" {
				parts = append(parts, chunk.Content)
			}
		case "error":
			return "", fmt.Errorf("%s", chunk.Content)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "")), nil
}
