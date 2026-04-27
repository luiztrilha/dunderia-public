# Public Export Exclusions

The public profile excludes data that should stay private or machine-local:

- authentication files, OAuth state, tokens, and API keys
- Codex session logs, history, archives, shell snapshots, and telemetry
- local memories and workspace-specific personal context
- raw user-home configuration folders
- raw Codex `config.toml`
- private approval rules containing real paths or project names
- local database helper skills and commands
- browser profiles, downloads, debug output, and caches
- live DunderIA office state such as `company.json`, `broker-state.json`, or runtime journals

The shipped config files are examples or sanitized shapes. Keep real credentials
and real machine paths in a private repository or a local-only secret store.
