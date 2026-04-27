package team

import (
	"testing"

	"github.com/nex-crm/wuphf/internal/config"
)

func TestResolveMemoryBackendStatusIsAlwaysLocalOnly(t *testing.T) {
	t.Setenv("WUPHF_NO_NEX", "")
	t.Setenv("WUPHF_MEMORY_BACKEND", "gbrain")
	t.Setenv("WUPHF_OPENAI_API_KEY", "sk-test-openai")
	t.Setenv("WUPHF_API_KEY", "nex-test-key")

	status := ResolveMemoryBackendStatus()
	if status.SelectedKind != config.MemoryBackendNone {
		t.Fatalf("expected selected backend none, got %+v", status)
	}
	if status.ActiveKind != config.MemoryBackendNone {
		t.Fatalf("expected active backend none, got %+v", status)
	}
	if status.SelectedLabel != "Local-only" || status.ActiveLabel != "Local-only" {
		t.Fatalf("expected local-only labels, got %+v", status)
	}
}

func TestShouldPollNexNotificationsIsAlwaysDisabled(t *testing.T) {
	t.Setenv("WUPHF_MEMORY_BACKEND", "nex")
	t.Setenv("WUPHF_API_KEY", "nex-test-key")
	if shouldPollNexNotifications() {
		t.Fatal("expected nex notification polling to stay disabled")
	}
}
