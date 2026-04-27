package team

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebLoginFormRendersAccessibleLoginForm(t *testing.T) {
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
		`<form id="login-form" novalidate data-login-endpoint="/api/auth/login">`,
		`<label for="email">E-mail</label>`,
		`id="email"`,
		`type="email"`,
		`autocomplete="email"`,
		`<label for="password">Senha</label>`,
		`id="password"`,
		`autocomplete="current-password"`,
		`id="toggle-password"`,
		`id="remember" name="remember" type="checkbox"`,
		`const response = await fetch(endpoint, {`,
		`window.location.assign(payload.redirectTo || "/");`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected login page to contain %q", want)
		}
	}

	for _, stale := range []string{
		`Broker access key`,
		`/api-token`,
		`wuphf.brokerToken`,
	} {
		if strings.Contains(body, stale) {
			t.Fatalf("expected login page to avoid stale broker-token login copy %q", stale)
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
