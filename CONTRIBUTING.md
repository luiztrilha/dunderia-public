# Contributing

## Before You Start

- Open an issue first for large changes, public API shifts, or architecture changes.
- Keep the local-first runtime promise intact: local broker, fresh per-turn runners, scoped MCP, isolated worktrees.
- Treat Nex, Telegram, One, and Composio as optional integrations, not baseline assumptions.

## Development Setup

Core build:

```bash
go build -o wuphf ./cmd/wuphf
```

Web build:

```bash
npm --prefix web install
npm --prefix web run build
```

## Validation

Before opening a PR, run the smallest meaningful checks that prove your change.

Common checks:

```bash
go test ./...
npm --prefix web run build
```

If the full suite is too expensive locally, run the nearest relevant package tests and say exactly what you did not run in the PR description.

## Change Expectations

- Keep docs aligned with behavior when you change commands, flags, runtime assumptions, or onboarding text.
- Prefer targeted changes over broad refactors.
- Do not silently reintroduce hosted or shared-memory assumptions into local-only flows.
- Avoid logging secrets, message contents, or private user data unless that logging is explicitly gated behind a debug path.

## Pull Requests

Include:

- what changed
- why it changed
- how you validated it
- what remains unverified, if anything

If your change affects onboarding, runtime setup, install paths, or public docs, call that out explicitly.
