package main

import (
	"fmt"
	"os"

	"github.com/nex-crm/wuphf/internal/team"
)

func main() {
	stats, err := team.RepairChannelMemory(os.Args[1:]...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error rebuilding channel memory: %v\n", err)
		os.Exit(1)
	}
	if len(stats) == 0 {
		fmt.Println("No channel memory namespaces matched the requested scope.")
		return
	}
	fmt.Println("Channel memory rebuild complete.")
	for _, stat := range stats {
		fmt.Printf("  %s: %d -> %d\n", stat.Channel, stat.Before, stat.After)
	}
}
