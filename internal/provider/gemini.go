package provider

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/nex-crm/wuphf/internal/agent"
)

const GeminiDefaultModel = "gemini-3.1-pro-preview"

// CreateGeminiStreamFn returns a StreamFn backed by the Gemini API.
func CreateGeminiStreamFn(apiKey string) agent.StreamFn {
	return CreateGeminiStreamFnContext(context.Background(), apiKey)
}

// CreateGeminiStreamFnContext returns a Gemini StreamFn bound to the provided
// context so headless turns can be cancelled by their scheduler timeout.
func CreateGeminiStreamFnContext(ctx context.Context, apiKey string) agent.StreamFn {
	return CreateGeminiStreamFnForModelContext(ctx, apiKey, "")
}

func CreateGeminiStreamFnForModelContext(ctx context.Context, apiKey string, model string) agent.StreamFn {
	model = resolveGeminiModel(model)
	return func(msgs []agent.Message, tools []agent.AgentTool) <-chan agent.StreamChunk {
		ch := make(chan agent.StreamChunk, 64)
		go func() {
			defer close(ch)

			client, err := genai.NewClient(ctx, &genai.ClientConfig{
				APIKey:  apiKey,
				Backend: genai.BackendGeminiAPI,
			})
			if err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("Gemini client initialization failed: %v. Check your API key and network connection.", err)}
				return
			}

			contents := msgsToGenAIContents(msgs)
			config := &genai.GenerateContentConfig{}

			if len(tools) > 0 {
				config.Tools = []*genai.Tool{agentToolsToGenAI(tools)}
			}

			stream := client.Models.GenerateContentStream(ctx, model, contents, config)
			for result, err := range stream {
				if err != nil {
					ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("Gemini streaming failed: %v. The model may be unavailable or the request was rejected.", err)}
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

func resolveGeminiModel(model string) string {
	if trimmed := strings.TrimSpace(model); trimmed != "" {
		return trimmed
	}
	return GeminiDefaultModel
}

// msgsToGenAIContents converts agent messages to the genai Content slice.
// Gemini requires alternating user/model turns.
func msgsToGenAIContents(msgs []agent.Message) []*genai.Content {
	contents := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: []*genai.Part{{Text: m.Content}},
		})
	}
	return contents
}

// agentToolsToGenAI converts AgentTools to a single genai.Tool with function declarations.
func agentToolsToGenAI(tools []agent.AgentTool) *genai.Tool {
	decls := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
		})
	}
	return &genai.Tool{FunctionDeclarations: decls}
}
