# Paperclip Absorption Plan

This plan captures the Paperclip ideas that are worth bringing into DunderIA while preserving DunderIA's local-first Go runtime and protected office topology.

## Non-negotiable Boundaries

- Do not mutate `company.json`, `broker-state.json`, onboarding state, saved workflows, agent rosters, channel lists, or blueprints that change rosters/channels without explicit current user authorization.
- Prefer DunderIA-native Go/runtime patterns over copying Paperclip's TypeScript database/control-plane architecture.
- Port behavior contracts, checks, and narrow runtime utilities first; only port heavier subsystems after migration and rollback paths exist.
- Keep Nex, GBrain, Telegram, Composio, and external adapters optional.

## Phase 1 - Absorbed Now

- Runtime contract: codify wake paths, task states, ownership, blockers, recovery, usage visibility, skills, adapter boundaries, and operator handoff in `RUNTIME_CONTRACT.md`.
- Workflow skills: add DunderIA-adapted plan-to-tasks, doc-maintenance, and contribution-report skills to the starter kit.
- Contribution governance: require a clearer PR story, verification, risks, model disclosure, and UI evidence when relevant.
- Sensitive-area ownership map: define CODEOWNERS patterns for release, workflows, config, topology/state code, starter-kit assets, lockfiles, and runtime broker surfaces.
- Public-release scanner: add a local script that checks tracked files for dynamic/local forbidden tokens before packaging.
- Stable test runner: add a PowerShell runner that isolates `WUPHF_RUNTIME_HOME`, task logs, Go cache, and temp directories for repeatable local test runs.
- Agent behavior eval seed: document the first prompt/eval scenarios for task pickup, approval gates, blockers, company boundaries, and durable state changes.
- Log-read hardening: expose paginated task-log reads with optional SHA-256 so large logs can be inspected without loading the whole file.

## Phase 2 - Runtime Liveness Patch Absorbed

- Added a DunderIA-native run liveness classifier with states equivalent to: `advanced`, `completed`, `blocked`, `failed`, `empty_response`, `plan_only`, and `needs_followup`.
- Integrated the classifier into headless turn completion for office-mode tasks, after the existing durable-state checks, so it complements rather than replaces current coding/local-worktree/live-external guardrails.
- Added tests around "agent promised future work", empty successful response, durable completion, narrative research progress, office task mutation, and blocked task acceptance.
- Surfaced the latest liveness verdict through agent activity snapshots, session observability, SSE activity payloads, `/members`, and the web runtime summary.
- Still pending: add a dedicated task-detail visual treatment for liveness history instead of only showing the latest agent/runtime verdict.

## Phase 3 - Secrets And Plugin Safety

- Added starter-kit skill manifest validation for `skill-lock.json`, packaged `SKILL.md` paths, frontmatter names/descriptions, source types, and portable paths.
- Added an encrypted local secret store backed by AES-GCM and scrypt, plus a manual `wuphf secret` CLI for set/get/list/delete.
- Env vars and existing `config.json` fields remain authoritative so existing automation is not broken.
- Added an explicit `wuphf secret migrate-config` path that dry-runs by default, writes only with `--write`, and clears plaintext only with `--clear-config --confirm-clear-plaintext`.
- Still pending: decide whether runtime resolvers should optionally read from the encrypted store after env vars and before plaintext config.
- Keep plugin execution out of process until a sandbox and capability model is designed for DunderIA.

## Phase 4 - Release And Operations Maturity

- Add a release-smoke matrix for `npx wuphf`, installer scripts, and public starter-kit packaging.
- Add CODEOWNERS-backed review expectations for release workflows, config, topology protection, dependency locks, and starter-kit distribution.
- Add behavior evals that can run locally without provider keys, then optional provider-backed evals for real agent prompts.
- Added a Windows-first `scripts/dev-services.ps1` helper for `list` and dry-run/forced `stop` of local broker/web development processes.

## Deliberately Not Ported Directly

- Paperclip's DB-backed issue schema: DunderIA already has local broker state and task contracts.
- Paperclip's full plugin worker runtime: useful but too broad without a DunderIA sandbox/capability design.
- Company import/export topology automation: conflicts with DunderIA protected topology unless explicitly requested by the user.
- Paperclip's web UI architecture: DunderIA has a separate React app and TUI surface with existing tests.
