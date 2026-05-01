package team

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type openCoDesignStatusResponse struct {
	Available       bool     `json:"available"`
	Executable      string   `json:"executable,omitempty"`
	PrototypeDir    string   `json:"prototype_dir"`
	Message         string   `json:"message"`
	InstallCommands []string `json:"install_commands,omitempty"`
}

type openCoDesignLaunchRequest struct {
	PrototypeDir string `json:"prototype_dir,omitempty"`
}

type openCoDesignLaunchResponse struct {
	OK           bool   `json:"ok"`
	Available    bool   `json:"available"`
	Launched     bool   `json:"launched"`
	Executable   string `json:"executable,omitempty"`
	PrototypeDir string `json:"prototype_dir"`
	Message      string `json:"message"`
}

var openCoDesignLookPath = exec.LookPath

var openCoDesignStartProcess = func(executable, workingDir string) error {
	cmd := exec.Command(executable)
	cmd.Dir = workingDir
	return cmd.Start()
}

func (b *Broker) handleOpenCoDesignStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := resolveOpenCoDesignStatus("")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (b *Broker) handleOpenCoDesignLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body openCoDesignLaunchRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	status := resolveOpenCoDesignStatus(body.PrototypeDir)
	if !status.Available {
		writeOpenCoDesignJSON(w, http.StatusOK, openCoDesignLaunchResponse{
			OK:           false,
			Available:    false,
			Launched:     false,
			PrototypeDir: status.PrototypeDir,
			Message:      status.Message,
		})
		return
	}

	if err := os.MkdirAll(status.PrototypeDir, 0o755); err != nil {
		http.Error(w, fmt.Sprintf("failed to prepare Open CoDesign handoff directory: %v", err), http.StatusInternalServerError)
		return
	}

	if err := openCoDesignStartProcess(status.Executable, status.PrototypeDir); err != nil {
		http.Error(w, fmt.Sprintf("failed to launch Open CoDesign: %v", err), http.StatusInternalServerError)
		return
	}

	_ = b.RecordAction("external_tool_launched", "open-codesign", "studio", "human", "Opened Open CoDesign companion from Studio.", "", nil, "")

	writeOpenCoDesignJSON(w, http.StatusOK, openCoDesignLaunchResponse{
		OK:           true,
		Available:    true,
		Launched:     true,
		Executable:   status.Executable,
		PrototypeDir: status.PrototypeDir,
		Message:      "Open CoDesign launched.",
	})
}

func writeOpenCoDesignJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

func resolveOpenCoDesignStatus(prototypeDir string) openCoDesignStatusResponse {
	resolvedPrototypeDir := resolveOpenCoDesignPrototypeDir(prototypeDir)
	executable := resolveOpenCoDesignExecutable()
	if executable == "" {
		return openCoDesignStatusResponse{
			Available:       false,
			PrototypeDir:    resolvedPrototypeDir,
			Message:         "Open CoDesign is not installed or not on PATH.",
			InstallCommands: openCoDesignInstallCommands(),
		}
	}

	return openCoDesignStatusResponse{
		Available:    true,
		Executable:   executable,
		PrototypeDir: resolvedPrototypeDir,
		Message:      "Open CoDesign is available.",
	}
}

func resolveOpenCoDesignPrototypeDir(prototypeDir string) string {
	prototypeDir = strings.TrimSpace(prototypeDir)
	if prototypeDir == "" {
		prototypeDir = filepath.Join(repoRootForRuntimeDefaults(), "temp", "open-codesign")
	}
	if abs, err := filepath.Abs(prototypeDir); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(prototypeDir)
}

func resolveOpenCoDesignExecutable() string {
	if envPath := strings.TrimSpace(os.Getenv("WUPHF_OPEN_CODESIGN_EXE")); envPath != "" {
		if info, err := os.Stat(envPath); err == nil && !info.IsDir() {
			return envPath
		}
	}

	for _, command := range []string{"open-codesign", "open-codesign.exe"} {
		if path, err := openCoDesignLookPath(command); err == nil && strings.TrimSpace(path) != "" {
			return path
		}
	}

	for _, candidate := range openCoDesignExecutableCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return ""
}

func openCoDesignExecutableCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return filterOpenCoDesignCandidates([]string{
			joinEnvPath("LOCALAPPDATA", "Programs", "open-codesign", "Open CoDesign.exe"),
			joinEnvPath("LOCALAPPDATA", "Programs", "Open CoDesign", "Open CoDesign.exe"),
			joinEnvPath("ProgramFiles", "Open CoDesign", "Open CoDesign.exe"),
			joinEnvPath("ProgramFiles(x86)", "Open CoDesign", "Open CoDesign.exe"),
			joinHomePath("scoop", "shims", "open-codesign.exe"),
			joinHomePath("scoop", "apps", "open-codesign", "current", "Open CoDesign.exe"),
		})
	case "darwin":
		return []string{
			"/Applications/Open CoDesign.app/Contents/MacOS/Open CoDesign",
			"/Applications/open-codesign.app/Contents/MacOS/open-codesign",
		}
	default:
		return []string{
			"/usr/local/bin/open-codesign",
			"/usr/bin/open-codesign",
			"/opt/open-codesign/open-codesign",
		}
	}
}

func filterOpenCoDesignCandidates(candidates []string) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			out = append(out, candidate)
		}
	}
	return out
}

func joinEnvPath(env string, parts ...string) string {
	root := strings.TrimSpace(os.Getenv(env))
	if root == "" {
		return ""
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

func joinHomePath(parts ...string) string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(append([]string{home}, parts...)...)
}

func openCoDesignInstallCommands() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			"winget install OpenCoworkAI.OpenCoDesign",
			"scoop bucket add opencoworkai https://github.com/OpenCoworkAI/scoop-bucket",
			"scoop install open-codesign",
		}
	case "darwin":
		return []string{
			"brew install --cask opencoworkai/tap/open-codesign",
		}
	default:
		return []string{
			"Download a Linux package from https://github.com/OpenCoworkAI/open-codesign/releases",
		}
	}
}
