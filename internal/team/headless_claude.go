package team

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nex-crm/wuphf/internal/config"
	"github.com/nex-crm/wuphf/internal/provider"
)

var (
	headlessClaudeLookPath       = exec.LookPath
	headlessClaudeCommandContext = exec.CommandContext
	headlessClaudeResolveGitBash = defaultHeadlessClaudeGitBashPath
)

type headlessClaudeRuntime struct {
	Binary               string
	HeadlessProvider     string
	ProviderFlag         string
	IncludeModel         bool
	UsageModelDescriptor string
}

func (l *Launcher) runHeadlessClaudeTurn(ctx context.Context, slug string, notification string, channel ...string) error {
	return l.runHeadlessClaudeCompatibleTurn(ctx, slug, notification, headlessClaudeRuntime{
		Binary:               "claude",
		HeadlessProvider:     "claude",
		IncludeModel:         true,
		UsageModelDescriptor: l.headlessClaudeModel(slug),
	}, channel...)
}

func (l *Launcher) runHeadlessOpenClaudeTurn(ctx context.Context, slug, runtimeKind, notification string, channel ...string) error {
	runtimeKind = normalizeProviderKind(runtimeKind)
	runtime := headlessClaudeRuntime{
		Binary:               "openclaude",
		HeadlessProvider:     runtimeKind,
		UsageModelDescriptor: runtimeKind,
	}
	switch runtimeKind {
	case provider.KindOpenclaude:
		runtime.ProviderFlag = "vertex"
	case provider.KindGemini:
		runtime.ProviderFlag = "gemini"
	default:
		return fmt.Errorf("unsupported openclaude runtime %q", runtimeKind)
	}
	return l.runHeadlessClaudeCompatibleTurn(ctx, slug, notification, runtime, channel...)
}

func (l *Launcher) runHeadlessClaudeCompatibleTurn(ctx context.Context, slug string, notification string, runtime headlessClaudeRuntime, channel ...string) error {
	notification, changed := normalizeHeadlessPromptPayload(notification)
	if changed {
		appendHeadlessClaudeLog(slug, "turn-input-sanitize: normalized non-UTF8 bytes in turn notification before claude stdin dispatch")
	}
	if l == nil || l.broker == nil {
		return fmt.Errorf("broker is not running")
	}

	turnChannel := l.headlessTurnChannel(slug, channel...)
	workspaceDir := normalizeHeadlessWorkspaceDir(l.headlessWorkspaceDirForExecution(slug, turnChannel))
	if workspaceDir == "" {
		workspaceDir = normalizeHeadlessWorkspaceDir(strings.TrimSpace(l.cwd))
	}
	route := l.resolveHeadlessModelRoute(runtime.HeadlessProvider, slug, notification, turnChannel)
	appendHeadlessClaudeLog(slug, "model-routing: "+route.summary())

	agentMCP := l.mcpConfig
	if path, err := l.ensureAgentMCPConfigForContext(slug, turnChannel, workspaceDir); err == nil {
		agentMCP = path
	}

	args := make([]string, 0, 18)
	if strings.TrimSpace(runtime.ProviderFlag) != "" {
		args = append(args, "--provider", runtime.ProviderFlag)
	}
	if runtime.IncludeModel {
		args = append(args, "--model", route.Model)
	}
	args = append(args,
		"--print", "-",
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", l.headlessClaudeMaxTurns(slug),
		"--disable-slash-commands",
		"--setting-sources", "user",
		"--append-system-prompt", l.buildPrompt(slug),
		"--mcp-config", agentMCP,
		"--strict-mcp-config",
	)
	args = append(args, strings.Fields(l.resolvePermissionFlags(slug))...)

	commandName := runtime.Binary
	commandArgs := append([]string(nil), args...)
	if runtime.Binary == "openclaude" {
		var err error
		commandName, commandArgs, err = resolveHeadlessOpenClaudeCommand(args)
		if err != nil {
			return err
		}
	} else if _, err := headlessClaudeLookPath(runtime.Binary); err != nil {
		return fmt.Errorf("%s not found: %w", runtime.Binary, err)
	}

	cmd := headlessClaudeCommandContext(ctx, commandName, commandArgs...)
	cmd.Dir = firstNonEmpty(workspaceDir, strings.TrimSpace(l.cwd))
	configureHeadlessProcess(cmd)
	env := l.buildHeadlessClaudeEnvForRuntime(slug, runtime.HeadlessProvider, workspaceDir, turnChannel)
	if task := l.headlessTaskForExecution(slug, turnChannel); task != nil && workspaceDir != normalizeHeadlessWorkspaceDir(strings.TrimSpace(l.cwd)) {
		switch strings.ToLower(strings.TrimSpace(task.ExecutionMode)) {
		case "local_worktree":
			env = append(env, "WUPHF_WORKTREE_PATH="+workspaceDir)
		case "external_workspace":
			env = append(env, "WUPHF_WORKSPACE_PATH="+workspaceDir)
		}
	} else if workspaceDir != "" && workspaceDir != normalizeHeadlessWorkspaceDir(strings.TrimSpace(l.cwd)) {
		env = append(env, "WUPHF_WORKSPACE_PATH="+workspaceDir)
	}
	cmd.Env = env

	stdinPayload := notification
	memoryCtx, memoryCancel := context.WithTimeout(ctx, 2*time.Second)
	if brief := fetchScopedMemoryBrief(memoryCtx, slug, notification, l.broker); brief != "" {
		stdinPayload = brief + "\n\n" + notification
	}
	memoryCancel()
	stdinPayload, changed = normalizeHeadlessPromptPayload(stdinPayload)
	if changed {
		appendHeadlessClaudeLog(slug, "stdin-sanitize: normalized non-UTF8 bytes in claude stdin payload")
	}
	cmd.Stdin = strings.NewReader(stdinPayload)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("attach %s stdout: %w", runtime.Binary, err)
	}

	var agentStream *agentStreamBuffer
	if l.broker != nil {
		agentStream = l.broker.AgentStream(slug, turnChannel)
	}
	pr, pw := io.Pipe()
	teedStdout := io.TeeReader(stdout, pw)
	go func() {
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if agentStream != nil && line != "" {
				agentStream.Push(line)
			}
		}
	}()

	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			terminateHeadlessProcess(cmd)
			_ = stdout.Close()
			_ = pw.CloseWithError(ctx.Err())
		case <-done:
		}
	}()

	startedAt := time.Now()
	metrics := headlessProgressMetrics{
		TotalMs:      -1,
		FirstEventMs: -1,
		FirstTextMs:  -1,
		FirstToolMs:  -1,
	}
	l.updateHeadlessProgress(slug, "active", "thinking", "reviewing work packet · "+route.progressDetail(), metrics, turnChannel)

	var firstEventAt time.Time
	var firstTextAt time.Time
	var firstToolAt time.Time
	textStarted := false

	result, parseErr := provider.ReadClaudeJSONStream(teedStdout, func(event provider.ClaudeStreamEvent) {
		if firstEventAt.IsZero() {
			firstEventAt = time.Now()
			metrics.FirstEventMs = durationMillis(startedAt, firstEventAt)
		}
		switch event.Type {
		case "thinking":
			l.updateHeadlessProgress(slug, "active", "thinking", "planning next step", metrics, turnChannel)
		case "text":
			if firstTextAt.IsZero() && strings.TrimSpace(event.Text) != "" {
				firstTextAt = time.Now()
				metrics.FirstTextMs = durationMillis(startedAt, firstTextAt)
			}
			if !textStarted && strings.TrimSpace(event.Text) != "" {
				textStarted = true
				l.updateHeadlessProgress(slug, "active", "text", "drafting response", metrics, turnChannel)
			}
		case "tool_use":
			if firstToolAt.IsZero() {
				firstToolAt = time.Now()
				metrics.FirstToolMs = durationMillis(startedAt, firstToolAt)
			}
			appendHeadlessClaudeLog(slug, fmt.Sprintf("tool_use: %s %s", event.ToolName, truncate(event.ToolInput, 120)))
			l.updateHeadlessProgress(slug, "active", "tool_use", fmt.Sprintf("running %s", strings.TrimSpace(event.ToolName)), metrics, turnChannel)
		case "tool_result":
			appendHeadlessClaudeLog(slug, "tool_result: "+truncate(event.Text, 140))
			l.updateHeadlessProgress(slug, "active", "tool_result", truncate(event.Text, 140), metrics, turnChannel)
		case "error":
			appendHeadlessClaudeLog(slug, "stream_error: "+event.Detail)
			l.updateHeadlessProgress(slug, "error", "error", truncate(event.Detail, 180), metrics, turnChannel)
		}
	})
	_ = pw.Close()
	if err := cmd.Wait(); err != nil {
		detail := strings.TrimSpace(firstNonEmpty(result.LastError, strings.TrimSpace(stderr.String()), err.Error()))
		if fallbackText := strings.TrimSpace(firstNonEmpty(result.FinalMessage, result.LastError)); fallbackText != "" {
			if shouldPublishHeadlessErrorFallback(fallbackText) {
				l.publishHeadlessFallbackReply(slug, notification, fallbackText, startedAt)
			} else {
				appendHeadlessClaudeLog(slug, "fallback-suppressed: provider failure text was not published")
			}
		}
		metrics.TotalMs = time.Since(startedAt).Milliseconds()
		appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=error provider=%s profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
			runtime.HeadlessProvider,
			route.Profile,
			route.Model,
			metrics.TotalMs,
			durationMillis(startedAt, firstEventAt),
			durationMillis(startedAt, firstTextAt),
			durationMillis(startedAt, firstToolAt),
			detail,
		))
		l.updateHeadlessProgress(slug, "error", "error", truncate(detail, 180), metrics, turnChannel)
		return fmt.Errorf("%w: %s", err, detail)
	}
	if parseErr != nil {
		metrics.TotalMs = time.Since(startedAt).Milliseconds()
		appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=error provider=%s profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d detail=%q",
			runtime.HeadlessProvider,
			route.Profile,
			route.Model,
			metrics.TotalMs,
			durationMillis(startedAt, firstEventAt),
			durationMillis(startedAt, firstTextAt),
			durationMillis(startedAt, firstToolAt),
			parseErr.Error(),
		))
		l.updateHeadlessProgress(slug, "error", "error", truncate(parseErr.Error(), 180), metrics, turnChannel)
		return parseErr
	}

	metrics.TotalMs = time.Since(startedAt).Milliseconds()
	appendHeadlessClaudeLatency(slug, fmt.Sprintf("status=ok provider=%s profile=%s model=%q total_ms=%d first_event_ms=%d first_text_ms=%d first_tool_ms=%d final_chars=%d",
		runtime.HeadlessProvider,
		route.Profile,
		route.Model,
		metrics.TotalMs,
		durationMillis(startedAt, firstEventAt),
		durationMillis(startedAt, firstTextAt),
		durationMillis(startedAt, firstToolAt),
		len(strings.TrimSpace(result.FinalMessage)),
	))
	summary := strings.TrimSpace(formatHeadlessLatencySummary(metrics))
	if summary == "" {
		summary = "reply ready"
	} else {
		summary = "reply ready · " + summary
	}
	l.updateHeadlessProgress(slug, "idle", "idle", summary, metrics, turnChannel)
	if l.broker != nil {
		l.broker.RecordAgentUsage(slug, firstNonEmpty(route.Model, runtime.usageModel(l, slug)), result.Usage)
	}
	if text := strings.TrimSpace(result.FinalMessage); text != "" {
		appendHeadlessClaudeLog(slug, "result: "+text)
		l.publishHeadlessFallbackReply(slug, notification, text, startedAt)
	}
	return nil
}

func resolveHeadlessOpenClaudeCommand(args []string) (string, []string, error) {
	if runtime.GOOS != "windows" {
		return "openclaude", args, nil
	}
	bin, err := headlessClaudeLookPath("openclaude")
	if err != nil {
		return "", nil, fmt.Errorf("openclaude not found: %w", err)
	}
	cmdShim := strings.TrimSuffix(bin, filepath.Ext(bin)) + ".cmd"
	if _, err := os.Stat(cmdShim); err == nil {
		return "cmd", append([]string{"/c", cmdShim}, args...), nil
	}
	ps1 := strings.TrimSuffix(bin, filepath.Ext(bin)) + ".ps1"
	if _, err := os.Stat(ps1); err == nil {
		return "pwsh", append([]string{"-File", ps1}, args...), nil
	}
	return "cmd", append([]string{"/c", bin}, args...), nil
}

func (r headlessClaudeRuntime) usageModel(l *Launcher, slug string) string {
	if strings.TrimSpace(r.UsageModelDescriptor) != "" {
		return r.UsageModelDescriptor
	}
	if l != nil {
		return l.headlessClaudeModel(slug)
	}
	return ""
}

func (l *Launcher) headlessClaudeModel(slug string, channel ...string) string {
	return l.resolveHeadlessModelRoute(provider.KindClaudeCode, slug, "", channel...).Model
}

// headlessClaudeMaxTurns returns the turn budget for an agent. The CEO routes
// untagged and DM messages, which typically requires looking up tasks, channel
// members, and posting an assignment. Specialists also need enough slack for
// tool calls and follow-up turns when working a single task.
func (l *Launcher) headlessClaudeMaxTurns(slug string) string {
	if slug == l.officeLeadSlug() {
		return "12"
	}
	return "12"
}

func (l *Launcher) buildHeadlessClaudeEnv(slug string) []string {
	return l.buildHeadlessClaudeEnvForRuntime(slug, "claude", "", "")
}

func (l *Launcher) buildHeadlessClaudeEnvForRuntime(slug string, runtimeProvider string, workspaceDir string, channel string) []string {
	env := os.Environ()
	runtimeProvider = strings.TrimSpace(runtimeProvider)
	if runtimeProvider == "" {
		runtimeProvider = "claude"
	}
	if workspaceDir = normalizeHeadlessWorkspaceDir(workspaceDir); workspaceDir != "" {
		env = setEnvValue(env, "PWD", workspaceDir)
	}
	env = append(env,
		"WUPHF_AGENT_SLUG="+slug,
		"WUPHF_BROKER_TOKEN="+l.broker.Token(),
		"WUPHF_BROKER_BASE_URL="+l.BrokerBaseURL(),
		"WUPHF_HEADLESS_PROVIDER="+runtimeProvider,
		"WUPHF_MEMORY_BACKEND="+config.ResolveMemoryBackend(""),
		fmt.Sprintf("WUPHF_NO_NEX=%t", config.ResolveNoNex()),
		"ANTHROPIC_PROMPT_CACHING=1",
	)
	if channel = strings.TrimSpace(channel); channel != "" {
		env = append(env, "WUPHF_CHANNEL="+channel)
	}
	if runtimeProvider == provider.KindGemini {
		env = append(env, "GEMINI_MODEL="+provider.GeminiDefaultModel)
	}
	if l.isOneOnOne() {
		env = append(env,
			"WUPHF_ONE_ON_ONE=1",
			"WUPHF_ONE_ON_ONE_AGENT="+l.oneOnOneAgent(),
		)
	}
	if secret := strings.TrimSpace(config.ResolveOneSecret()); secret != "" {
		env = append(env, "ONE_SECRET="+secret)
	}
	if identity := strings.TrimSpace(config.ResolveOneIdentity()); identity != "" {
		env = append(env, "ONE_IDENTITY="+identity)
		if identityType := strings.TrimSpace(config.ResolveOneIdentityType()); identityType != "" {
			env = append(env, "ONE_IDENTITY_TYPE="+identityType)
		}
	}
	if apiKey := strings.TrimSpace(config.ResolveAPIKey("")); apiKey != "" {
		env = append(env,
			"WUPHF_API_KEY="+apiKey,
			"NEX_API_KEY="+apiKey,
		)
	}
	if headlessClaudeNeedsGitBash(runtimeProvider) {
		if current := strings.TrimSpace(os.Getenv("CLAUDE_CODE_GIT_BASH_PATH")); current == "" {
			if gitBashPath := strings.TrimSpace(headlessClaudeResolveGitBash()); gitBashPath != "" {
				env = setEnvValue(env, "CLAUDE_CODE_GIT_BASH_PATH", gitBashPath)
			}
		}
	}
	return env
}

func headlessClaudeNeedsGitBash(runtimeProvider string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	switch normalizeProviderKind(runtimeProvider) {
	case "", "claude", provider.KindClaudeCode:
		return true
	default:
		return false
	}
}

func defaultHeadlessClaudeGitBashPath() string {
	for _, candidate := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\Git\usr\bin\bash.exe`,
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func appendHeadlessClaudeLog(slug string, line string) {
	dir := wuphfLogDir()
	if dir == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "headless-claude-"+slug+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "[%s] %s\n", time.Now().Format(time.RFC3339), strings.TrimSpace(line))
}

func appendHeadlessClaudeLatency(slug string, line string) {
	dir := wuphfLogDir()
	if dir == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "headless-claude-latency.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "[%s] agent=%s %s\n", time.Now().Format(time.RFC3339), strings.TrimSpace(slug), strings.TrimSpace(line))
}
