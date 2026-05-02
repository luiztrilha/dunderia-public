# Starter Kit Exclusions

This profile intentionally does not export everything byte-for-byte. It exports the validated working playbooks and configuration shape while excluding files that are unsafe or too local for a public release.

## Excluded

- `~/.codex/auth.json`: authentication material.
- `~/.codex/history.jsonl`: private conversation history.
- `~/.codex/sessions/`, `archived_sessions/`, `log/`, `tmp/`, `.tmp/`, `cache/`: runtime state.
- `~/.codex/state_*.sqlite`, `logs_*.sqlite`: local state and logs.
- `~/.codex/models_cache.json`, `installation_id`, `cap_sid`: local installation metadata.
- `~/.codex/skills/.system`: Codex runtime-provided skills, not project-owned profile content.
- `~/.codex/skills/sql-convenios`: private database/repository skill tied to a non-public client environment.
- `~/.codex/memories/serena-runtime-*`: local tool runtime distribution.
- Private `.megamemory` database paths and disabled private MCP server entries.
- Personal orientation files such as `MEMORY.md`, `USER.md`, `SOUL.md`, `IDENTITY.md`, `TOOLS.md`, heartbeat state, and workspace-specific standards indexes.
- Full Claude plugin marketplace mirrors and generated upstream reference trees; keep those as upstream references, not installed starter-kit content.
- Live DunderIA state: `company.json`, `broker-state.json`, onboarding state, saved workflows, task receipts, and channel history.
- Cloud backup bootstrap files, ADC credentials, API keys, private keys, `.env*`, certificates, and local browser profiles.

## Exported Instead

- Sanitized global instructions in `codex/AGENTS.validated.md`.
- Sanitized active config shape in `codex/config.sanitized.toml`.
- Validated local skills in `codex/skills/`.
- Validated Superpowers workflows in `codex/superpowers/skills/`.
- Validated design/UI skills in `agents/skills/`.
- Claude Code command wrappers in `claude/commands/`.
- OpenCode GitNexus skills in `opencode/skills/`.
- Prompt commands in `prompts/`.
- Local rules in `rules/default.rules`.
- Validated policies in `policies.validated.md`.
- Public base topology in the runtime default manifest: `ceo`, `pm`, `research-lead`, `estagiario`, `backend`, `frontend`, `builder`, `reviewer`, and `game-master` in `#general`.

## Private Skill Note

`sql-convenios` is useful in the private environment but should not ship publicly because it names private repositories, database workflows, and operational tables. If a public user needs a SQL skill, create a new domain-neutral database skill that reads connection details from their own local config.
