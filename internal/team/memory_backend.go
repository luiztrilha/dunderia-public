package team

import (
	"context"
	"fmt"

	"github.com/nex-crm/wuphf/internal/config"
)

type MemoryBackendStatus struct {
	SelectedKind  string
	SelectedLabel string
	ActiveKind    string
	ActiveLabel   string
	Detail        string
	NextStep      string
}

type memoryMCPServer struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
	EnvVars []string
}

type ScopedMemoryHit struct {
	Scope      string
	Backend    string
	Identifier string
	Title      string
	Snippet    string
	OwnerSlug  string
}

type SharedMemoryWrite struct {
	Actor   string
	Key     string
	Title   string
	Content string
}

func ResolveMemoryBackendStatus() MemoryBackendStatus {
	return MemoryBackendStatus{
		SelectedKind:  config.MemoryBackendNone,
		SelectedLabel: config.MemoryBackendLabel(config.MemoryBackendNone),
		ActiveKind:    config.MemoryBackendNone,
		ActiveLabel:   config.MemoryBackendLabel(config.MemoryBackendNone),
		Detail:        "Shared org memory is currently disabled. Durable state lives in local persistence plus configured cloud backup.",
		NextStep:      "Use channel history, scoped local memory, and cloud backup snapshots for recovery.",
	}
}

func shouldPollNexNotifications() bool {
	return false
}

func fetchMemoryBrief(context.Context, string) string {
	return ""
}

func QuerySharedMemory(context.Context, string, int) ([]ScopedMemoryHit, error) {
	return nil, nil
}

func WriteSharedMemory(_ context.Context, note SharedMemoryWrite) (string, error) {
	if note.Content == "" {
		return "", fmt.Errorf("content is required")
	}
	return "", fmt.Errorf("shared external memory is not active for this run")
}

func resolvedMemoryMCPServer() (*memoryMCPServer, error) {
	return nil, nil
}

func directMemoryPromptBlock() string {
	return "Memory scopes:\n- team_memory_query: Your private notes still work with `scope=private`\n- team_memory_write: Store private notes for yourself\n- Shared org memory is not active for this run, so `scope=shared` and team_memory_promote are unavailable\n\n"
}

func directMemoryStorageRule() string {
	return "7. Do not pretend anything was stored outside your private note scope.\n"
}

func leadMemoryPromptBlock() string {
	return "Shared org memory is not active for this run. You can still use private notes with team_memory_query/team_memory_write scope=private.\n\n"
}

func leadMemoryFirstRule() string {
	return "1. Coordinate inside the office channel first, and use private memory only for your own scratch history.\n"
}

func leadMemoryStorageRule() string {
	return "8. Summarize final decisions clearly in-channel; shared org memory is unavailable in this run\n"
}

func leadMemoryFinalWarning() string {
	return "Do not claim you stored anything outside your private notes.\n"
}

func specialistMemoryPromptBlock() string {
	return "Shared org memory is not active for this run. You can still use private notes with team_memory_query/team_memory_write scope=private.\n\n"
}

func specialistMemoryStorageRule() string {
	return "9. Don't fake shared memory. Surface uncertainty in-channel and keep any retained notes private.\n\n"
}
