package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nex-crm/wuphf/internal/team"
)

func runEvalsCmd(args []string) {
	format := "text"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			format = "json"
		case "--format":
			if i+1 < len(args) {
				format = strings.TrimSpace(args[i+1])
				i++
			}
		}
	}
	report := team.RunBehaviorEvals()
	if format == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(report)
	} else {
		fmt.Println("Behavior evals")
		fmt.Printf("  status: %s\n", report.Status)
		for _, c := range report.Cases {
			fmt.Printf("  [%s] %s - %s\n", c.Status, c.ID, c.Summary)
		}
	}
	if report.Status != "pass" {
		os.Exit(1)
	}
}
