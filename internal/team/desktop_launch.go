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
	"time"
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

type desktopIDEPreviewResponse struct {
	GeneratedAt string                     `json:"generated_at"`
	Persisted   bool                       `json:"persisted"`
	Status      string                     `json:"status"`
	Summary     map[string]int             `json:"summary"`
	Surfaces    []desktopIDEPreviewSurface `json:"surfaces"`
}

type desktopIDEPreviewSurface struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	Readiness        string   `json:"readiness"`
	RequiredChecks   []string `json:"required_checks,omitempty"`
	MissingChecks    []string `json:"missing_checks,omitempty"`
	RiskSignals      []string `json:"risk_signals,omitempty"`
	LaunchEndpoint   string   `json:"launch_endpoint,omitempty"`
	CanonicalSurface string   `json:"canonical_surface,omitempty"`
	NextStep         string   `json:"next_step,omitempty"`
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

func (b *Broker) handleDesktopIDEPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	desktopDir := desktopRuntimeDir()
	npm, npmErr := resolveDesktopNPM()
	payload := buildDesktopIDEPreview(desktopDir, npm, npmErr)
	writeDesktopLaunchJSON(w, http.StatusOK, payload)
}

func buildDesktopIDEPreview(desktopDir, npmPath string, npmErr error) desktopIDEPreviewResponse {
	packageOK := false
	if _, err := os.Stat(filepath.Join(desktopDir, "package.json")); err == nil {
		packageOK = true
	}
	npmOK := npmErr == nil && strings.TrimSpace(npmPath) != ""
	desktop := desktopIDEPreviewSurface{
		ID:               "desktop-shell",
		Name:             "Desktop shell",
		Kind:             "electron_shell",
		Readiness:        desktopPreviewReadiness(packageOK && npmOK),
		RequiredChecks:   []string{"desktop_package", "npm_binary", "reuse_existing_broker", "no_topology_mutation"},
		LaunchEndpoint:   "/integrations/desktop/launch",
		CanonicalSurface: "web_studio",
		NextStep:         "Use the desktop shell as an optional wrapper around the existing web Studio; do not replace the broker/runtime.",
	}
	if !packageOK {
		desktop.MissingChecks = append(desktop.MissingChecks, "desktop_package")
	}
	if !npmOK {
		desktop.MissingChecks = append(desktop.MissingChecks, "npm_binary")
	}
	desktop.RiskSignals = desktopPreviewRiskSignals(desktop)

	tray := desktopIDEPreviewSurface{
		ID:               "desktop-tray",
		Name:             "Tray shell",
		Kind:             "desktop_tray",
		Readiness:        desktopPreviewReadiness(packageOK),
		RequiredChecks:   []string{"desktop_package", "reuse_existing_broker", "no_topology_mutation"},
		LaunchEndpoint:   "/integrations/desktop/launch",
		CanonicalSurface: "web_studio",
		NextStep:         "Keep tray actions limited to opening Studio, reload, and diagnostics; no agent/channel/topology mutation.",
	}
	if !packageOK {
		tray.MissingChecks = append(tray.MissingChecks, "desktop_package")
	}
	tray.RiskSignals = desktopPreviewRiskSignals(tray)

	browserLab := desktopIDEPreviewSurface{
		ID:               "browser-lab",
		Name:             "Browser Lab",
		Kind:             "browser_inspection",
		Readiness:        desktopPreviewReadiness(packageOK),
		RequiredChecks:   []string{"desktop_package", "browserview_bridge", "task_artifact_handoff"},
		LaunchEndpoint:   "/integrations/desktop/launch",
		CanonicalSurface: "task_browser_inspection_artifacts",
		NextStep:         "Capture browser selections as task browser_inspection artifacts before using them as agent handoff context.",
	}
	if !packageOK {
		browserLab.MissingChecks = append(browserLab.MissingChecks, "desktop_package")
	}
	browserLab.RiskSignals = desktopPreviewRiskSignals(browserLab)

	surfaces := []desktopIDEPreviewSurface{desktop, tray, browserLab}
	status := "ok"
	summary := map[string]int{"total": len(surfaces)}
	for _, surface := range surfaces {
		summary["readiness_"+surface.Readiness]++
		if len(surface.MissingChecks) > 0 {
			summary["missing_check"] += len(surface.MissingChecks)
		}
		if surface.Readiness == "blocked" {
			status = "blocked"
		} else if surface.Readiness == "review" && status == "ok" {
			status = "review"
		}
	}
	return desktopIDEPreviewResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Persisted:   false,
		Status:      status,
		Summary:     summary,
		Surfaces:    surfaces,
	}
}

func desktopPreviewReadiness(ok bool) string {
	if ok {
		return "ready"
	}
	return "blocked"
}

func desktopPreviewRiskSignals(surface desktopIDEPreviewSurface) []string {
	signals := []string{"optional_wrapper", "web_studio_canonical", "no_topology_mutation"}
	if len(surface.MissingChecks) > 0 {
		signals = append(signals, "missing_check")
	}
	return compactStringList(signals)
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
