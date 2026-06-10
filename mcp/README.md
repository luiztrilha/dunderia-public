# MCP Runtime

`mcp/dist` is the runnable MCP runtime in this repo today.

`mcp/dunderia-mcp-settings.json` is the tracked product-local MCP profile. It stays intentionally small and local-first:

- `github`
- `playwright`
- `brave-search`
- repo-local `filesystem`
- repo-local `megamemory-dunderia`

Expanding that profile to additional repos or servers is opt-in per workstation. It is not part of the base checked-in profile.

For the active shared Codex workspace, prefer the neutral layer under `D:\Repos\scripts\mcp\` and `D:\Repos\config\codex\workspace-mcp-settings.json` instead of treating this repo-local profile as the machine-wide default.

The profile is portable by design: it uses `powershell.exe` from `PATH`, `${workspaceFolder}` for repo-local launchers, and does not opt into `ExecutionPolicy Bypass`.

When using `MCP_TRANSPORT=http`, bind stays on `127.0.0.1` by default and the server requires a dedicated bearer token via `WUPHF_MCP_HTTP_TOKEN` or `MCP_HTTP_TOKEN`.

`mcp/src` is not the source of truth for a full rebuild of the runtime. See `mcp/src/README.md` before editing files there.
