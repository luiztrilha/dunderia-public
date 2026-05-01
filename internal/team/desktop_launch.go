package team

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type desktopLaunchRequest struct {
	WebURL string `json:"web_url,omitempty"`
}

type desktopLaunchResponse struct {
	OK         bool   `json:"ok"`
	Launched   bool   `json:"launched"`
	WebURL     string `json:"web_url"`
	DesktopDir string `json:"desktop_dir,omitempty"`
	Message    string `json:"message"`
}

var desktopLaunchLookPath = exec.LookPath

var desktopLaunchStartProcess = func(command string, args []string, workingDir string, env []string) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = workingDir
	cmd.Env = env
	configureHeadlessProcess(cmd)
	return cmd.Start()
}

func (b *Broker) handleDesktopLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body desktopLaunchRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	webURL := b.resolveDesktopLaunchWebURL(r, body.WebURL)
	if webURL == "" {
		http.Error(w, "desktop launch requires a local web UI URL", http.StatusBadRequest)
		return
	}
	desktopDir := desktopRuntimeDir()
	if _, err := os.Stat(filepath.Join(desktopDir, "package.json")); err != nil {
		http.Error(w, fmt.Sprintf("desktop shell is not available: %v", err), http.StatusServiceUnavailable)
		return
	}
	npm, err := resolveDesktopNPM()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	env := os.Environ()
	env = setEnvValue(env, "MAESTRIA_WEB_URL", webURL)
	env = setEnvValue(env, "DUNDERIA_WEB_URL", webURL)
	env = setEnvValue(env, "MAESTRIA_DESKTOP_NO_BROKER", "1")
	env = setEnvValue(env, "DUNDERIA_DESKTOP_NO_BROKER", "1")
	if err := desktopLaunchStartProcess(npm, []string{"run", "start"}, desktopDir, env); err != nil {
		http.Error(w, fmt.Sprintf("failed to launch desktop mode: %v", err), http.StatusInternalServerError)
		return
	}

	_ = b.RecordAction("external_tool_launched", "dunderia-desktop", "studio", "human", "Opened MaestrIA desktop mode from the web UI.", "", nil, "")
	writeDesktopLaunchJSON(w, http.StatusOK, desktopLaunchResponse{
		OK:         true,
		Launched:   true,
		WebURL:     webURL,
		DesktopDir: desktopDir,
		Message:    "Desktop mode launched.",
	})
}

func writeDesktopLaunchJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

func (b *Broker) resolveDesktopLaunchWebURL(r *http.Request, requested string) string {
	for _, candidate := range []string{
		requested,
		requestOrigin(r.Header.Get("Origin")),
		requestOrigin(r.Header.Get("Referer")),
		firstNonEmpty(b.webUIOrigins...),
	} {
		if normalized := normalizeDesktopWebURL(candidate); normalized != "" {
			return normalized
		}
	}
	return ""
}

func requestOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func normalizeDesktopWebURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func desktopRuntimeDir() string {
	root := repoRootForRuntimeDefaults()
	for {
		candidate := filepath.Join(root, "desktop")
		if _, err := os.Stat(filepath.Join(candidate, "package.json")); err == nil {
			return candidate
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	return filepath.Join(repoRootForRuntimeDefaults(), "desktop")
}

func resolveDesktopNPM() (string, error) {
	candidates := []string{"npm"}
	if runtime.GOOS == "windows" {
		candidates = []string{"npm.cmd", "npm.exe", "npm"}
	}
	for _, candidate := range candidates {
		if path, err := desktopLaunchLookPath(candidate); err == nil && strings.TrimSpace(path) != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("npm was not found; install Node.js dependencies before opening desktop mode")
}
