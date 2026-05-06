package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type releaseCheckReport struct {
	GeneratedAt string                `json:"generated_at"`
	Status      string                `json:"status"`
	Checks      []releaseCheckResult  `json:"checks"`
	Artifact    *releaseCheckArtifact `json:"artifact,omitempty"`
}

type releaseCheckResult struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Summary    string `json:"summary"`
	Detail     string `json:"detail,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

type releaseCheckArtifact struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Summary   string `json:"summary"`
	Path      string `json:"path,omitempty"`
	MIMEType  string `json:"mime_type,omitempty"`
	Checksum  string `json:"checksum,omitempty"`
	CreatedAt string `json:"created_at"`
}

func runReleaseCheckCmd(args []string) {
	format := "text"
	skipBuild := false
	artifactPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			format = "json"
		case "--skip-build":
			skipBuild = true
		case "--artifact":
			if i+1 < len(args) {
				artifactPath = strings.TrimSpace(args[i+1])
				i++
			}
		case "--format":
			if i+1 < len(args) {
				format = strings.TrimSpace(args[i+1])
				i++
			}
		}
	}
	report := runReleaseChecks(skipBuild)
	if strings.TrimSpace(artifactPath) != "" {
		artifact, err := writeReleaseCheckArtifact(report, artifactPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error writing release artifact: %v\n", err)
			os.Exit(1)
		}
		report.Artifact = &artifact
	}
	if format == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(report)
	} else {
		fmt.Println("Release check")
		fmt.Printf("  status: %s\n", report.Status)
		for _, check := range report.Checks {
			fmt.Printf("  [%s] %s - %s\n", check.Status, check.ID, check.Summary)
			if strings.TrimSpace(check.Detail) != "" {
				fmt.Printf("       %s\n", check.Detail)
			}
		}
		if report.Artifact != nil {
			fmt.Printf("  artifact: %s (%s)\n", report.Artifact.Path, report.Artifact.State)
		}
	}
	if report.Status != "ready" {
		os.Exit(1)
	}
}

func runReleaseChecks(skipBuild bool) releaseCheckReport {
	checks := make([]releaseCheckResult, 0, 5)
	checks = append(checks, runReleaseCommandCheck("go-team-tests", "", 2*time.Minute, "go", "test", "./internal/team", "-run", "TestPaperclipPhase15|TestPaperclipPhase14|TestPaperclipPhase13|TestPaperclipPhase12", "-count=1"))
	if !skipBuild {
		checks = append(checks, runReleaseCommandCheck("web-build", filepath.Join(".", "web"), 2*time.Minute, "npm", "run", "build"))
	}
	checks = append(checks, releaseRuntimeSmokeCheck())
	checks = append(checks, releaseReadinessCheck())
	checks = append(checks, releaseGitStatusCheck())
	status := "ready"
	for _, check := range checks {
		switch check.Status {
		case "fail", "blocked":
			status = "blocked"
		case "warn", "review":
			if status == "ready" {
				status = "review"
			}
		}
	}
	return releaseCheckReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Status: status, Checks: checks}
}

func writeReleaseCheckArtifact(report releaseCheckReport, targetPath string) (releaseCheckArtifact, error) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return releaseCheckArtifact{}, fmt.Errorf("artifact path required")
	}
	cleanPath := filepath.Clean(targetPath)
	if dir := filepath.Dir(cleanPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return releaseCheckArtifact{}, err
		}
	}
	state := "draft"
	if report.Status == "ready" {
		state = "accepted"
	} else if report.Status == "blocked" {
		state = "rejected"
	}
	artifact := releaseCheckArtifact{
		ID:        "release-check-" + strings.ToLower(report.Status),
		Kind:      "release_check",
		Title:     "Release check artifact",
		State:     state,
		Summary:   fmt.Sprintf("Release check %s with %d checks.", report.Status, len(report.Checks)),
		Path:      cleanPath,
		MIMEType:  "application/json",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	report.Artifact = &artifact
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return releaseCheckArtifact{}, err
	}
	sum := sha256.Sum256(raw)
	artifact.Checksum = hex.EncodeToString(sum[:])
	report.Artifact = &artifact
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		return releaseCheckArtifact{}, err
	}
	if err := os.WriteFile(cleanPath, append(raw, '\n'), 0o644); err != nil {
		return releaseCheckArtifact{}, err
	}
	return artifact, nil
}

func runReleaseCommandCheck(id, dir string, timeout time.Duration, name string, args ...string) releaseCheckResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	result := releaseCheckResult{ID: id, DurationMs: time.Since(start).Milliseconds()}
	detail := strings.TrimSpace(out.String())
	if len(detail) > 1000 {
		detail = detail[len(detail)-1000:]
	}
	result.Detail = detail
	if ctx.Err() != nil {
		result.Status = "fail"
		result.Summary = "command timed out"
		return result
	}
	if err != nil {
		result.Status = "fail"
		result.Summary = err.Error()
		return result
	}
	result.Status = "ok"
	result.Summary = "passed"
	return result
}

func releaseRuntimeSmokeCheck() releaseCheckResult {
	snapshot := fetchRuntimeSmokeSnapshot()
	if len(snapshot) == 0 {
		return releaseCheckResult{ID: "runtime-smoke", Status: "warn", Summary: "live broker smoke not reachable"}
	}
	status := strings.ToLower(firstNonEmptyCLI(mapStringAny(snapshot, "status"), "unknown"))
	checkStatus := "ok"
	if status != "ok" {
		checkStatus = "warn"
	}
	return releaseCheckResult{ID: "runtime-smoke", Status: checkStatus, Summary: "runtime smoke status: " + status}
}

func releaseReadinessCheck() releaseCheckResult {
	snapshot := fetchReleaseReadinessSnapshot()
	if len(snapshot) == 0 {
		return releaseCheckResult{ID: "release-readiness", Status: "warn", Summary: "readiness endpoint not reachable"}
	}
	status := strings.ToLower(firstNonEmptyCLI(mapStringAny(snapshot, "status"), "unknown"))
	score := ""
	if raw, ok := snapshot["score"].(float64); ok {
		score = fmt.Sprintf(" score %.0f/100", raw)
	}
	checkStatus := "ok"
	if status == "blocked" {
		checkStatus = "fail"
	} else if status != "ready" {
		checkStatus = "warn"
	}
	return releaseCheckResult{ID: "release-readiness", Status: checkStatus, Summary: "release readiness: " + status + score}
}

func releaseGitStatusCheck() releaseCheckResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "status", "--short")
	out, err := cmd.Output()
	result := releaseCheckResult{ID: "git-status", DurationMs: time.Since(start).Milliseconds()}
	if ctx.Err() != nil {
		result.Status = "fail"
		result.Summary = "command timed out"
		return result
	}
	if err != nil {
		result.Status = "fail"
		result.Summary = err.Error()
		return result
	}
	lines := compactCLILines(string(out))
	detail := strings.TrimSpace(string(out))
	if len(detail) > 1000 {
		detail = detail[len(detail)-1000:]
	}
	result.Detail = detail
	result.Status = "ok"
	if len(lines) == 0 {
		result.Summary = "clean"
		return result
	}
	result.Status = "warn"
	result.Summary = fmt.Sprintf("%d changed paths", len(lines))
	return result
}

func fetchReleaseReadinessSnapshot() map[string]any {
	token := currentBrokerAuthToken()
	if token != "" {
		if snapshot := fetchRuntimeDoctorURL(brokerURL("/release/readiness"), token); len(snapshot) > 0 {
			return snapshot
		}
	}
	return fetchRuntimeDoctorURL("http://127.0.0.1:7891/api/release/readiness", "")
}

func mapStringAny(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func compactCLILines(text string) []string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
