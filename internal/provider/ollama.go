package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/agent"
)

const OllamaDefaultModel = "qwen2.5-coder:7b"

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Messages []ollamaMessage `json:"messages"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatChunk struct {
	Message *struct {
		Content string `json:"content"`
	} `json:"message,omitempty"`
	Error string `json:"error,omitempty"`
	Done  bool   `json:"done,omitempty"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func CreateOllamaStreamFn(baseURL, model string) agent.StreamFn {
	return CreateOllamaStreamFnWithContext(context.Background(), baseURL, model)
}

func CreateOllamaStreamFnWithContext(reqCtx context.Context, baseURL, model string) agent.StreamFn {
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = OllamaDefaultModel
	}

	return func(msgs []agent.Message, tools []agent.AgentTool) <-chan agent.StreamChunk {
		ch := make(chan agent.StreamChunk, 64)
		go func() {
			defer close(ch)

			payload := ollamaChatRequest{
				Model:    model,
				Stream:   true,
				Messages: buildOllamaMessages(msgs, tools),
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("marshal ollama request: %v", err)}
				return
			}

			req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(raw))
			if err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("build ollama request: %v", err)}
				return
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 2 * time.Minute}
			resp, err := client.Do(req)
			if err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("ollama request failed: %v", err)}
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
				return
			}

			scanner := bufio.NewScanner(resp.Body)
			scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
			var full strings.Builder
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				var chunk ollamaChatChunk
				if err := json.Unmarshal([]byte(line), &chunk); err != nil {
					full.WriteString(line)
					continue
				}
				if detail := strings.TrimSpace(chunk.Error); detail != "" {
					ch <- agent.StreamChunk{Type: "error", Content: detail}
					return
				}
				if chunk.Message != nil && chunk.Message.Content != "" {
					full.WriteString(chunk.Message.Content)
				}
			}
			if err := scanner.Err(); err != nil {
				ch <- agent.StreamChunk{Type: "error", Content: fmt.Sprintf("read ollama stream: %v", err)}
				return
			}

			for _, out := range ollamaOutputToChunks(full.String()) {
				ch <- out
			}
		}()
		return ch
	}
}

func RunOllamaHealthCheck(ctx context.Context, baseURL string) ([]string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(decoded.Models))
	for _, item := range decoded.Models {
		if name := strings.TrimSpace(item.Name); name != "" {
			models = append(models, name)
		}
	}
	return models, nil
}

func EnsureOllamaReady(ctx context.Context, baseURL, model string) error {
	models, err := RunOllamaHealthCheck(ctx, baseURL)
	if err != nil {
		return err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = OllamaDefaultModel
	}
	for _, candidate := range models {
		if candidate == model {
			return nil
		}
	}
	return fmt.Errorf("ollama model %q not found in local tags", model)
}

func buildOllamaMessages(msgs []agent.Message, tools []agent.AgentTool) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(msgs)+1)
	systemPrompt, conversation := buildOllamaPromptParts(msgs, tools)
	if strings.TrimSpace(systemPrompt) != "" {
		out = append(out, ollamaMessage{Role: "system", Content: systemPrompt})
	}
	out = append(out, conversation...)
	return out
}

func buildOllamaPromptParts(msgs []agent.Message, tools []agent.AgentTool) (string, []ollamaMessage) {
	conversation := make([]ollamaMessage, 0, len(msgs))
	var systemParts []string
	for _, msg := range msgs {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		switch role {
		case "system":
			systemParts = append(systemParts, content)
		case "assistant":
			conversation = append(conversation, ollamaMessage{Role: "assistant", Content: content})
		default:
			conversation = append(conversation, ollamaMessage{Role: "user", Content: content})
		}
	}
	if len(tools) > 0 {
		systemParts = append(systemParts, buildOllamaToolInstruction(tools))
	}
	return strings.TrimSpace(strings.Join(systemParts, "\n\n")), conversation
}

func buildOllamaToolInstruction(tools []agent.AgentTool) string {
	var lines []string
	lines = append(lines,
		"Return either plain text or exactly one JSON object for a tool call.",
		`Accepted tool-call envelopes: {"name":"tool_name","arguments":{...}} or {"type":"tool_name", ...top-level arguments...}.`,
		"Do not wrap JSON in markdown fences unless absolutely necessary.",
		"Available tools:",
	)
	for _, tool := range tools {
		line := "- " + strings.TrimSpace(tool.Name)
		if desc := strings.TrimSpace(tool.Description); desc != "" {
			line += ": " + desc
		}
		if len(tool.Schema) > 0 {
			if raw, err := json.Marshal(tool.Schema); err == nil {
				line += " schema=" + string(raw)
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func ollamaOutputToChunks(raw string) []agent.StreamChunk {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	normalized := normalizeOllamaStructuredOutput(trimmed)
	var payload map[string]any
	if err := json.Unmarshal([]byte(normalized), &payload); err == nil {
		if chunk, ok := ollamaStructuredChunk(payload); ok {
			return []agent.StreamChunk{chunk}
		}
	}
	return []agent.StreamChunk{{Type: "text", Content: trimmed}}
}

func normalizeOllamaStructuredOutput(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
				lines = lines[1:]
			}
			if last := len(lines) - 1; last >= 0 && strings.TrimSpace(lines[last]) == "```" {
				lines = lines[:last]
			}
			raw = strings.Join(lines, "\n")
		}
	}
	return strings.TrimSpace(raw)
}

func ollamaStructuredChunk(payload map[string]any) (agent.StreamChunk, bool) {
	if payload == nil {
		return agent.StreamChunk{}, false
	}
	if kind, _ := payload["type"].(string); strings.EqualFold(strings.TrimSpace(kind), "final") {
		if text, _ := payload["content"].(string); strings.TrimSpace(text) != "" {
			return agent.StreamChunk{Type: "text", Content: strings.TrimSpace(text)}, true
		}
	}
	name := strings.TrimSpace(stringValue(payload["name"]))
	if name != "" {
		return agent.StreamChunk{Type: "tool_call", ToolName: name, ToolParams: coerceToolParams(payload["arguments"])}, true
	}
	typeName := strings.TrimSpace(stringValue(payload["type"]))
	if typeName != "" && typeName != "final" {
		params := map[string]any{}
		for key, value := range payload {
			if strings.EqualFold(key, "type") {
				continue
			}
			params[key] = value
		}
		return agent.StreamChunk{Type: "tool_call", ToolName: typeName, ToolParams: params}, true
	}
	return agent.StreamChunk{}, false
}

func coerceToolParams(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case string:
		var decoded map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(typed)), &decoded); err == nil {
			return decoded
		}
	}
	return map[string]any{}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
