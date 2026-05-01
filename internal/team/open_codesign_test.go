package team

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCoDesignStatusUnavailableDoesNotMutate(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	t.Setenv("WUPHF_OPEN_CODESIGN_EXE", "")
	restore := stubOpenCoDesignProcess(t, "", nil)
	defer restore()

	b := NewBroker()
	req := newAuthenticatedOpenCoDesignRequest(t, b, http.MethodGet, "/integrations/open-codesign/status", nil)
	rec := httptestResponse(req, b.handleOpenCoDesignStatus)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp openCoDesignStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Available {
		t.Fatalf("expected unavailable status, got %+v", resp)
	}
	if len(b.Actions()) != 0 {
		t.Fatalf("status should not record actions, got %+v", b.Actions())
	}
}

func TestOpenCoDesignLaunchUsesDetectedExecutableAndRecordsAction(t *testing.T) {
	isolateBrokerPersistenceEnv(t)
	t.Setenv("WUPHF_OPEN_CODESIGN_EXE", "")
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "open-codesign.exe")
	started := ""
	restore := stubOpenCoDesignProcess(t, exePath, &started)
	defer restore()

	b := NewBroker()
	body := []byte(`{"prototype_dir":"` + strings.ReplaceAll(filepath.Join(tmpDir, "handoff"), `\`, `\\`) + `"}`)
	req := newAuthenticatedOpenCoDesignRequest(t, b, http.MethodPost, "/integrations/open-codesign/launch", body)
	rec := httptestResponse(req, b.handleOpenCoDesignLaunch)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp openCoDesignLaunchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || !resp.Launched || !resp.Available {
		t.Fatalf("expected launched response, got %+v", resp)
	}
	if started != exePath {
		t.Fatalf("expected fake executable to start, got %q", started)
	}
	actions := b.Actions()
	if len(actions) != 1 || actions[0].Kind != "external_tool_launched" || actions[0].Source != "open-codesign" {
		t.Fatalf("expected Open CoDesign action log, got %+v", actions)
	}
}

func stubOpenCoDesignProcess(t *testing.T, detectedPath string, started *string) func() {
	t.Helper()
	prevLookPath := openCoDesignLookPath
	prevStart := openCoDesignStartProcess
	openCoDesignLookPath = func(command string) (string, error) {
		if detectedPath == "" {
			return "", execErrNotFound(command)
		}
		return detectedPath, nil
	}
	openCoDesignStartProcess = func(executable, workingDir string) error {
		if started != nil {
			*started = executable
		}
		return nil
	}
	return func() {
		openCoDesignLookPath = prevLookPath
		openCoDesignStartProcess = prevStart
	}
}

func newAuthenticatedOpenCoDesignRequest(t *testing.T, b *Broker, method, path string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+b.Token())
	req.Header.Set("Content-Type", "application/json")
	return req
}

func httptestResponse(req *http.Request, handler func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

type execErrNotFound string

func (e execErrNotFound) Error() string {
	return "executable file not found: " + string(e)
}
