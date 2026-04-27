package team

import (
	"os"
	"strings"
	"testing"
)

func readLoginHTML(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile("login.html")
	if err != nil {
		t.Fatalf("read login.html: %v", err)
	}

	return string(content)
}

func TestLoginHTMLUsesRootFallbackAfterSuccessfulLogin(t *testing.T) {
	html := readLoginHTML(t)

	if !strings.Contains(html, `window.location.assign(payload.redirectTo || "/");`) {
		t.Fatalf("expected successful login fallback redirect to root, got:\n%s", html)
	}
	if strings.Contains(html, `"/dashboard"`) {
		t.Fatalf("expected login page to avoid dashboard-specific fallback copy")
	}
}

func TestLoginHTMLAutofocusesEmailField(t *testing.T) {
	html := readLoginHTML(t)

	if !strings.Contains(html, `id="email"`) {
		t.Fatalf("expected login page to include an email field")
	}
	if !strings.Contains(html, "autofocus") {
		t.Fatalf("expected login page to autofocus the email field")
	}
}

func TestLoginHTMLIncludesAccessibleCredentialControls(t *testing.T) {
	html := readLoginHTML(t)

	requiredSnippets := []string{
		`<form id="login-form" novalidate data-login-endpoint="/api/auth/login">`,
		`type="email"`,
		`autocomplete="email"`,
		`type="password"`,
		`autocomplete="current-password"`,
		`aria-describedby="password-hint password-error"`,
		`id="toggle-password"`,
		`aria-controls="password"`,
		`aria-pressed="false"`,
		`id="remember" name="remember" type="checkbox"`,
		`id="form-status" aria-live="polite"`,
		`id="form-error" role="alert" hidden`,
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(html, snippet) {
			t.Fatalf("expected login page to include %q", snippet)
		}
	}
}

func TestLoginHTMLPostsCredentialsToLoginEndpoint(t *testing.T) {
	html := readLoginHTML(t)

	requiredSnippets := []string{
		`const endpoint = form.dataset.loginEndpoint || "/api/auth/login";`,
		`const response = await fetch(endpoint, {`,
		`method: "POST"`,
		`"Content-Type": "application/json"`,
		`email: emailInput.value.trim()`,
		`password: passwordInput.value`,
		`remember: rememberInput.checked`,
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(html, snippet) {
			t.Fatalf("expected login submission code to include %q", snippet)
		}
	}
}

func TestLoginHTMLValidatesFieldsBeforeSubmitting(t *testing.T) {
	html := readLoginHTML(t)

	requiredSnippets := []string{
		`const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;`,
		`setFieldError(emailInput, emailError, "Informe seu e-mail.");`,
		`setFieldError(emailInput, emailError, "Informe um e-mail valido.");`,
		`setFieldError(passwordInput, passwordError, "Informe sua senha.");`,
		`setFieldError(passwordInput, passwordError, "A senha deve ter pelo menos 8 caracteres.");`,
		`if (!emailValid) {`,
		`emailInput.focus();`,
		`if (!passwordValid) {`,
		`passwordInput.focus();`,
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(html, snippet) {
			t.Fatalf("expected login validation code to include %q", snippet)
		}
	}
}
