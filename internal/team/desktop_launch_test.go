package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestDesktopLaunchUsesLocalWebURLAndRecordsAction(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	npmPath := filepath.Join(t.TempDir(), "npm.cmd")
	started := desktopLaunchRecord{}
	restore := stubDesktopLaunchProcess(t, npmPath, &started)
	defer restore()

	b := NewBroker()
	b.webUIOrigins = []string{"http://localhost:7891"}
	body := []byte(`{"web_url":"http://127.0.0.1:7891/apps/studio"}`)
	req := newAuthenticatedDesktopLaunchRequest(t, b, http.MethodPost, "/integrations/desktop/launch", body)
	rec := httptestResponse(req, b.handleDesktopLaunch)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp desktopLaunchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || !resp.Launched {
		t.Fatalf("expected launched response, got %+v", resp)
	}
	if resp.WebURL != "http://127.0.0.1:7891" {
		t.Fatalf("web_url = %q, want local origin", resp.WebURL)
	}
	if started.Command != npmPath {
		t.Fatalf("command = %q, want %q", started.Command, npmPath)
	}
	if got := stringsJoin(started.Args); got != "run start" {
		t.Fatalf("args = %q, want run start", got)
	}
	if envValue(started.Env, "MAESTRIA_WEB_URL") != "http://127.0.0.1:7891" {
		t.Fatalf("missing MAESTRIA_WEB_URL in env: %#v", started.Env)
	}
	actions := b.Actions()
	if len(actions) != 1 || actions[0].Kind != "external_tool_launched" || actions[0].Source != "dunderia-desktop" {
		t.Fatalf("expected desktop launch action, got %+v", actions)
	}
}

func TestDesktopLaunchRejectsRemoteWebURL(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	restore := stubDesktopLaunchProcess(t, filepath.Join(t.TempDir(), "npm.cmd"), nil)
	defer restore()

	b := NewBroker()
	body := []byte(`{"web_url":"https://example.com"}`)
	req := newAuthenticatedDesktopLaunchRequest(t, b, http.MethodPost, "/integrations/desktop/launch", body)
	rec := httptestResponse(req, b.handleDesktopLaunch)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(b.Actions()) != 0 {
		t.Fatalf("rejected launch should not record actions, got %+v", b.Actions())
	}
}

type desktopLaunchRecord struct {
	Command    string
	Args       []string
	WorkingDir string
	Env        []string
}

func stubDesktopLaunchProcess(t *testing.T, npmPath string, started *desktopLaunchRecord) func() {
	t.Helper()
	prevLookPath := desktopLaunchLookPath
	prevStart := desktopLaunchStartProcess
	desktopLaunchLookPath = func(command string) (string, error) {
		return npmPath, nil
	}
	desktopLaunchStartProcess = func(command string, args []string, workingDir string, env []string) error {
		if started != nil {
			*started = desktopLaunchRecord{
				Command:    command,
				Args:       append([]string(nil), args...),
				WorkingDir: workingDir,
				Env:        append([]string(nil), env...),
			}
		}
		return nil
	}
	return func() {
		desktopLaunchLookPath = prevLookPath
		desktopLaunchStartProcess = prevStart
	}
}

func newAuthenticatedDesktopLaunchRequest(t *testing.T, b *Broker, method, path string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	return req
}

func stringsJoin(items []string) string {
	out := ""
	for _, item := range items {
		if out != "" {
			out += " "
		}
		out += item
	}
	return out
}
