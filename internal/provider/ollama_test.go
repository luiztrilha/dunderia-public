package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/agent"
)

func TestCreateOllamaStreamFnParsesStructuredToolCallEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{
				"content": `{"name":"team_broadcast","arguments":{"channel":"general","content":"Hello","reply_to_id":"msg-1"}}`,
			},
			"done": true,
		})
	}))
	defer srv.Close()

	fn := CreateOllamaStreamFn(srv.URL, OllamaDefaultModel)
	chunks := collectStreamChunks(fn([]agent.Message{{Role: "user", Content: "reply"}}, nil))
	if len(chunks) != 1 || chunks[0].Type != "tool_call" {
		t.Fatalf("expected one tool_call chunk, got %#v", chunks)
	}
	if chunks[0].ToolName != "team_broadcast" {
		t.Fatalf("expected team_broadcast, got %#v", chunks[0])
	}
	if got := stringValue(chunks[0].ToolParams["channel"]); got != "general" {
		t.Fatalf("expected general channel, got %#v", chunks[0].ToolParams)
	}
}

func TestCreateOllamaStreamFnParsesFencedToolCallWithoutType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{
				"content": "```json\n{\"name\":\"team_broadcast\",\"arguments\":{\"channel\":\"estagiario__human\",\"content\":\"Ol\u00e1!\",\"reply_to_id\":\"msg-9\"}}\n```",
			},
			"done": true,
		})
	}))
	defer srv.Close()

	fn := CreateOllamaStreamFn(srv.URL, OllamaDefaultModel)
	chunks := collectStreamChunks(fn([]agent.Message{{Role: "user", Content: "reply"}}, nil))
	if len(chunks) != 1 || chunks[0].Type != "tool_call" {
		t.Fatalf("expected fenced JSON to become tool_call, got %#v", chunks)
	}
	if chunks[0].ToolName != "team_broadcast" {
		t.Fatalf("expected team_broadcast, got %#v", chunks[0])
	}
}

func TestCreateOllamaStreamFnParsesTopLevelTypeToolEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{
				"content": `{"type":"team_broadcast","channel":"estagiario__human","reply_to_id":"msg-10","content":"Olá! Como posso ajudar você hoje?"}`,
			},
			"done": true,
		})
	}))
	defer srv.Close()

	fn := CreateOllamaStreamFn(srv.URL, OllamaDefaultModel)
	chunks := collectStreamChunks(fn([]agent.Message{{Role: "user", Content: "reply"}}, nil))
	if len(chunks) != 1 || chunks[0].Type != "tool_call" {
		t.Fatalf("expected top-level type envelope to become tool_call, got %#v", chunks)
	}
	if chunks[0].ToolName != "team_broadcast" {
		t.Fatalf("expected team_broadcast, got %#v", chunks[0])
	}
	if got := stringValue(chunks[0].ToolParams["reply_to_id"]); got != "msg-10" {
		t.Fatalf("expected reply_to_id msg-10, got %#v", chunks[0].ToolParams)
	}
}

func TestCreateOllamaStreamFnWithContextCancelsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	fn := CreateOllamaStreamFnWithContext(ctx, srv.URL, OllamaDefaultModel)
	chunks := fn([]agent.Message{{Role: "user", Content: "hello"}}, nil)
	cancel()

	select {
	case chunk, ok := <-chunks:
		if !ok {
			t.Fatal("expected cancellation error chunk before channel close")
		}
		if chunk.Type != "error" {
			t.Fatalf("chunk type = %q, want error", chunk.Type)
		}
		if !strings.Contains(strings.ToLower(chunk.Content), "context canceled") {
			t.Fatalf("unexpected cancellation message %q", chunk.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancellation chunk")
	}
}
