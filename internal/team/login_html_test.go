package team

import (
	"os"
	"strings"
	"testing"
)

func TestLoginHTMLUsesRootFallbackAfterSuccessfulLogin(t *testing.T) {
	content, err := os.ReadFile("login.html")
	if err != nil {
		t.Fatalf("read login.html: %v", err)
	}

	html := string(content)
	if !strings.Contains(html, `window.location.assign(payload.redirectTo || "/");`) {
		t.Fatalf("expected successful login fallback redirect to root, got:\n%s", html)
	}
	if strings.Contains(html, `"/dashboard"`) {
		t.Fatalf("expected login page to avoid dashboard-specific fallback copy")
	}
}

func TestLoginHTMLAutofocusesEmailField(t *testing.T) {
	content, err := os.ReadFile("login.html")
	if err != nil {
		t.Fatalf("read login.html: %v", err)
	}

	html := string(content)
	if !strings.Contains(html, `id="email"`) {
		t.Fatalf("expected login page to include an email field")
	}
	if !strings.Contains(html, "autofocus") {
		t.Fatalf("expected login page to autofocus the email field")
	}
}
