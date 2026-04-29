package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nex-crm/wuphf/internal/config"
)

func printSecretHelp() {
	fmt.Fprintln(os.Stderr, "wuphf secret — manage the local encrypted secret store")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  wuphf secret list")
	fmt.Fprintln(os.Stderr, "  wuphf secret get --name openai_api_key")
	fmt.Fprintln(os.Stderr, "  wuphf secret set --name openai_api_key --value <secret>")
	fmt.Fprintln(os.Stderr, "  wuphf secret delete --name openai_api_key")
	fmt.Fprintln(os.Stderr, "  wuphf secret migrate-config")
	fmt.Fprintln(os.Stderr, "  wuphf secret migrate-config --write")
	fmt.Fprintln(os.Stderr, "  wuphf secret migrate-config --write --clear-config --confirm-clear-plaintext")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Passphrase source:")
	fmt.Fprintln(os.Stderr, "  --passphrase <value> or WUPHF_SECRET_STORE_PASSPHRASE")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "The runtime does not read this store automatically yet; env vars and config.json remain authoritative.")
}

func runSecretCmd(args []string) {
	if len(args) == 0 || subcommandWantsHelp(args) {
		printSecretHelp()
		return
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	fs := flag.NewFlagSet("secret "+action, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "Secret name")
	value := fs.String("value", "", "Secret value")
	path := fs.String("path", "", "Secret store path")
	passphrase := fs.String("passphrase", "", "Secret store passphrase")
	write := fs.Bool("write", false, "Write discovered config secrets into the encrypted store")
	clearConfig := fs.Bool("clear-config", false, "Clear migrated plaintext values from config.json")
	confirmClear := fs.Bool("confirm-clear-plaintext", false, "Confirm plaintext config clearing")
	if err := fs.Parse(args[1:]); err != nil {
		os.Exit(2)
	}

	storePassphrase := strings.TrimSpace(*passphrase)
	if storePassphrase == "" {
		storePassphrase = os.Getenv("WUPHF_SECRET_STORE_PASSPHRASE")
	}

	switch action {
	case "migrate-config":
		runSecretMigrateConfig(*path, storePassphrase, *write, *clearConfig, *confirmClear)
	case "list":
		store := mustOpenSecretStore(*path, storePassphrase)
		names, err := store.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, item := range names {
			fmt.Println(item)
		}
	case "get":
		store := mustOpenSecretStore(*path, storePassphrase)
		secret, ok, err := store.Get(*name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "secret %q not found\n", strings.TrimSpace(*name))
			os.Exit(1)
		}
		fmt.Println(secret)
	case "set", "put":
		store := mustOpenSecretStore(*path, storePassphrase)
		if err := store.Put(*name, *value); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Stored %s in %s\n", strings.TrimSpace(*name), store.Path())
	case "delete", "rm":
		store := mustOpenSecretStore(*path, storePassphrase)
		deleted, err := store.Delete(*name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if deleted {
			fmt.Printf("Deleted %s from %s\n", strings.TrimSpace(*name), store.Path())
			return
		}
		fmt.Printf("Secret %s was not present in %s\n", strings.TrimSpace(*name), store.Path())
	default:
		printSecretHelp()
		os.Exit(2)
	}
}

func mustOpenSecretStore(path, passphrase string) *config.SecretStore {
	store, err := config.NewSecretStore(path, passphrase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return store
}

func runSecretMigrateConfig(path, passphrase string, write, clearConfig, confirmClear bool) {
	if clearConfig && !write {
		fmt.Fprintln(os.Stderr, "error: --clear-config requires --write")
		os.Exit(1)
	}
	if clearConfig && !confirmClear {
		fmt.Fprintln(os.Stderr, "error: --clear-config requires --confirm-clear-plaintext")
		os.Exit(1)
	}
	if !write {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Config secret migration dry-run. Re-run with --write to copy values into the encrypted store.")
		for _, candidate := range config.ConfigSecretCandidates(cfg) {
			if candidate.Present {
				fmt.Printf("present: %s\n", candidate.Name)
			}
		}
		return
	}

	store := mustOpenSecretStore(path, passphrase)
	results, err := config.MigrateConfigSecretsToStore(store, clearConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for _, result := range results {
		if !result.Present {
			continue
		}
		status := "migrated"
		if result.Cleared {
			status += ", cleared"
		}
		fmt.Printf("%s: %s\n", status, result.Name)
	}
	fmt.Printf("Secret store: %s\n", store.Path())
	if !clearConfig {
		fmt.Println("Plaintext config values were preserved. Add --clear-config --confirm-clear-plaintext after verifying the store.")
	}
}
