# Local Runtime Profile

DunderIA uses two different profile shapes. Keep them separate.

## Active Runtime

The active private runtime lives outside this repo:

- Codex config and skills: `C:\Users\l.sousa\.codex`
- Shared agent skills: `C:\Users\l.sousa\.agents`
- Workspace rules and memory: `D:\Repos`

This is the source of truth for the local machine. Do not infer active behavior from an exported template without checking the active paths above.

## Public Starter Kit

The reusable public baseline lives in:

- `templates/starter-kit/`

Use this when a fresh DunderIA install should receive validated public defaults:

- Codex skills
- Superpowers skills
- `.agents` design skills
- OpenSpec prompts
- sanitized rules and config shape
- public base office bootstrap

The broker may point to `templates/starter-kit/...` skill paths to make packaged skills visible in a fresh office.

## Local Runtime Profile Template

The broad reference snapshot lives in the public export repo:

- `D:\Repos\dunderia-public-export\templates\local-runtime-profile`
- `D:\Repositórios\dunderia-public-export\templates\local-runtime-profile`

Treat it as reference-only material. It is useful for comparing what existed in a private runtime snapshot, but it should not be installed wholesale into this repo or into a live Codex profile.

It includes extra material that the active Codex setup may not consume directly, including Claude plugin docs, Opencode skills, broad orientation docs and sanitized config examples.

## Promotion Rules

Promote from `local-runtime-profile` only when all are true:

- the content is reusable outside the private machine
- it does not contain secrets, live state, history, customer context or private paths
- it fits an active consumer: Codex, DunderIA starter-kit, `.agents`, docs or prompts
- it has a clear canonical destination

Preferred destinations:

- reusable Codex behavior: `C:\Users\l.sousa\.codex\AGENTS.md`
- reusable DunderIA repo behavior: this repo under `docs/` or `AGENTS.md`
- public bootstrap assets: `templates/starter-kit/`
- public explanation: `docs/`
- private daily/context memory: `D:\Repos\memory\YYYY-MM-DD.md`

Do not promote:

- `MEMORY.md` snapshots
- `history.jsonl`, sessions, logs, sqlite databases or browser state
- `company.json`, `broker-state.json`, request journals or DM history
- raw `config.toml`, approval rules with private paths, tokens or credentials
- private database skills such as repo-local SQL helpers

## Drift Check

Use this when deciding whether the exported profile has anything worth promoting:

```powershell
$profile = 'D:\Repos\dunderia-public-export\templates\local-runtime-profile'
Get-ChildItem -Directory "$profile\skills\codex" | Select-Object -ExpandProperty Name
Get-ChildItem -Directory "$HOME\.codex\skills" | Select-Object -ExpandProperty Name
Get-ChildItem -Directory "$profile\skills\agents" | Select-Object -ExpandProperty Name
Get-ChildItem -Directory "$HOME\.agents\skills" | Select-Object -ExpandProperty Name
```

Compare only names and intended consumers first. Review file content manually before copying.
