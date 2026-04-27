package team

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nex-crm/wuphf/internal/provider"
)

func TestBuildHeadlessGeminiPromptOverridesToolUsage(t *testing.T) {
	l := newHeadlessLauncherForTest()

	prompt := l.buildHeadlessGeminiPrompt("pm")

	for _, fragment := range []string{
		"== GEMINI HEADLESS RUNTIME OVERRIDE ==",
		"This runtime has no tool execution.",
		"Ignore any earlier instruction to call tools",
		"Return only the final reply body as plain text.",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("expected prompt to contain %q, got %q", fragment, prompt)
		}
	}
}

func TestRunHeadlessGeminiTurnUsesToollessPromptAndPublishesFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WUPHF_BROKER_STATE_PATH", filepath.Join(t.TempDir(), "broker-state.json"))
	oldGeminiRunner := headlessGeminiVertexOneShot
	defer func() { headlessGeminiVertexOneShot = oldGeminiRunner }()

	var gotModel string
	var gotSystemPrompt string
	var gotPrompt string
	headlessGeminiVertexOneShot = func(_ context.Context, model, systemPrompt, prompt string) (string, error) {
		gotModel = model
		gotSystemPrompt = systemPrompt
		gotPrompt = prompt
		return "Plain fallback reply", nil
	}

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	ensureTestMemberAccess(l.broker, "general", "pm", "PM")
	before := len(l.broker.AllMessages())
	replyTo := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	notification := fmt.Sprintf(`Call team_broadcast with my_slug "pm", channel "general", reply_to_id "%s" and reply with the answer.`, replyTo)
	if err := l.runHeadlessGeminiTurn(context.Background(), "pm", provider.KindGeminiVertex, notification); err != nil {
		t.Fatalf("runHeadlessGeminiTurn: %v", err)
	}

	if !strings.Contains(gotPrompt, notification) {
		t.Fatalf("prompt should include notification, got %q", gotPrompt)
	}
	if gotModel != provider.GeminiVertexDefaultModel {
		t.Fatalf("expected default vertex model %q, got %q", provider.GeminiVertexDefaultModel, gotModel)
	}
	if strings.Contains(gotPrompt, "== PRIVATE MEMORY ==") {
		t.Fatalf("prompt should not inherit unrelated private memory in isolated test broker, got %q", gotPrompt)
	}
	if !strings.Contains(gotSystemPrompt, "This runtime has no tool execution.") {
		t.Fatalf("system prompt should include gemini runtime override, got %q", gotSystemPrompt)
	}

	msgs := l.broker.AllMessages()
	if len(msgs) != before+1 {
		t.Fatalf("expected one new fallback message, got before=%d after=%d", before, len(msgs))
	}
	msg := msgs[len(msgs)-1]
	if msg.From != "pm" {
		t.Fatalf("message from = %q, want pm", msg.From)
	}
	if msg.Content != "Plain fallback reply" {
		t.Fatalf("message content = %q", msg.Content)
	}
	if msg.ReplyTo != replyTo {
		t.Fatalf("reply_to = %q, want %q", msg.ReplyTo, replyTo)
	}
}

func TestRunHeadlessGeminiTurnRejectsEmptyOutput(t *testing.T) {
	oldGeminiRunner := headlessGeminiVertexOneShot
	defer func() { headlessGeminiVertexOneShot = oldGeminiRunner }()

	calls := 0
	headlessGeminiVertexOneShot = func(_ context.Context, model, systemPrompt, prompt string) (string, error) {
		calls++
		return "   ", nil
	}

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	before := len(l.broker.AllMessages())
	replyTo := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	notification := fmt.Sprintf(`Call team_broadcast with my_slug "pm", channel "general", reply_to_id "%s" and reply with the answer.`, replyTo)
	err := l.runHeadlessGeminiTurn(context.Background(), "pm", provider.KindGeminiVertex, notification)
	if err == nil {
		t.Fatal("expected empty gemini output to fail")
	}
	if !strings.Contains(err.Error(), "no plain-text reply") {
		t.Fatalf("unexpected error %q", err)
	}
	if calls != headlessGeminiEmptyReplyRetryLimit+1 {
		t.Fatalf("expected %d gemini attempts, got %d", headlessGeminiEmptyReplyRetryLimit+1, calls)
	}

	if msgs := l.broker.AllMessages(); len(msgs) != before {
		t.Fatalf("expected no new fallback message on empty output, got before=%d after=%d", before, len(msgs))
	}
}

func TestRunHeadlessGeminiTurnPassesTurnContextToProvider(t *testing.T) {
	oldGeminiRunner := headlessGeminiVertexOneShot
	defer func() { headlessGeminiVertexOneShot = oldGeminiRunner }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	headlessGeminiVertexOneShot = func(ctx context.Context, model, systemPrompt, prompt string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			t.Fatal("expected cancelled turn context to reach gemini provider")
			return "", nil
		}
	}

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	notification := fmt.Sprintf(`Call team_broadcast with my_slug "pm", channel "general", reply_to_id "%s" and reply with the answer.`, fmt.Sprintf("msg-%d", time.Now().UnixNano()))
	err := l.runHeadlessGeminiTurn(ctx, "pm", provider.KindGeminiVertex, notification)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context cancellation from gemini provider, got %v", err)
	}
}

func TestRunHeadlessGeminiTurnRetriesEmptyOutputOnceBeforePublishingFallback(t *testing.T) {
	oldGeminiRunner := headlessGeminiVertexOneShot
	defer func() { headlessGeminiVertexOneShot = oldGeminiRunner }()
	t.Setenv("HOME", t.TempDir())

	calls := 0
	headlessGeminiVertexOneShot = func(_ context.Context, model, systemPrompt, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return "   ", nil
		}
		return "Plain fallback reply", nil
	}

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	ensureTestMemberAccess(l.broker, "general", "pm", "PM")
	before := len(l.broker.AllMessages())
	replyTo := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	notification := fmt.Sprintf(`Call team_broadcast with my_slug "pm", channel "general", reply_to_id "%s" and reply with the answer.`, replyTo)
	if err := l.runHeadlessGeminiTurn(context.Background(), "pm", provider.KindGeminiVertex, notification); err != nil {
		t.Fatalf("runHeadlessGeminiTurn: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected retry after empty output, got %d calls", calls)
	}

	msgs := l.broker.AllMessages()
	if len(msgs) != before+1 {
		t.Fatalf("expected one new fallback message, got before=%d after=%d", before, len(msgs))
	}
	msg := msgs[len(msgs)-1]
	if msg.Content != "Plain fallback reply" {
		t.Fatalf("message content = %q", msg.Content)
	}
}

func TestRunHeadlessGeminiTurnPublishesFallbackDespiteUnrelatedChannelPost(t *testing.T) {
	oldGeminiRunner := headlessGeminiVertexOneShot
	defer func() { headlessGeminiVertexOneShot = oldGeminiRunner }()
	t.Setenv("HOME", t.TempDir())

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	ensureTestMemberAccess(l.broker, "review", "pm", "PM")
	replyTo := fmt.Sprintf("msg-%d", time.Now().UnixNano())

	headlessGeminiVertexOneShot = func(_ context.Context, model, systemPrompt, prompt string) (string, error) {
		if _, err := l.broker.PostMessage("pm", "review", "Unrelated progress update", nil, "msg-review"); err != nil {
			t.Fatalf("post unrelated progress update: %v", err)
		}
		return "DM fallback reply", nil
	}

	notification := fmt.Sprintf(`Call team_broadcast with my_slug "pm", channel "human__pm", reply_to_id "%s" and reply with the answer. [WUPHF_REPLY_ROUTE channel="human__pm" reply_to_id="%s"]`, replyTo, replyTo)
	if err := l.runHeadlessGeminiTurn(context.Background(), "pm", provider.KindGeminiVertex, notification); err != nil {
		t.Fatalf("runHeadlessGeminiTurn: %v", err)
	}

	var dmReplies []channelMessage
	for _, msg := range l.broker.AllMessages() {
		if msg.From == "pm" && msg.Channel == "human__pm" && msg.ReplyTo == replyTo {
			dmReplies = append(dmReplies, msg)
		}
	}
	if len(dmReplies) != 1 {
		t.Fatalf("expected exactly one DM reply, got %+v", dmReplies)
	}
	if dmReplies[0].Content != "DM fallback reply" {
		t.Fatalf("DM reply content = %q, want %q", dmReplies[0].Content, "DM fallback reply")
	}
}

func TestRunHeadlessGeminiTurnUsesConfiguredVertexModel(t *testing.T) {
	oldGeminiRunner := headlessGeminiVertexOneShot
	defer func() { headlessGeminiVertexOneShot = oldGeminiRunner }()

	var gotModel string
	headlessGeminiVertexOneShot = func(_ context.Context, model, systemPrompt, prompt string) (string, error) {
		gotModel = model
		return "Configured model reply", nil
	}

	l := newHeadlessLauncherForTest()
	l.broker = NewBroker()
	l.broker.mu.Lock()
	l.broker.members = append(l.broker.members, officeMember{
		Slug: "pm",
		Name: "PM",
		Provider: provider.ProviderBinding{
			Kind:  provider.KindGeminiVertex,
			Model: "gemini-2.5-pro",
		},
	})
	l.broker.mu.Unlock()
	ensureTestMemberAccess(l.broker, "general", "pm", "PM")

	notification := fmt.Sprintf(`Call team_broadcast with my_slug "pm", channel "general", reply_to_id "%s" and reply with the answer.`, fmt.Sprintf("msg-%d", time.Now().UnixNano()))
	if err := l.runHeadlessGeminiTurn(context.Background(), "pm", provider.KindGeminiVertex, notification); err != nil {
		t.Fatalf("runHeadlessGeminiTurn: %v", err)
	}
	if gotModel != "gemini-2.5-pro" {
		t.Fatalf("expected configured vertex model, got %q", gotModel)
	}
}
