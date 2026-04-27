package team

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func signedGitHubWebhookRequest(event, secret, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	req.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%x", mac.Sum(nil)))
	return req
}

func TestHandleGitHubWebhookRejectsWhenSecretIsMissing(t *testing.T) {
	t.Setenv("WUPHF_GITHUB_WEBHOOK_SECRET", "")

	b := NewBroker()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")

	b.handleGitHubWebhook(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected webhook disabled status, got %d", recorder.Code)
	}
}

func TestHandleGitHubWebhookQueuesSignedReconcile(t *testing.T) {
	t.Setenv("WUPHF_GITHUB_WEBHOOK_SECRET", "super-secret")

	oldAsync := scheduleTaskGitHubAsync
	oldReconcile := runTaskGitHubWebhookReconcile
	oldRefresh := runTaskGitHubWebhookRefreshPublication
	defer func() {
		scheduleTaskGitHubAsync = oldAsync
		runTaskGitHubWebhookReconcile = oldReconcile
		runTaskGitHubWebhookRefreshPublication = oldRefresh
	}()

	scheduleTaskGitHubAsync = func(fn func()) { fn() }
	var reconcileCalls int32
	runTaskGitHubWebhookReconcile = func(b *Broker) {
		atomic.AddInt32(&reconcileCalls, 1)
	}
	runTaskGitHubWebhookRefreshPublication = func(b *Broker, taskID, kind string) {
		t.Fatalf("expected no targeted refresh, got %s/%s", taskID, kind)
	}

	b := NewBroker()
	recorder := httptest.NewRecorder()
	req := signedGitHubWebhookRequest("pull_request", "super-secret", `{"repository":{"full_name":"acme/dunderia"}}`)

	b.handleGitHubWebhook(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted webhook, got %d (%s)", recorder.Code, recorder.Body.String())
	}
	if atomic.LoadInt32(&reconcileCalls) != 1 {
		t.Fatalf("expected webhook to queue one reconcile, got %d", reconcileCalls)
	}
}

func TestHandleGitHubWebhookQueuesTargetedIssueRefreshForTrackedIssue(t *testing.T) {
	t.Setenv("WUPHF_GITHUB_WEBHOOK_SECRET", "super-secret")

	oldAsync := scheduleTaskGitHubAsync
	oldReconcile := runTaskGitHubWebhookReconcile
	oldRefresh := runTaskGitHubWebhookRefreshPublication
	defer func() {
		scheduleTaskGitHubAsync = oldAsync
		runTaskGitHubWebhookReconcile = oldReconcile
		runTaskGitHubWebhookRefreshPublication = oldRefresh
	}()

	scheduleTaskGitHubAsync = func(fn func()) { fn() }
	var reconcileCalls int32
	runTaskGitHubWebhookReconcile = func(b *Broker) {
		atomic.AddInt32(&reconcileCalls, 1)
	}
	type refreshCall struct {
		taskID string
		kind   string
	}
	refreshCalls := make(chan refreshCall, 1)
	runTaskGitHubWebhookRefreshPublication = func(b *Broker, taskID, kind string) {
		refreshCalls <- refreshCall{taskID: taskID, kind: kind}
	}

	b := NewBroker()
	b.tasks = []teamTask{{
		ID:      "task-1",
		Channel: "general",
		Title:   "Tracked issue",
		IssuePublication: &taskGitHubPublication{
			Status: "opened",
			Number: 42,
			URL:    "https://github.com/acme/dunderia/issues/42",
		},
		RepoContext: &taskGitHubRepoContext{Owner: "acme", Repo: "dunderia"},
	}}
	recorder := httptest.NewRecorder()
	req := signedGitHubWebhookRequest("issues", "super-secret", `{"action":"edited","repository":{"full_name":"acme/dunderia"},"issue":{"number":42,"html_url":"https://github.com/acme/dunderia/issues/42"}}`)

	b.handleGitHubWebhook(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted webhook, got %d (%s)", recorder.Code, recorder.Body.String())
	}
	got := waitForDispatchCall(t, refreshCalls)
	if got.taskID != "task-1" || got.kind != "issue" {
		t.Fatalf("expected targeted issue refresh, got %+v", got)
	}
	if atomic.LoadInt32(&reconcileCalls) != 0 {
		t.Fatalf("expected targeted issue refresh to skip global reconcile, got %d", reconcileCalls)
	}
}

func TestHandleGitHubWebhookQueuesTargetedPRRefreshForReviewEvent(t *testing.T) {
	t.Setenv("WUPHF_GITHUB_WEBHOOK_SECRET", "super-secret")

	oldAsync := scheduleTaskGitHubAsync
	oldReconcile := runTaskGitHubWebhookReconcile
	oldRefresh := runTaskGitHubWebhookRefreshPublication
	defer func() {
		scheduleTaskGitHubAsync = oldAsync
		runTaskGitHubWebhookReconcile = oldReconcile
		runTaskGitHubWebhookRefreshPublication = oldRefresh
	}()

	scheduleTaskGitHubAsync = func(fn func()) { fn() }
	var reconcileCalls int32
	runTaskGitHubWebhookReconcile = func(b *Broker) {
		atomic.AddInt32(&reconcileCalls, 1)
	}
	type refreshCall struct {
		taskID string
		kind   string
	}
	refreshCalls := make(chan refreshCall, 1)
	runTaskGitHubWebhookRefreshPublication = func(b *Broker, taskID, kind string) {
		refreshCalls <- refreshCall{taskID: taskID, kind: kind}
	}

	b := NewBroker()
	b.tasks = []teamTask{{
		ID:      "task-7",
		Channel: "general",
		Title:   "Tracked pr",
		PRPublication: &taskGitHubPublication{
			Status: "opened",
			Number: 7,
			URL:    "https://github.com/acme/dunderia/pull/7",
		},
		RepoContext: &taskGitHubRepoContext{Owner: "acme", Repo: "dunderia"},
	}}
	recorder := httptest.NewRecorder()
	req := signedGitHubWebhookRequest("pull_request_review", "super-secret", `{"action":"submitted","repository":{"full_name":"acme/dunderia"},"pull_request":{"number":7,"html_url":"https://github.com/acme/dunderia/pull/7"}}`)

	b.handleGitHubWebhook(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted webhook, got %d (%s)", recorder.Code, recorder.Body.String())
	}
	got := waitForDispatchCall(t, refreshCalls)
	if got.taskID != "task-7" || got.kind != "pr" {
		t.Fatalf("expected targeted pr refresh, got %+v", got)
	}
	if atomic.LoadInt32(&reconcileCalls) != 0 {
		t.Fatalf("expected targeted pr refresh to skip global reconcile, got %d", reconcileCalls)
	}
}
