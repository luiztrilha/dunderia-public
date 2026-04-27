package team

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebUIProxyHandlerForwardsOnboardingRoutes(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotQuery string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	b := NewBroker()
	req := httptest.NewRequest(http.MethodGet, "/onboarding/state?step=providers", nil)
	rec := httptest.NewRecorder()

	b.webUIProxyHandler(upstream.URL, "").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/onboarding/state" {
		t.Fatalf("expected proxied onboarding path, got %q", gotPath)
	}
	if gotQuery != "step=providers" {
		t.Fatalf("expected query to be forwarded, got %q", gotQuery)
	}
	if gotAuth != "Bearer "+b.Token() {
		t.Fatalf("expected broker auth header, got %q", gotAuth)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"ok":true}` {
		t.Fatalf("unexpected proxied body %q", body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("expected onboarding proxy response to disable cache, got %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("expected onboarding proxy response pragma no-cache, got %q", got)
	}
	if got := rec.Header().Get("Expires"); got != "0" {
		t.Fatalf("expected onboarding proxy response expires 0, got %q", got)
	}
}

func TestWebUIProxyHandlerForwardsOnboardingMutationAuth(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotQuery string
	var gotMethod string
	var gotBody string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	b := NewBroker()
	req := httptest.NewRequest(http.MethodPost, "/onboarding/progress?step=identity", strings.NewReader(`{"step":"identity","answers":{"company_name":"Initech"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	b.webUIProxyHandler(upstream.URL, "").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/onboarding/progress" {
		t.Fatalf("expected proxied onboarding path, got %q", gotPath)
	}
	if gotQuery != "step=identity" {
		t.Fatalf("expected query to be forwarded, got %q", gotQuery)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected proxied method POST, got %q", gotMethod)
	}
	if gotAuth != "Bearer "+b.Token() {
		t.Fatalf("expected broker auth header, got %q", gotAuth)
	}
	if gotBody != `{"step":"identity","answers":{"company_name":"Initech"}}` {
		t.Fatalf("expected proxied body to be forwarded, got %q", gotBody)
	}
}

func TestWebUIProxyHandlerDisablesCacheForAPIResponses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"channels":[]}`)
	}))
	defer upstream.Close()

	b := NewBroker()
	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	rec := httptest.NewRecorder()

	b.webUIProxyHandler(upstream.URL, "/api").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("expected api proxy response to disable cache, got %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("expected api proxy response pragma no-cache, got %q", got)
	}
	if got := rec.Header().Get("Expires"); got != "0" {
		t.Fatalf("expected api proxy response expires 0, got %q", got)
	}
}
