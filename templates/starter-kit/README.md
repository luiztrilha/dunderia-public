# DunderIA Validated Starter Kit

This folder contains a sanitized export of the working profile currently used to develop and operate DunderIA.

It is not a toy example. The skills, prompts, rules, and operating defaults here come from the local setup that has been used in practice. Items that would leak credentials, private customer context, local office state, or live topology are excluded or replaced with placeholders.

## Contents

- `codex/AGENTS.validated.md`: sanitized global Codex instructions.
- `codex/config.sanitized.toml`: sanitized version of the active Codex config shape.
- `codex/skills/`: validated local Codex skills, excluding `.system` and private database skills.
- `codex/superpowers/skills/`: validated Superpowers workflows.
- `agents/skills/`: validated design and UI skills from the active `.agents` profile.
- `agents/skill-lock.json`: source metadata for installed `.agents` skills.
- `prompts/`: validated OpenSpec prompt commands.
- `rules/default.rules`: active local approval/prefix rules, retained for review before public use.
- `policies.validated.md`: operating policies distilled from the active global and repo rules.
- `EXCLUSIONS.md`: what was intentionally not exported and why.
- `install-profile.ps1`: copies this profile into a user's local Codex/Agents home.

## Suggested Setup

From the DunderIA repo:

```powershell
Copy-Item templates\starter-kit\codex\AGENTS.validated.md AGENTS.md
```

To install the local profile:

```powershell
pwsh -ExecutionPolicy Bypass -File templates\starter-kit\install-profile.ps1
```

The script installs:

- Codex skills into `$HOME\.codex\skills`
- Superpowers into `$HOME\.codex\superpowers\skills`
- Prompts into `$HOME\.codex\prompts`
- Rules into `$HOME\.codex\rules`
- Agent skills into `$HOME\.agents\skills`

It does not install `auth.json`, live state, private database skills, or cloud credentials.

After reviewing `codex/config.sanitized.toml`, copy the supported settings you want into your own `$HOME\.codex\config.toml`.

Then run DunderIA:

```powershell
wuphf init
wuphf
```

## What Was Sanitized

- Absolute private repo paths were replaced with placeholders where they mattered.
- Auth, history, sessions, sqlite state, logs, and cloud credentials were not copied.
- `.codex/skills/.system` was not copied because it belongs to the Codex runtime distribution.
- `sql-exampleworkflow` was not copied because it is tied to private database and repository context.
- Live office topology was not copied. Use blueprints instead of shipping `company.json` or `broker-state.json`.
