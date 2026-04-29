package team

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebLoginFormRendersAccessibleLocalLogin(t *testing.T) {
	b := NewBroker()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	b.handleWebLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("expected html content type, got %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("expected no-store cache header, got %q", got)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<form id="login-form"`,
		`<label for="access-key">Broker access key</label>`,
		`id="access-key"`,
		`required`,
		`/api-token`,
		`wuphf.brokerToken`,
		`Enter office`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected login page to contain %q", want)
		}
	}
}

func TestWebLoginFormRejectsUnsupportedMethods(t *testing.T) {
	b := NewBroker()
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	rec := httptest.NewRecorder()

	b.handleWebLogin(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("expected GET, HEAD allow header, got %q", got)
	}
}
