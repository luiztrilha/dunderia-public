# Development

## Office Build

```bash
go build -o wuphf ./cmd/wuphf
```

For normal app usage you do not need Bun. The local office/team MCP tools now run from the main Go binary through the hidden `wuphf mcp-team` subcommand.

## Latest Published CLI

The old standalone CLI is no longer vendored in this repo.

If you need the latest published CLI separately:

```bash
bash scripts/install-latest-wuphf-cli.sh
```

The same install step is also wired into setup:

```bash
./wuphf init
```

With cloud backup configured and reachable, `wuphf init` also restores and re-mirrors the lightweight local machine state (`company.json`, `onboarded.json`, `cloud-backup-bootstrap.json`, Codex auth/config/skills, agent skills, and Google ADC when present in backup). Large local repos remain out of scope.

## Windows User Logon Bootstrap

To register the current-user Windows logon bootstrap that silently starts `codex-lb`, `ollama`, `Google DriveFS`, and repo `wuphf.exe`:

```powershell
pwsh -File scripts/windows/install_user_logon_bootstrap.ps1
```

To remove it:

```powershell
pwsh -File scripts/windows/uninstall_user_logon_bootstrap.ps1
```

## Environments

The DunderIA runtime still reads `WUPHF_BASE_URL` as a legacy compatibility variable for the Nex-compatible API surface, falling back to `https://app.nex.ai` in production only when that backend is explicitly used. The staging hostname below still carries the old `wuphf.ai` name because that legacy backend has not been renamed.

| Environment | `WUPHF_BASE_URL` |
|-------------|----------------|
| Production  | _(unset — default)_ |
| Staging     | `https://app.staging.wuphf.ai` |
| Local       | `http://localhost:30000` |

### Switching environments

```bash
# Staging
export WUPHF_BASE_URL="https://app.staging.wuphf.ai"

# Local
export WUPHF_BASE_URL="http://localhost:30000"

# Back to production
unset WUPHF_BASE_URL
```

or set it directly in `.zshrc` or `.bashrc`.
