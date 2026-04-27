package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"google.golang.org/genai"

	"github.com/nex-crm/wuphf/internal/agent"
)

const GeminiVertexDefaultModel = "gemini-3.1-pro-preview"

var geminiVertexLookupGcloudValue = func(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateGeminiVertexStreamFn returns a StreamFn backed by Gemini on Vertex AI,
// which consumes the caller's Google Cloud project quota instead of the direct
// Gemini API key/account path.
func CreateGeminiVertexStreamFn() agent.StreamFn {
	return CreateGeminiVertexStreamFnForModel("")
}

func CreateGeminiVertexStreamFnForModel(model string) agent.StreamFn {
	return CreateGeminiVertexStreamFnForModelContext(context.Background(), model)
}

func CreateGeminiVertexStreamFnForModelContext(ctx context.Context, model string) agent.StreamFn {
	model = resolveGeminiVertexModel(model)
	return func(msgs []agent.Message, tools []agent.AgentTool) <-chan agent.StreamChunk {
		ch := make(chan agent.StreamChunk, 64)
		go func() {
			defer close(ch)

			project, err := resolveGeminiVertexProject(ctx)
			if err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("Gemini Vertex project is not configured: %v", err)}
				return
			}
			location := resolveGeminiVertexLocation()

			client, err := genai.NewClient(ctx, &genai.ClientConfig{
				Project:  project,
				Location: location,
				Backend:  genai.BackendVertexAI,
			})
			if err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("Gemini Vertex client initialization failed: %v. Verify ADC/gcloud auth, project, and location.", err)}
				return
			}

			systemInstruction, contents := splitSystemInstruction(msgs)
			cfg := &genai.GenerateContentConfig{}
			if systemInstruction != nil {
				cfg.SystemInstruction = systemInstruction
			}
			if len(tools) > 0 {
				cfg.Tools = []*genai.Tool{agentToolsToGenAI(tools)}
			}

			stream := client.Models.GenerateContentStream(ctx, model, contents, cfg)
			for result, err := range stream {
				if err != nil {
					ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("Gemini Vertex streaming failed: %v. Verify Vertex billing/quota for the configured project.", err)}
					return
				}
				for _, cand := range result.Candidates {
					if cand.Content == nil {
						continue
					}
					for _, part := range cand.Content.Parts {
						if txt := part.Text; txt != "" {
							ch <- agent.StreamChunk{Type: "text", Content: txt}
						}
					}
				}
			}
		}()
		return ch
	}
}

func RunGeminiVertexOneShot(systemPrompt, prompt string) (string, error) {
	return RunGeminiVertexOneShotWithModel("", systemPrompt, prompt)
}

func RunGeminiVertexOneShotWithModel(model, systemPrompt, prompt string) (string, error) {
	return RunGeminiVertexOneShotWithModelContext(context.Background(), model, systemPrompt, prompt)
}

func RunGeminiVertexOneShotWithModelContext(ctx context.Context, model, systemPrompt, prompt string) (string, error) {
	fn := CreateGeminiVertexStreamFnForModelContext(ctx, model)
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

func resolveGeminiVertexModel(model string) string {
	if trimmed := strings.TrimSpace(model); trimmed != "" {
		return trimmed
	}
	return GeminiVertexDefaultModel
}

func ResolveGeminiVertexProject(ctx context.Context) (string, error) {
	return resolveGeminiVertexProject(ctx)
}

func ResolveGeminiVertexLocation() string {
	return resolveGeminiVertexLocation()
}

func resolveGeminiVertexProject(ctx context.Context) (string, error) {
	for _, value := range []string{
		os.Getenv("WUPHF_VERTEX_PROJECT"),
		os.Getenv("VERTEX_AI_PROJECT"),
		os.Getenv("GOOGLE_CLOUD_PROJECT"),
		os.Getenv("GCP_PROJECT"),
	} {
		if trimmed := strings.TrimSpace(value); trimmed != "" && trimmed != "(unset)" {
			return trimmed, nil
		}
	}
	if value, err := geminiVertexLookupGcloudValue(ctx, "config", "get-value", "project"); err == nil {
		if trimmed := strings.TrimSpace(value); trimmed != "" && trimmed != "(unset)" {
			return trimmed, nil
		}
	}
	return "", fmt.Errorf("set WUPHF_VERTEX_PROJECT/GOOGLE_CLOUD_PROJECT or run `gcloud config set project <id>`")
}

func resolveGeminiVertexLocation() string {
	for _, value := range []string{
		os.Getenv("WUPHF_VERTEX_LOCATION"),
		os.Getenv("VERTEX_AI_LOCATION"),
		os.Getenv("GOOGLE_CLOUD_LOCATION"),
		os.Getenv("GOOGLE_CLOUD_REGION"),
	} {
		if trimmed := strings.TrimSpace(value); trimmed != "" && trimmed != "(unset)" {
			return trimmed
		}
	}
	return "global"
}

func splitSystemInstruction(msgs []agent.Message) (*genai.Content, []*genai.Content) {
	var systemParts []string
	filtered := make([]agent.Message, 0, len(msgs))
	for _, msg := range msgs {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "system") {
			if text := strings.TrimSpace(msg.Content); text != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}
		filtered = append(filtered, msg)
	}
	if len(systemParts) == 0 {
		return nil, msgsToGenAIContents(filtered)
	}
	return &genai.Content{
		Parts: []*genai.Part{{Text: strings.Join(systemParts, "\n\n")}},
	}, msgsToGenAIContents(filtered)
}
