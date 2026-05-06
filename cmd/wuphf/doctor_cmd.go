package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func runDoctorCmd(args []string) {
	format := "text"
	smoke := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			format = "json"
		case "--smoke":
			smoke = true
		case "--format":
			if i+1 < len(args) {
				format = strings.TrimSpace(args[i+1])
				i++
			}
		}
	}
	if smoke {
		runDoctorSmoke(format)
		return
	}
	report, err := inspectDoctor()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: doctor failed: %v\n", err)
		os.Exit(1)
	}
	runtimeDoctor := fetchRuntimeDoctorSnapshot()
	if format == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"setup":          report,
			"runtime_doctor": runtimeDoctor,
		})
		return
	}
	fmt.Println("Doctor")
	fmt.Println("  " + report.StatusLine())
	for _, check := range report.Checks {
		fmt.Printf("  [%s] %s — %s\n", check.Severity, check.Label, check.Detail)
		if strings.TrimSpace(check.NextStep) != "" {
			fmt.Printf("       next: %s\n", check.NextStep)
		}
	}
	if len(runtimeDoctor) == 0 {
		fmt.Println("  [info] runtime doctor — live broker not reachable or not authenticated")
		return
	}
	if status, _ := runtimeDoctor["status"].(string); status != "" {
		fmt.Printf("  [%s] runtime doctor — live office runtime snapshot\n", status)
	}
	if checks, ok := runtimeDoctor["checks"].([]any); ok {
		for _, raw := range checks {
			check, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			fmt.Printf("       [%v] %v — %v\n", check["severity"], check["label"], check["summary"])
		}
	}
}

func runDoctorSmoke(format string) {
	snapshot := fetchRuntimeSmokeSnapshot()
	if format == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"smoke": snapshot})
		return
	}
	if len(snapshot) == 0 {
		fmt.Println("Runtime smoke")
		fmt.Println("  [warn] live broker not reachable or not authenticated")
		return
	}
	status, _ := snapshot["status"].(string)
	fmt.Println("Runtime smoke")
	fmt.Printf("  [%s] smoke snapshot\n", firstNonEmptyCLI(status, "unknown"))
	if checks, ok := snapshot["checks"].([]any); ok {
		for _, raw := range checks {
			check, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			fmt.Printf("       [%v] %v — %v\n", check["status"], check["id"], check["summary"])
			if next, _ := check["next_step"].(string); strings.TrimSpace(next) != "" {
				fmt.Printf("            next: %s\n", next)
			}
		}
	}
}

func fetchRuntimeDoctorSnapshot() map[string]any {
	token := currentBrokerAuthToken()
	if token != "" {
		if snapshot := fetchRuntimeDoctorURL(brokerURL("/runtime/doctor"), token); len(snapshot) > 0 {
			return snapshot
		}
	}
	return fetchRuntimeDoctorURL("http://127.0.0.1:7891/api/runtime/doctor", "")
}

func fetchRuntimeSmokeSnapshot() map[string]any {
	token := currentBrokerAuthToken()
	if token != "" {
		if snapshot := fetchRuntimeDoctorURL(brokerURL("/runtime/smoke"), token); len(snapshot) > 0 {
			return snapshot
		}
	}
	return fetchRuntimeDoctorURL("http://127.0.0.1:7891/api/runtime/smoke", "")
}

func firstNonEmptyCLI(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fetchRuntimeDoctorURL(url string, token string) map[string]any {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	return out
}
