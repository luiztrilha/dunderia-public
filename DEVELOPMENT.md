# Development

## Office Build

```bash
go build -o wuphf ./cmd/wuphf
```

## Stable Local Tests

For repeatable local runs, isolate DunderIA runtime state, task logs, Go temp, and Go build cache:

```powershell
pwsh -File scripts/run-stable-tests.ps1
```

To pass normal `go test` arguments:

```powershell
pwsh -File scripts/run-stable-tests.ps1 -Package ./internal/agent -GoTestArg -run=TestReadTaskLogRange
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

## Paperclip-Inspired Runtime Hardening

The current absorption plan lives in [`docs/paperclip-absorption-plan.md`](docs/paperclip-absorption-plan.md). It tracks what has already been ported, what should be ported next, and what stays out of scope unless explicitly authorized.

Before preparing a public release, run:

```powershell
pwsh -File scripts/check-public-release.ps1
pwsh -File scripts/validate-starter-kit-skills.ps1
```

Add project-specific forbidden tokens to `.git/hooks/forbidden-tokens.txt` or pass `-Token` values explicitly.

For local broker/web process hygiene on Windows:

```powershell
pwsh -File scripts/dev-services.ps1
pwsh -File scripts/dev-services.ps1 -Action stop
pwsh -File scripts/dev-services.ps1 -Action stop -Force
```

The stop action is dry-run by default. `-Force` is required before the script kills matching local development processes.

## Local Secret Store

DunderIA includes an encrypted local secret store for manual operator use:

```powershell
$env:WUPHF_SECRET_STORE_PASSPHRASE = "<local passphrase>"
go run ./cmd/wuphf secret set --name openai_api_key --value "<secret>"
go run ./cmd/wuphf secret list
go run ./cmd/wuphf secret get --name openai_api_key
go run ./cmd/wuphf secret delete --name openai_api_key
```

To migrate existing plaintext secret fields from `config.json`, start with the dry-run:

```powershell
go run ./cmd/wuphf secret migrate-config
go run ./cmd/wuphf secret migrate-config --write
go run ./cmd/wuphf secret migrate-config --write --clear-config --confirm-clear-plaintext
```

The store writes to `.wuphf/secrets.enc.json` under the runtime home unless `WUPHF_SECRET_STORE_PATH` or `--path` is provided. Existing env vars and `config.json` fields remain the active runtime resolution path until an explicit resolver migration is added.
