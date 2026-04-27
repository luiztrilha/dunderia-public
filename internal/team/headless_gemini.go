package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/provider"
)

var (
	headlessGeminiOneShot       = provider.RunGeminiOneShotContext
	headlessGeminiVertexOneShot = provider.RunGeminiVertexOneShotWithModelContext
)

const headlessGeminiEmptyReplyRetryLimit = 1

func (l *Launcher) runHeadlessGeminiTurn(ctx context.Context, slug, runtimeKind, notification string, channel ...string) error {
	notification, changed := normalizeHeadlessPromptPayload(notification)
	if changed {
		appendHeadlessClaudeLog(slug, "turn-input-sanitize: normalized non-UTF8 bytes in turn notification before gemini dispatch")
	}
	if l == nil || l.broker == nil {
		return fmt.Errorf("broker is not running")
	}
	turnChannel := l.headlessTurnChannel(slug, channel...)

	runtimeKind = normalizeProviderKind(runtimeKind)
	var runOneShot func(context.Context, string, string) (string, error)
	switch runtimeKind {
	case provider.KindGemini:
		runOneShot = headlessGeminiOneShot
	case provider.KindGeminiVertex:
		model := provider.GeminiVertexDefaultModel
		if l != nil && l.broker != nil {
			if bindingModel := strings.TrimSpace(l.broker.MemberProviderBinding(slug).Model); bindingModel != "" {
				model = bindingModel
			}
		}
		if task := l.headlessTaskForExecution(slug, turnChannel); task != nil {
			if taskModel := strings.TrimSpace(task.RuntimeModel); taskModel != "" {
				explicitProvider := ""
				if strings.TrimSpace(task.RuntimeProvider) != "" {
					explicitProvider = normalizeProviderKind(task.RuntimeProvider)
				}
				inferredProvider := inferRuntimeProviderFromModel(taskModel)
				if explicitProvider == provider.KindGeminiVertex || (explicitProvider == "" && inferredProvider == provider.KindGemini) {
					model = taskModel
				}
			}
		}
		runOneShot = func(ctx context.Context, systemPrompt, prompt string) (string, error) {
			return headlessGeminiVertexOneShot(ctx, model, systemPrompt, prompt)
		}
	default:
		return fmt.Errorf("unsupported gemini runtime %q", runtimeKind)
	}

	startedAt := time.Now()
	metrics := headlessProgressMetrics{
		TotalMs:      -1,
		FirstEventMs: 0,
		FirstTextMs:  -1,
		FirstToolMs:  -1,
	}
	l.updateHeadlessProgress(slug, "active", "thinking", "reviewing work packet", metrics, turnChannel)

	prompt := notification
	memoryCtx, memoryCancel := context.WithTimeout(ctx, 2*time.Second)
	if brief := fetchScopedMemoryBrief(memoryCtx, slug, notification, l.broker); brief != "" {
		prompt = brief + "\n\n" + notification
	}
	memoryCancel()
	prompt, changed = normalizeHeadlessPromptPayload(prompt)
	if changed {
		appendHeadlessClaudeLog(slug, "prompt-sanitize: normalized non-UTF8 bytes in gemini prompt payload")
	}

	var text string
	var err error
	for attempt := 0; attempt <= headlessGeminiEmptyReplyRetryLimit; attempt++ {
		text, err = runOneShot(ctx, l.buildHeadlessGeminiPrompt(slug), prompt)
		metrics.FirstTextMs = time.Since(startedAt).Milliseconds()
		metrics.TotalMs = metrics.FirstTextMs
		if err != nil {
			detail := truncate(strings.TrimSpace(err.Error()), 180)
			appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=error provider=%s total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
				runtimeKind,
				metrics.TotalMs,
				metrics.FirstEventMs,
				metrics.FirstTextMs,
				metrics.FirstToolMs,
				detail,
			))
			l.updateHeadlessProgress(slug, "error", "error", detail, metrics, turnChannel)
			if fallbackText := strings.TrimSpace(text); fallbackText != "" {
				if shouldPublishHeadlessErrorFallback(fallbackText) {
					l.publishHeadlessFallbackReply(slug, notification, fallbackText, startedAt)
				} else {
					appendHeadlessClaudeLog(slug, "fallback-suppressed: provider failure text was not published")
				}
			}
			return err
		}

		text = strings.TrimSpace(text)
		if text != "" {
			break
		}
		if attempt < headlessGeminiEmptyReplyRetryLimit {
			appendHeadlessClaudeLog(slug, fmt.Sprintf("empty-reply-retry: provider=%s attempt=%d", runtimeKind, attempt+1))
			l.updateHeadlessProgress(slug, "active", "thinking", "empty provider reply; retrying once", metrics, turnChannel)
			continue
		}
		detail := "model returned no plain-text reply"
		appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=error provider=%s total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
			runtimeKind,
			metrics.TotalMs,
			metrics.FirstEventMs,
			metrics.FirstTextMs,
			metrics.FirstToolMs,
			detail,
		))
		l.updateHeadlessProgress(slug, "error", "error", detail, metrics, turnChannel)
		return fmt.Errorf("%s", detail)
	}
	appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=ok provider=%s total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d final_chars=%d",
		runtimeKind,
		metrics.TotalMs,
		metrics.FirstEventMs,
		metrics.FirstTextMs,
		metrics.FirstToolMs,
		len(text),
	))
	summary := strings.TrimSpace(formatHeadlessLatencySummary(metrics))
	if summary == "" {
		summary = "reply ready"
	} else {
		summary = "reply ready · " + summary
	}
	l.updateHeadlessProgress(slug, "idle", "idle", summary, metrics, turnChannel)
	if text != "" {
		appendHeadlessClaudeLog(slug, "result: "+text)
		l.publishHeadlessFallbackReply(slug, notification, text, startedAt)
	}
	return nil
}

func (l *Launcher) buildHeadlessGeminiPrompt(slug string) string {
	base := strings.TrimSpace(l.buildPrompt(slug))
	override := strings.Join([]string{
		"== GEMINI HEADLESS RUNTIME OVERRIDE ==",
		"This runtime has no tool execution.",
		"Ignore any earlier instruction to call tools such as team_broadcast, team_poll, team_task, team_bridge, team_member, team_channel, human_message, or human_interview.",
		"Do not output JSON, XML, markdown code fences, MCP payloads, or pseudo tool calls.",
		"Return only the final reply body as plain text. The office runtime will post it for you.",
		"If context is genuinely missing, say what is missing in one short plain-text sentence instead of inventing a tool call.",
	}, "\n")
	if base == "" {
		return override
	}
	return base + "\n\n" + override
}
