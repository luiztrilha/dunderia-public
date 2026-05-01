package team

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/agent"
	"github.com/nex-crm/wuphf/internal/config"
	"github.com/nex-crm/wuphf/internal/provider"
)

var headlessOllamaStreamFactory = func(ctx context.Context, baseURL, model string) agent.StreamFn {
	return provider.CreateOllamaStreamFnWithContext(ctx, baseURL, model)
}

var headlessOllamaLocalExec = func(ctx context.Context, workingDirectory, command string, timeout time.Duration) (headlessOllamaLocalExecResult, error) {
	return runHeadlessOllamaLocalExec(ctx, workingDirectory, command, timeout)
}

const (
	headlessOllamaMaxToolRounds           = 4
	headlessOllamaLocalExecDefaultTimeout = 20 * time.Second
	headlessOllamaLocalExecOutputLimit    = 4000
)

type headlessOllamaLocalExecResult struct {
	Command          string `json:"command"`
	WorkingDirectory string `json:"working_directory"`
	ExitCode         int    `json:"exit_code"`
	Stdout           string `json:"stdout,omitempty"`
	Stderr           string `json:"stderr,omitempty"`
}

type headlessOllamaToolOutcome struct {
	Detail       string
	ModelResult  string
	DidBroadcast bool
}

func (l *Launcher) runHeadlessOllamaTurn(ctx context.Context, slug, notification string, channel ...string) error {
	notification, changed := normalizeHeadlessPromptPayload(notification)
	if changed {
		appendHeadlessClaudeLog(slug, "turn-input-sanitize: normalized non-UTF8 bytes in turn notification before ollama dispatch")
	}
	if l == nil || l.broker == nil {
		return fmt.Errorf("broker is not running")
	}
	turnChannel := l.headlessTurnChannel(slug, channel...)
	route := l.resolveHeadlessModelRoute(provider.KindOllama, slug, notification, turnChannel)
	appendHeadlessClaudeLog(slug, "model-routing: "+route.summary())

	startedAt := time.Now()
	metrics := headlessProgressMetrics{
		TotalMs:      -1,
		FirstEventMs: -1,
		FirstTextMs:  -1,
		FirstToolMs:  -1,
	}
	l.updateHeadlessProgress(slug, "active", "thinking", "reviewing work packet · "+route.progressDetail(), metrics, turnChannel)

	prompt := notification
	if workspace := strings.TrimSpace(l.headlessTaskWorkspaceDir(slug, turnChannel)); workspace != "" {
		prompt += "\n\nAssigned working directory: " + workspace
	}
	memoryCtx, memoryCancel := context.WithTimeout(ctx, 2*time.Second)
	if brief := fetchScopedMemoryBrief(memoryCtx, slug, notification, l.broker); brief != "" {
		prompt = brief + "\n\n" + prompt
	}
	memoryCancel()
	prompt, changed = normalizeHeadlessPromptPayload(prompt)
	if changed {
		appendHeadlessClaudeLog(slug, "prompt-sanitize: normalized non-UTF8 bytes in ollama prompt payload")
	}

	fn := headlessOllamaStreamFactory(ctx, config.ResolveOllamaBaseURL(), route.Model)
	messages := []agent.Message{
		{Role: "system", Content: l.buildHeadlessOllamaPrompt(slug)},
		{Role: "user", Content: prompt},
	}
	tools := l.headlessOllamaTools(slug)

	var firstEventAt time.Time
	var firstTextAt time.Time
	var firstToolAt time.Time
	var fullText strings.Builder
	broadcasted := false
	finalized := false
	exhaustedToolRounds := false

	defaultChannel, defaultReplyTo, _ := headlessReplyRoute(notification)
	for round := 0; round < headlessOllamaMaxToolRounds; round++ {
		stream := fn(messages, tools)
		var roundText strings.Builder
		continued := false
		for chunk := range stream {
			if firstEventAt.IsZero() {
				firstEventAt = time.Now()
				metrics.FirstEventMs = durationMillis(startedAt, firstEventAt)
			}
			switch chunk.Type {
			case "text":
				if strings.TrimSpace(chunk.Content) == "" {
					continue
				}
				if firstTextAt.IsZero() {
					firstTextAt = time.Now()
					metrics.FirstTextMs = durationMillis(startedAt, firstTextAt)
				}
				roundText.WriteString(chunk.Content)
				l.updateHeadlessProgress(slug, "active", "text", "drafting response", metrics, turnChannel)
			case "tool_call":
				if firstToolAt.IsZero() {
					firstToolAt = time.Now()
					metrics.FirstToolMs = durationMillis(startedAt, firstToolAt)
				}
				outcome, err := l.executeHeadlessOllamaTool(ctx, slug, turnChannel, chunk.ToolName, chunk.ToolParams, defaultChannel, defaultReplyTo)
				if err != nil {
					metrics.TotalMs = time.Since(startedAt).Milliseconds()
					appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=error provider=%s profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
						provider.KindOllama,
						route.Profile,
						route.Model,
						metrics.TotalMs,
						durationMillis(startedAt, firstEventAt),
						durationMillis(startedAt, firstTextAt),
						durationMillis(startedAt, firstToolAt),
						err.Error(),
					))
					l.updateHeadlessProgress(slug, "error", "error", truncate(err.Error(), 180), metrics, turnChannel)
					return err
				}
				if outcome.DidBroadcast {
					broadcasted = true
				}
				l.updateHeadlessProgress(slug, "active", "tool_use", truncate(outcome.Detail, 180), metrics, turnChannel)
				if strings.TrimSpace(outcome.ModelResult) != "" {
					messages = append(messages,
						agent.Message{Role: "assistant", Content: headlessOllamaToolCallMessage(chunk.ToolName, chunk.ToolParams)},
						agent.Message{Role: "user", Content: headlessOllamaToolResultMessage(outcome.ModelResult)},
					)
					continued = true
				}
			case "error":
				metrics.TotalMs = time.Since(startedAt).Milliseconds()
				appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=error provider=%s profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
					provider.KindOllama,
					route.Profile,
					route.Model,
					metrics.TotalMs,
					durationMillis(startedAt, firstEventAt),
					durationMillis(startedAt, firstTextAt),
					durationMillis(startedAt, firstToolAt),
					chunk.Content,
				))
				l.updateHeadlessProgress(slug, "error", "error", truncate(chunk.Content, 180), metrics, turnChannel)
				return fmt.Errorf("%s", strings.TrimSpace(chunk.Content))
			}
		}
		if continued {
			if round == headlessOllamaMaxToolRounds-1 {
				exhaustedToolRounds = true
			}
			continue
		}
		if text := strings.TrimSpace(roundText.String()); text != "" {
			fullText.WriteString(text)
			finalized = true
			break
		}
		if broadcasted {
			finalized = true
			break
		}
		break
	}
	if !finalized && !broadcasted && exhaustedToolRounds {
		detail := "ollama exceeded tool round limit without a final reply"
		metrics.TotalMs = time.Since(startedAt).Milliseconds()
		appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=error provider=%s profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
			provider.KindOllama,
			route.Profile,
			route.Model,
			metrics.TotalMs,
			durationMillis(startedAt, firstEventAt),
			durationMillis(startedAt, firstTextAt),
			durationMillis(startedAt, firstToolAt),
			detail,
		))
		l.updateHeadlessProgress(slug, "error", "error", detail, metrics, turnChannel)
		return fmt.Errorf("%s", detail)
	}

	text := strings.TrimSpace(fullText.String())
	metrics.TotalMs = time.Since(startedAt).Milliseconds()
	if text == "" && !broadcasted {
		detail := "model returned no plain-text reply"
		appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=error provider=%s profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
			provider.KindOllama,
			route.Profile,
			route.Model,
			metrics.TotalMs,
			durationMillis(startedAt, firstEventAt),
			durationMillis(startedAt, firstTextAt),
			durationMillis(startedAt, firstToolAt),
			detail,
		))
		l.updateHeadlessProgress(slug, "error", "error", detail, metrics, turnChannel)
		return fmt.Errorf("%s", detail)
	}

	appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=ok provider=%s profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d final_chars=%d",
		provider.KindOllama,
		route.Profile,
		route.Model,
		metrics.TotalMs,
		durationMillis(startedAt, firstEventAt),
		durationMillis(startedAt, firstTextAt),
		durationMillis(startedAt, firstToolAt),
		len(text),
	))
	summary := strings.TrimSpace(formatHeadlessLatencySummary(metrics))
	if summary == "" {
		summary = "reply ready"
	} else {
		summary = "reply ready · " + summary
	}
	l.updateHeadlessProgress(slug, "idle", "idle", summary, metrics, turnChannel)
	if text != "" && !broadcasted {
		appendHeadlessClaudeLog(slug, "result: "+text)
		l.publishHeadlessFallbackReply(slug, notification, text, startedAt)
	}
	return nil
}

func (l *Launcher) executeHeadlessOllamaTool(ctx context.Context, slug, turnChannel, toolName string, params map[string]any, defaultChannel, defaultReplyTo string) (headlessOllamaToolOutcome, error) {
	switch strings.TrimSpace(toolName) {
	case "team_broadcast":
		channel := normalizeChannelSlug(stringParam(params, "channel"))
		if channel == "" {
			channel = normalizeChannelSlug(defaultChannel)
		}
		if channel == "" {
			channel = "general"
		}
		replyTo := strings.TrimSpace(stringParam(params, "reply_to_id"))
		if replyTo == "" {
			replyTo = strings.TrimSpace(defaultReplyTo)
		}
		content := strings.TrimSpace(stringParam(params, "content"))
		if content == "" {
			return headlessOllamaToolOutcome{}, fmt.Errorf("team_broadcast requires content")
		}
		tagged := stringSliceParam(params, "tagged")
		if _, err := l.broker.PostMessage(slug, channel, content, tagged, replyTo); err != nil {
			return headlessOllamaToolOutcome{}, err
		}
		return headlessOllamaToolOutcome{
			Detail:       fmt.Sprintf("posted team_broadcast to #%s", channel),
			DidBroadcast: true,
		}, nil
	case "team_task":
		action := strings.TrimSpace(stringParam(params, "action"))
		if action == "" {
			return headlessOllamaToolOutcome{}, fmt.Errorf("team_task requires action")
		}
		task, err := l.broker.ApplyHeadlessTaskMutation(headlessTaskMutationInput{
			Action:        action,
			Channel:       firstNonEmpty(stringParam(params, "channel"), defaultChannel),
			ExecutionKey:  stringParam(params, "execution_key"),
			ID:            stringParam(params, "id"),
			Title:         stringParam(params, "title"),
			Details:       stringParam(params, "details"),
			Owner:         stringParam(params, "owner"),
			CreatedBy:     slug,
			TaskType:      stringParam(params, "task_type"),
			ExecutionMode: stringParam(params, "execution_mode"),
			WorkspacePath: stringParam(params, "workspace_path"),
			DependsOn:     stringSliceParam(params, "depends_on"),
		})
		if err != nil {
			return headlessOllamaToolOutcome{}, err
		}
		return headlessOllamaToolOutcome{
			Detail: fmt.Sprintf("updated task %s [%s]", task.ID, task.Status),
		}, nil
	case "local_exec":
		command := strings.TrimSpace(stringParam(params, "command"))
		if command == "" {
			return headlessOllamaToolOutcome{}, fmt.Errorf("local_exec requires command")
		}
		workingDirectory := l.resolveHeadlessOllamaWorkingDirectory(slug, turnChannel, stringParam(params, "working_directory"))
		result, err := headlessOllamaLocalExec(ctx, workingDirectory, command, durationParamMillis(params, "timeout_ms", headlessOllamaLocalExecDefaultTimeout))
		if err != nil {
			return headlessOllamaToolOutcome{}, err
		}
		raw, err := json.Marshal(map[string]any{
			"tool":   "local_exec",
			"result": result,
		})
		if err != nil {
			return headlessOllamaToolOutcome{}, err
		}
		return headlessOllamaToolOutcome{
			Detail:      fmt.Sprintf("local_exec exit=%d in %s", result.ExitCode, result.WorkingDirectory),
			ModelResult: string(raw),
		}, nil
	default:
		return headlessOllamaToolOutcome{}, fmt.Errorf("unsupported ollama tool %q", toolName)
	}
}

func (l *Launcher) buildHeadlessOllamaPrompt(slug string) string {
	base := strings.TrimSpace(l.buildPrompt(slug))
	override := strings.Join([]string{
		"== OLLAMA HEADLESS RUNTIME OVERRIDE ==",
		"Use JSON tool calls when you need to act in the office.",
		`Valid tool-call forms: {"name":"team_broadcast","arguments":{...}} or {"type":"team_broadcast", ...top-level args...}.`,
		`Use {"name":"team_task","arguments":{...}} when you need to create or mutate durable task state.`,
		`For team_task complete, review, block, or reconcile, details must be a structured handoff with ## Task Report and ## Downstream Context. Use ## Blockers on block and ## Review Findings when reporting review output. When a blocker or finding is a genuinely separate follow-up, mark it with New Demand: yes.`,
		`Use {"name":"local_exec","arguments":{"command":"...","working_directory":"...","timeout_ms":20000}} when you need verified local command or filesystem evidence.`,
		"In task turns, local_exec defaults to the assigned workspace path. In DM and general turns, it defaults to the office working directory.",
		"When you are simply answering the human, plain text is acceptable and the office runtime will post it.",
		"Do not output markdown code fences unless you are returning normal prose and cannot avoid them.",
	}, "\n")
	if base == "" {
		return override
	}
	return base + "\n\n" + override
}

func (l *Launcher) headlessOllamaModel(slug string) string {
	return l.resolveHeadlessModelRoute(provider.KindOllama, slug, "").Model
}

func (l *Launcher) headlessOllamaTools(slug string) []agent.AgentTool {
	return []agent.AgentTool{
		{
			Name:        "team_broadcast",
			Description: "Post a reply to a shared channel or DM thread.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel":     map[string]any{"type": "string"},
					"content":     map[string]any{"type": "string"},
					"reply_to_id": map[string]any{"type": "string"},
					"tagged":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"content"},
			},
		},
		{
			Name:        "team_task",
			Description: "Create or update durable task state for owned work.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":         map[string]any{"type": "string"},
					"channel":        map[string]any{"type": "string"},
					"id":             map[string]any{"type": "string"},
					"title":          map[string]any{"type": "string"},
					"details":        map[string]any{"type": "string", "description": "Structured handoff details for complete, review, block, or reconcile. Use ## Task Report and ## Downstream Context; add ## Blockers on block and ## Review Findings for review output. Mark separate follow-up work with New Demand: yes."},
					"owner":          map[string]any{"type": "string"},
					"task_type":      map[string]any{"type": "string"},
					"execution_mode": map[string]any{"type": "string"},
					"workspace_path": map[string]any{"type": "string"},
					"depends_on":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"action"},
			},
		},
		{
			Name:        "local_exec",
			Description: "Run a local shell command and return stdout, stderr, exit_code, and the working directory used.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":           map[string]any{"type": "string"},
					"working_directory": map[string]any{"type": "string"},
					"timeout_ms":        map[string]any{"type": "integer"},
				},
				"required": []string{"command"},
			},
		},
	}
}

func (l *Launcher) resolveHeadlessOllamaWorkingDirectory(slug string, channel string, requested string) string {
	base := strings.TrimSpace(l.headlessTaskWorkspaceDir(slug, channel))
	if base == "" {
		base = strings.TrimSpace(l.cwd)
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return normalizeHeadlessWorkspaceDir(base)
	}
	if !filepath.IsAbs(requested) && base != "" {
		requested = filepath.Join(base, requested)
	}
	return normalizeHeadlessWorkspaceDir(requested)
}

func runHeadlessOllamaLocalExec(ctx context.Context, workingDirectory, command string, timeout time.Duration) (headlessOllamaLocalExecResult, error) {
	result := headlessOllamaLocalExecResult{
		Command:          strings.TrimSpace(command),
		WorkingDirectory: normalizeHeadlessWorkspaceDir(workingDirectory),
		ExitCode:         -1,
	}
	if result.Command == "" {
		return result, fmt.Errorf("local_exec requires command")
	}
	if result.WorkingDirectory == "" {
		return result, fmt.Errorf("local_exec requires a valid working directory")
	}
	info, err := os.Stat(result.WorkingDirectory)
	if err != nil {
		return result, fmt.Errorf("local_exec working directory unavailable: %w", err)
	}
	if !info.IsDir() {
		return result, fmt.Errorf("local_exec working directory is not a directory")
	}
	if timeout <= 0 {
		timeout = headlessOllamaLocalExecDefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(runCtx, "powershell", "-NoLogo", "-NoProfile", "-Command", result.Command)
	} else {
		cmd = exec.CommandContext(runCtx, "sh", "-lc", result.Command)
	}
	cmd.Dir = result.WorkingDirectory
	configureHeadlessProcess(cmd)
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result.Stdout = truncate(strings.TrimSpace(stdout.String()), headlessOllamaLocalExecOutputLimit)
	result.Stderr = truncate(strings.TrimSpace(stderr.String()), headlessOllamaLocalExecOutputLimit)
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			result.Stderr = strings.TrimSpace(firstNonEmpty(result.Stderr, fmt.Sprintf("command timed out after %s", timeout.Round(time.Second))))
			return result, nil
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, fmt.Errorf("local_exec failed to start: %w", err)
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if result.ExitCode < 0 {
		result.ExitCode = 0
	}
	return result, nil
}

func headlessOllamaToolCallMessage(toolName string, params map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"name":      strings.TrimSpace(toolName),
		"arguments": params,
	})
	if err != nil {
		return strings.TrimSpace(toolName)
	}
	return string(raw)
}

func headlessOllamaToolResultMessage(result string) string {
	return "Tool result JSON:\n" + strings.TrimSpace(result) + "\nUse this result to decide the next step. Reply with plain text if the answer is ready, or call another tool if needed."
}

func stringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	if raw, ok := params[key]; ok {
		switch typed := raw.(type) {
		case string:
			return strings.TrimSpace(typed)
		}
	}
	return ""
}

func stringSliceParam(params map[string]any, key string) []string {
	if params == nil {
		return nil
	}
	raw, ok := params[key]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				out = append(out, strings.TrimSpace(item))
			}
		}
		return out
	default:
		return nil
	}
}

func durationParamMillis(params map[string]any, key string, fallback time.Duration) time.Duration {
	if params == nil {
		return fallback
	}
	raw, ok := params[key]
	if !ok {
		return fallback
	}
	switch typed := raw.(type) {
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case int64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case float64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	}
	return fallback
}
