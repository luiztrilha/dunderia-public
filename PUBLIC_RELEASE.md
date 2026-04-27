# Public Release Guide

Use this checklist before publishing a public DunderIA build or fork.

The goal is to ship the validated operating profile, not a copy of a private office. Keep the public release focused on reusable docs, templates, real skills, prompts, rules, and sanitized configuration.

## Ship

- Product docs: `README.md`, `ARCHITECTURE.md`, `DEVELOPMENT.md`, `FORKING.md`, `CONTRIBUTING.md`, `SECURITY.md`, `SUPPORT.md`.
- Validated onboarding profile: `templates/starter-kit/`.
- Operation and employee blueprints: `templates/operations/` and `templates/employees/`, after reviewing for customer-specific language.
- Sanitized global agent rules: `templates/starter-kit/codex/AGENTS.validated.md`.
- Validated skills from the active local profile, excluding private/customer-specific skills.
- Sanitized active config shape: `templates/starter-kit/codex/config.sanitized.toml`.
- Public local runtime profile guide: `docs/local-runtime-profile.md`, describing how to separate reusable skills, Markdown orientation and sanitized config without shipping private state.
- Public base office topology in code: `ceo`, `pm`, `backend`, `builder`, `frontend`, `reviewer`, `estagiario`, and `game-master` in `#general`.
- Public web experience: shell, layout, themes, message views, channel/DM navigation, and onboarding screens aligned with the validated local `web/` source.
- Public starter DMs for the validated base agents, seeded without message history.

## Do Not Ship

- Auth and secrets: `~/.codex/auth.json`, API keys, cloud credentials, ADC files, `.env*`, certificates, private tokens.
- Live office state: `company.json`, `broker-state.json`, `onboarded.json`, cloud backup bootstrap state, workflow state, task receipts, channel history, and DM messages.
- Protected topology from a private office: customer-specific channels, linked repos, saved workflows that recreate a private team, or blueprints generated from private operations.
- Local mirrors and client code: `.links/`, private archive folders, generated help mirrors, local customer repos.
- Scratch artifacts: `.tmp-*`, test logs, local binaries, screenshots unless they are intentionally documented assets.
- Personal model preferences that only work on your machine.

## Public Starter Kit

The starter kit lives in `templates/starter-kit/` and is meant to be copied by a new user into their own workspace:

```text
templates/starter-kit/
  README.md
  codex/
  agents/
  prompts/
  rules/
  policies.validated.md
  EXCLUSIONS.md
  install-profile.ps1
```

Recommended public onboarding flow:

1. User installs DunderIA.
2. User runs `wuphf init`.
3. User reviews `templates/starter-kit/EXCLUSIONS.md`.
4. User installs the validated profile with `templates/starter-kit/install-profile.ps1`.
5. User merges supported settings from `templates/starter-kit/codex/config.sanitized.toml`.
6. User starts with the public office shell, agent DMs, and a blueprint from `templates/operations/` or with `wuphf --from-scratch`.

## Sanitization Pass

Before pushing public history or a release branch:

```powershell
git status --short
rg -n "api_key|token|secret|password|credential|Authorization|Bearer|BEGIN .*PRIVATE|client_secret|refresh_token" .
rg -n "D:\\\\|C:\\\\Users|\\.links|PRIVATE_CLIENT_NAME|PRIVATE_REPO_NAME|company\\.json|broker-state|auth\\.json|application_default_credentials" .
```

Review matches manually. Some hits are documentation or tests, but every hit should be intentional.

## Release Branch Rule

Cut a clean public branch from a known tag or commit. Do not publish from a worktree that includes private linked repos, local office state, or temporary binaries.

Use this as the minimum release gate:

```powershell
go test ./...
npm --prefix web test
git status --short
```

If the web dependencies are not installed, document that the web test command was not run and why.

## Migration Notes

DunderIA still uses the historical `wuphf` binary/package name in several places. That is expected for compatibility. Public docs should call the product DunderIA and treat `wuphf` as the CLI name unless the fork intentionally renames the technical surface.
