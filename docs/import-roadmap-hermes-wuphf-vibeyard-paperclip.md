# Import Roadmap: Hermes, WUPHF, Vibeyard, Paperclip

Snapshot date: 2026-05-05

Refresh pass: 2026-05-05, after local commits `29e210a` and `6e9cd52`.

This note captures what is worth importing into MaestrIA from four adjacent systems:

- Hermes Agent: https://github.com/NousResearch/hermes-agent
- WUPHF upstream: https://github.com/nex-crm/wuphf
- Vibeyard: https://github.com/elirantutia/vibeyard
- Paperclip: https://github.com/paperclipai/paperclip

The goal is not to copy runtimes. The useful move is to extract product and architecture patterns that strengthen MaestrIA's local-first office model: broker push, fresh per-turn runners, scoped MCP, isolated worktrees, protected topology, explicit task contracts, and operator-visible evidence.

## Boundaries

- Do not replace the local broker/task/worktree model with another runtime.
- Do not mutate `company.json`, `broker-state.json`, onboarding/bootstrap state, saved workflows, rosters, channels, or blueprints without explicit current authorization.
- Keep Nex, GBrain, Telegram, Composio, browser automation, desktop shells, remote sandboxes, and other integrations optional.
- Prefer dry-run previews, evidence gates, and confirmation-gated apply paths before any mutation.
- Prefer MaestrIA-native Go/runtime patterns over wholesale TypeScript, Python, Electron, Postgres, or cloud control-plane ports.
- Keep public bootstrap material separate from private local runtime profiles. Never copy secrets, raw sessions, broker state, memory snapshots, or private database helpers into the active runtime.

## Executive Read

Short term should focus on visibility and decision quality:

- Vibeyard's strongest near-term contribution is session ergonomics: visible status, cost/context pressure, session inspector, task-to-session continuity, and smart alerts.
- Paperclip's strongest near-term contribution is stricter operational accountability: liveness history, budget gates, outcome evidence, activity trace, and review/rollback surfaces.
- Hermes' strongest near-term contribution is learning and skill hygiene: skill provenance, bounded memory, recall quality, command registry drift checks, and safe promotion from evidence to reusable skills.
- WUPHF upstream's strongest near-term contribution is productization: a git-native wiki/notebook direction, onboarding language, release/install polish, and file-over-app knowledge posture.

Medium term should turn previews into governed workflows:

- A local git-backed wiki or knowledge article layer, with linting and citations.
- A stronger session inspector that combines timeline, tool use, cost, context, liveness, artifacts, and next action.
- A plugin/adapter sandbox model with capabilities, secret refs, host services, and audit trail.
- Recall and learning apply paths that are useful but never silently rewrite future behavior.

Long term is platform work:

- Remote execution environments and sandbox fleets.
- Skill/plugin marketplace with signatures, provenance, drift checks, and security scanning.
- Optional desktop/IDE mode with richer terminal panes and browser inspection.
- Multi-company or multi-human control plane only if the product direction actually requires it.

## What Is Already Absorbed Locally

Hermes-inspired surfaces already present or planned in `docs/hermes-agent-absorption-plan.md`:

- `/learning/candidates`
- progressive skill file preview via `/skills/{name}/files-preview`
- `/skills/trust` and `/skills/provenance-preview`
- `/memory/curation-preview`
- `/recall/search-preview`
- `/scheduler/skill-preview`
- `/toolsets/profile-preview`
- `/commands/manifest` with `web`, `tui`, and `all` surface filters
- `/runtime/execution-environments-preview`

Paperclip-inspired surfaces already present or planned in `docs/paperclip-absorption-plan.md`:

- runtime contract, stable evals, task liveness classifier, secret store, secret audit, dev services
- outcome evidence contract and task completion blockers
- plugin/capability metadata, adapter registry, adapter checks, adapter actions
- queue metadata, work queues, intake queues, plan revisions, deep planning preview
- learning promotion, knowledge index, skill metadata preview, capability-upgrade preview
- org proposals, governance history, replay, rollback packages
- execution locks, heartbeats, work products, scheduler concurrency/catch-up
- runtime doctor, smoke, release readiness, release artifacts, worktree preview
- operator overview, triage, runbook, cleanup preview, apply-preview
- plugin runtime inventory, workspace inventory, outcomes taxonomy
- agent sessions and execution trace
- browser inspection artifacts for task evidence: page URL, selector, element text, viewport, and screenshot path
- git-native knowledge wiki preview and lint via `/knowledge/wiki-preview` and `/knowledge/wiki-lint`
- plugin sandbox readiness preview via `/plugins/sandbox-preview`, including a built-in health-only `worker:noop-health` candidate; action-running worker execution remains blocked
- smart operator alerts via `/operator/alerts`
- learning candidate diff preview via `/learning/candidates/diff-preview`
- command manifest drift checks for manual/manifest alignment
- budget/context guardrail preview via `/budget/context-preview`
- session inspector liveness history and provider-independent `normalized_status` in `/agent-sessions` and `/execution-trace`
- Docker/SSH execution adapter preview hardening via `/runtime/execution-environments-preview`: blocked future adapters now expose binary presence, missing governance policies, and next steps without starting backends or connecting remotely
- multi-human permissions lite via `/humans/permissions-preview`: read-only viewer/channel capability snapshots separate read, answer, review, approve, and blocked topology mutation without creating shared users or changing channels
- skill/plugin marketplace manifest design via `/marketplace/manifest-preview`: read-only signed-manifest shape, source hash drift, capability drift, missing policy checks, and disabled install/update flags before any marketplace apply surface exists
- git-native team wiki promotion preview via `/knowledge/wiki-promotion-preview`: task/learning evidence becomes a reviewable markdown diff with lint findings and reviewed-commit-only guidance, without writing files or shared memory
- browser inspection handoff via `/browser/inspection-handoff-preview`: recorded `browser_inspection` artifacts become ready/review handoff packets with URL, selector, observed text, viewport, screenshot, and prompt context without opening or embedding a browser
- remote sandbox fleet preview via `/runtime/remote-sandbox-preview`: Docker, SSH, and self-hosted worker candidates expose missing governance policies, binary checks, risk signals, and disabled execution before any remote backend exists
- Desktop/IDE mode preview via `/integrations/desktop/preview`: optional desktop shell, tray, and browser-lab surfaces are visible as wrappers around web Studio without launching Electron or mutating topology
- multi-company control-plane preview via `/companies/control-plane-preview`: export/isolation contracts, blocked mutations, missing policies, and scrubbed preview items are visible without creating, importing, switching, or deleting companies

This means the next import wave should avoid duplicate "more dashboards". The better target is to connect existing surfaces into clearer operator decisions.

## Latest Upstream Refresh: 2026-05-05

The upstream repositories moved again after this roadmap wave was emptied. The new material does not invalidate the absorbed previews, but it does create a fresh implementation queue.

New Hermes Agent signals:

- API server streaming hardening: SSE token batching, large tool-call argument trimming, request-size handling, and error chunks for mid-stream crashes.
- Hindsight/memory dedupe: stable session-scoped document IDs plus `update_mode=append` when the backing API supports it.
- Kanban runtime safety: `kanban.max_spawn` concurrency limits, run-id lifecycle guards, and metadata handoff round-trip tests.
- Locale expansion is useful product polish but not a MaestrIA architecture import by itself.

New WUPHF upstream signals:

- Rich wiki editor MVP behind a per-article toggle, with markdown round-trip tests, wikilink preservation, code-region safety, draft/conflict behavior, and accessibility fixes.
- Human-share/join UX now surfaces the host display name so invited humans understand whose office they joined.
- Human-share transport keeps moving toward an office-bound transport model. This remains adjacent to MaestrIA's protected topology and multi-human previews.

New Vibeyard signals:

- Customizable project overview page with draggable/resizable widgets, including readiness, provider-tools, GitHub PRs, and GitHub issues.
- Drag-and-drop from file tree/Finder into session prompt, making file context handoff lower friction.
- Instruction-file discovery now recognizes `CLAUDE.md` more reliably.
- Default new-project layout changed toward tabs with swarm off, reinforcing that dense multi-session views should be optional rather than default.

New Paperclip signals:

- Routine revision history and restore flow: append-only routine revisions, conflict-aware saves, history tab, dirty-edit blocking, and restore confirmation.
- Per-adapter sandbox install commands: sandbox leases can run adapter-specific install steps before resolvability/hello probes, with structured non-throwing checks.
- Expanded plugin host surface: plugin-managed database namespaces, local folders, managed agents/routines, scoped plugin APIs, UI bridge, and reusable plugin UI pieces.
- Gemini CLI v0.38 stream-json parser compatibility across server, UI, and CLI formatter.

Fit for MaestrIA:

- Strong fit: routine/scheduler revision safety preview; wiki editor readiness/round-trip preview; sandbox install-command governance; provider/CLI wire-format compatibility checks.
- Medium fit: project overview widget manifest preview and GitHub PR/issue widgets, if kept read-only and driven by linked repos.
- Defer: actual rich editor, routine restore apply, plugin-managed database namespaces, sandbox installs, human-share transport mutation, and file drag/drop into agent prompts until the preview contracts are clear.

## Source Notes

### Hermes Agent

Observed strengths:

- Built-in learning loop: skills are created and improved from experience.
- Agent-curated memory, periodic reminders to persist knowledge, and recall over past sessions.
- FTS5-style session search with summarization.
- Shared command behavior across CLI and messaging gateway.
- Broad messaging gateways: Telegram, Discord, Slack, WhatsApp, Signal, Email, Home Assistant.
- Scheduler with delivery to platforms.
- Subagents and parallel workstreams.
- Multiple execution backends: local, Docker, SSH, Daytona, Singularity, Modal.
- Toolsets and command approval/security documentation.
- Migration tooling from OpenClaw with dry-run and presets.

Best fit for MaestrIA:

- Learning candidate apply lifecycle with provenance and review.
- Skill source/provenance/drift/security lifecycle.
- Bounded private memory with frozen prompt snapshots.
- Recall summarization with cost visibility.
- Command manifest as source of truth for help, palette, slash commands, and docs drift.
- Execution environment abstraction as governed adapter, not a new default runtime.

Avoid:

- Replacing MaestrIA's fresh-runner broker model with Hermes' session-centric loop.
- Broad gateway expansion as a core requirement.
- Direct skill writes by agents without review.
- Python path/layout/runtime copy.
- Remote sandboxes before secrets, doctor checks, adapter policies, and approval model are strong.

### WUPHF Upstream

Observed strengths:

- One-command product story: `npx wuphf`.
- Office metaphor with visible agents, channels, tasks, and work.
- Fresh per-turn runners, scoped MCP, push broker, isolated worktrees.
- Agent packs and optional integrations.
- Shared memory direction has evolved into notebooks plus a git-native team wiki.
- Wiki design emphasizes file-over-app: local git repo, markdown articles, typed facts, append-only fact logs, synthesized briefs, cited lookup, and linting for contradictions/staleness/broken references.
- Onboarding and README text are strong product artifacts.
- Forking/branding guidance and release/product polish are part of the public posture.

Best fit for MaestrIA:

- Knowledge articles as readable files with provenance, not opaque memory blobs.
- Notebook-to-wiki promotion flow: private working notes first, durable shared article only after review.
- Wiki lint as an operator surface over contradictions, stale claims, missing citations, broken backlinks, and orphan pages.
- Public install/release smoke around `npx wuphf`, startup, first office view, and provider setup.
- Product copy and onboarding checklists that reduce first-run ambiguity.

Avoid:

- Treating Nex as required.
- Importing destructive state-reset semantics. Local MaestrIA must preserve protected office state unless explicitly authorized.
- Copying upstream topology/agent packs into an existing office without current permission.
- Installing or syncing wiki/memory backends automatically.

### Vibeyard

Observed strengths:

- Desktop IDE built around AI coding agents, not generic chat.
- Kanban task board where cards can spawn or resume CLI sessions.
- Multiple sessions per project, each in its own PTY.
- Swarm/grid view for many sessions.
- Real-time cost, token usage, context window monitoring.
- Session inspector with timeline, cost breakdown, tool-use stats, and context health.
- Session status indicators: working, waiting, input needed, completed.
- Smart alerts for missing tools, context bloat, and session health.
- Session resume after restart.
- Embedded browser tab with DOM inspection and selector-aware editing context.
- Provider abstraction for Claude Code, Codex CLI, Gemini CLI, and future CLIs.
- Hook event model mapping CLI events into session status and inspector logs.
- Electron app packaging across macOS, Linux, and Windows.

Best fit for MaestrIA:

- Better per-agent session inspector in Studio, using existing `/agent-sessions`, `/execution-trace`, `/activity`, `/usage`, `/resume-pack`, `/runtime/doctor`, and liveness data.
- A status transition model that separates `waiting`, `working`, `input_needed`, `completed`, `failed`, `interrupted`, and `stale`.
- Smart alerts that convert runtime signals into one-line operator actions.
- Task board ergonomics: search, tags, per-task run/resume/focus affordances, and automatic done review only when outcome evidence is satisfied.
- Browser inspection handoff for frontend tasks, likely through Playwright/selector context rather than embedding a full browser immediately.
- Provider capability display per session: which provider supports resume, system prompt, cost parsing, tool hooks, browser handoff, etc.

Avoid:

- Rebuilding MaestrIA as an Electron-first IDE before validating the web Studio improvements.
- Making PTY persistence the core runtime. MaestrIA's fresh-runner model is a strength.
- Read-write P2P sharing before auth, permissions, and audit are mature.
- Provider-specific hook assumptions that only work for one CLI.

### Paperclip

Observed strengths:

- Company control plane model: org chart, goals, budgets, governance, tasks, routines, plugins, secrets, activity, portability.
- Bring-your-own-agent orientation.
- Goal alignment: tasks trace back to mission/project context.
- Heartbeats and routines with tracked issues.
- Atomic execution and task checkout to avoid double work.
- Budget enforcement, not just visibility.
- Persistent agent state across heartbeats.
- Runtime skill injection.
- Governance with rollback.
- Portable company templates with secret scrubbing and collision handling.
- Multi-company isolation.
- Out-of-process plugin workers with capability-gated host services, job scheduling, tool exposure, and UI contributions.

Best fit for MaestrIA:

- Finish turning "observability" into "control": budget limits, task checkout invariants, liveness history, and rollback packages.
- Plugin runtime as inventory first, sandbox later.
- Company portability as dry-run export/import with topology protections.
- Stronger goal ancestry and outcome classification in task prompts.
- Multi-human permissions if the office becomes shared by more than one operator.

Avoid:

- Direct Postgres/DB-backed issue schema port.
- Full plugin worker execution before sandbox/capability/secret model.
- Automatic company import/export topology mutation.
- Multi-company complexity before single-office workflows are crisp.

## Short-Term Wave: Absorbed

The short-term wave has been absorbed into MaestrIA as read-only or confirmation-gated surfaces. Do not keep re-importing these as new work unless a later task asks to deepen them.

- Studio session inspector: `/agent-sessions`, `/execution-trace`, liveness history, normalized session/trace status, Studio panels.
- Smart operator alerts: `/operator/alerts`, action labels, runtime/task/usage/workspace signals, read-only.
- Learning candidate apply plan: `/learning/candidates/diff-preview`, virtual skill files, duplicate/risk/provenance metadata, no automatic writes.
- Knowledge wiki feasibility and preview: `/knowledge/wiki-preview` and `/knowledge/wiki-lint`, read-only sourced markdown projection.
- Command manifest drift check: `/commands/manifest-drift` warns about manual/manifest mismatches.
- Budget and usage guardrail preview: `/budget/context-preview`, read-only budget/context risk before enforcement.
- Plugin no-op worker sandbox: `/plugins/sandbox-preview` now includes `worker:noop-health` with manifest metadata, static health check, filesystem `none`, network `none`, and no secret refs; it cannot execute plugin actions, shell commands, network calls, or filesystem writes.
- Docker/SSH execution adapter preview hardening: `/runtime/execution-environments-preview` now reports Docker/SSH binary presence, required/missing governance policies, policy checks, and next steps while keeping both adapters `blocked`.
- Multi-human permissions lite: `/humans/permissions-preview` now reports viewer/channel capability snapshots while keeping topology mutation blocked and avoiding user/channel creation.
- Skill/plugin marketplace manifest design: `/marketplace/manifest-preview` now reports preview-only manifest signatures, trust/source hash drift, capability additions, review policies, and explicit `install_enabled:false` / `update_enabled:false`.
- Git-native team wiki promotion design: `/knowledge/wiki-promotion-preview` now reports proposed wiki file paths, markdown, git-style diffs, commit messages, required reviews, lint findings, and `reviewed_commit_only:true` without persisting wiki files or shared memory.
- Browser inspection handoff hardening: `/browser/inspection-handoff-preview` now packages existing browser inspection artifacts into ready/review handoffs with missing-field checks, risk signals, and prompt context, without launching a browser or creating new evidence.
- Remote sandbox fleet first design: `/runtime/remote-sandbox-preview` now reports Docker, SSH, and self-hosted worker candidates with required/missing policies, binary checks, risk signals, next steps, and `execution_enabled:false` without starting any remote backend.
- Desktop/IDE mode first design: `/integrations/desktop/preview` now reports optional desktop shell, tray shell, and browser-lab surfaces, readiness checks, canonical surface, and topology-safety signals without opening Electron.
- Multi-company control plane first design: `/companies/control-plane-preview` now reports the current company snapshot, scrubbed export items, isolation contracts, blocked mutations, required/missing policies, and disabled apply/topology mutation flags without changing `company.json`, `broker-state.json`, agents, or channels.
- Routine revision safety first design: `/scheduler/revisions-preview` now reports scheduler jobs, revision/restore readiness gaps, missing policies, blocked revision actions, and `restore_enabled:false` without writing history or restoring routines.
- Wiki editor readiness first design: `/knowledge/wiki-editor-preview` now reports source/rich editor mode readiness, round-trip/wikilink/code/draft/conflict/accessibility checks, and `editor_enabled:false`.
- Sandbox install-command governance first design: `/runtime/remote-sandbox-preview` now includes install-command policy, disabled install execution, preview-only command guidance, missing policy checks, and `install_command_disabled` risk.
- Provider stream compatibility first design: `/providers/compatibility-preview` now reports Codex, Gemini, Claude Code, and Ollama wire-format assumptions, parser risks, missing fixture tests, and `mutation_enabled:false`.
- Project overview widgets first design: `/studio/project-overview-preview` now reports a read-only widget manifest for readiness, provider tools, GitHub PRs/issues, workspaces, and active tasks with widget mutation and external GitHub queries disabled.
- File/context handoff first design: `/files/context-handoff-preview` now reports task artifact, workspace, and worktree references with `content_read_enabled:false`, missing file-scope/secret-scan/prompt-injection policies, and blocked prompt injection actions.

## Medium-Term Plan: 1 To 2 Months

These need more design because they touch persistence, security, or workflow semantics.

### 1. Git-Native Team Wiki

Sources: WUPHF upstream, Hermes memory.

Shape:

- optional local git repo for shared knowledge articles
- private per-agent notebooks remain separate
- promotion flow: notebook or task evidence -> proposed wiki diff -> review -> commit
- article pages support backlinks, sources, history, raw markdown, and cited lookup
- lint surface for contradictions, stale claims, missing sources, orphans, broken refs

Why medium:

- First slice absorbed: `/knowledge/wiki-preview`, `/knowledge/wiki-lint`, and `/knowledge/wiki-promotion-preview` now project knowledge into sourced markdown, lint it, and build a reviewable git diff without writing files, committing, or mutating shared memory.
- Remaining work is an actual optional local wiki repo, reviewed apply/commit workflow, article history, cited lookup, backlinks, and stronger contradiction/staleness checks.

Validation:

- unit tests for article parser/lint
- access filtering tests
- secret/prompt-injection scan tests
- manual docs update if user-visible

### 2. Session Health State Machine

Sources: Vibeyard hooks, Paperclip liveness.

Shape:

- normalize status transitions across providers and runtime paths
- preserve provider-specific events as trace details
- derive sticky completed/blocked states carefully
- expose stale/interrupted/permission-needed states to the operator

Why medium:

- First slice absorbed: `/agent-sessions`, `/execution-trace`, and session observability now expose provider-independent `normalized_status` values derived from raw task/session/liveness states.
- Remaining work is a durable transition history that handles retries, interrupts, provider quirks, and old events.

### 3. Governed Plugin Worker Sandbox

Sources: Paperclip, Hermes skills/security.

Shape:

- out-of-process plugin workers only after capability contracts exist
- plugins declare host services, secret refs, filesystem scope, network needs, scheduler hooks, UI contributions
- runtime exposes health and action requests, not arbitrary command execution
- dangerous capability expansions require review

Why medium:

- First slice absorbed: `/plugins/sandbox-preview` now includes a built-in health-only `worker:noop-health` class with manifest metadata, static health check, filesystem `none`, network `none`, and no secret refs.
- Remaining work is action-running out-of-process workers with capability-gated host services and reviewable expansion paths. Execution remains the risky part.

First implementation:

- already done for the no-op worker; do not deepen this into arbitrary shell or plugin-action execution without a separate sandbox design.

### 4. Execution Environment Adapters

Sources: Hermes.

Shape:

- local worktree remains default
- Docker/SSH start as blocked or dry-run adapters
- doctor explains readiness and risk
- every remote execution requires explicit workspace, secret policy, cleanup policy, and audit trail

Why medium:

- First slice absorbed: Docker/SSH remain blocked future adapters, but `/runtime/execution-environments-preview` now explains local binary availability and missing workspace, secret, network, cleanup, audit, approval, host, and key policies.
- Remaining work is actual governed execution adapter design. It is useful, but changes failure modes and security posture.

### 5. Browser Inspection Handoff

Sources: Vibeyard.

Shape:

- for frontend tasks, capture page URL, selected element, selector, text content, screenshot path, and viewport
- feed that as task evidence/context
- prefer Playwright or browser tool integration before embedding a browser in Studio

Why medium:

- First slice absorbed: task artifacts already accept `browser_inspection`, and `/browser/inspection-handoff-preview` now packages those records into handoff context with URL, selector, observed text, screenshot path, viewport, missing fields, and risk signals.
- Remaining work is direct Browser Lab capture into task artifacts, richer selector history, and apply paths that attach fresh browser evidence after an actual visual verification run.

### 6. Multi-Human Permissions Lite

Sources: Paperclip, WUPHF share.

Shape:

- keep single local operator as default
- model viewer/channel capability snapshots first
- add read-only vs approve vs mutate permissions only where needed
- never expose topology mutation to invited users without explicit role and audit

Why medium:

- First slice absorbed: `/humans/permissions-preview` exposes read-only viewer/channel capability snapshots and shows topology mutation as blocked.
- Remaining work is any real multi-operator invitation, auth, role assignment, or shared-office mutation path. WUPHF upstream has co-founder sharing and Paperclip has multiple users; MaestrIA should only import those once trust boundaries are crisp.

## Long-Term Plan: 2 Months Plus

These are valuable only after the short/medium control surfaces are reliable.

### 1. Remote Sandbox Fleet

Sources: Hermes, Paperclip.

Possible shape:

- Docker, SSH, e2b-like, Daytona/Modal-like, or self-hosted workers
- environment health in doctor
- per-task environment choice
- secret scoping and workspace cleanup
- cost and quota accounting

Reason to defer:

- First slice absorbed: `/runtime/remote-sandbox-preview` turns Docker, SSH, and self-hosted worker ideas into a blocked governance checklist with execution disabled.
- Remaining work is actual remote execution, task-level environment selection, fleet health in doctor, secret/workspace cleanup enforcement, cost/quota accounting, and approval-gated apply.

### 2. Skill And Plugin Marketplace

Sources: Hermes Skills Hub, Paperclip plugins.

Possible shape:

- signed skill/plugin manifests
- source trust tiers
- installed hash vs upstream hash
- security scan on install and update
- drift preview
- explicit review for capability expansion

Reason to defer:

- First slice absorbed: `/marketplace/manifest-preview` defines the signed manifest and drift-review contract without installing or updating skills/plugins. It reuses trust, provenance, capability-upgrade, adapter config, and sandbox signals while keeping install/update disabled.
- Remaining work is a real marketplace source, trusted signature verification, security scanning on downloaded content, review-gated capability expansion, and an explicit apply path.

### 3. Desktop Or IDE Mode

Sources: Vibeyard.

Possible shape:

- optional desktop shell around MaestrIA Studio
- native notifications, tray, terminal panes, local browser inspection, provider install checks
- web Studio remains canonical

Reason to defer:

- First slice absorbed: `/integrations/desktop/preview` exposes desktop shell, tray, and browser-lab readiness as optional wrappers around the canonical web Studio.
- Remaining work is richer terminal panes, native notifications beyond tray basics, local browser inspection capture, provider install checks, packaging polish, and explicit launch UX hardening.

### 4. Multi-Company Control Plane

Sources: Paperclip.

Possible shape:

- one deployment can host multiple isolated offices
- company export/import is secret-scrubbed and collision-aware
- governance history and rollback are company-scoped

Reason to defer:

- First slice absorbed: `/companies/control-plane-preview` keeps multi-company at export/isolation preview level with topology mutation and apply disabled.
- Remaining work is true isolated state roots, scrubbed export package writing, collision handling, company-scoped governance/rollback, restore flows, and explicit human-reviewed apply.

### 5. Autonomous Organizational Learning

Sources: Hermes, WUPHF wiki, Paperclip roadmap.

Possible shape:

- repeated evidence/correction/failure patterns propose skill or wiki updates
- evals verify the proposed learning before promotion
- operator confirms the patch
- regression checks catch stale or harmful learnings later

Reason to defer:

- The safe path is incremental: candidates, diffs, apply gates, lint, then automation.

## Remaining Candidate Matrix

No remaining candidates from the 2026-05-05 upstream refresh. The refreshed wave has been absorbed as read-only previews and should not be re-imported unless a later upstream scan finds new material or the user asks to deepen an apply path.

## Next Implementation Queue

Empty after this pass.

## Explicit Non-Imports

Do not import in the next wave:

- Hermes' full Python runtime, native path layout, broad gateway expansion, or automatic agent-authored skill mutation.
- WUPHF upstream's destructive `shred` semantics or any topology/agent-pack changes without explicit authorization.
- Vibeyard's Electron-first architecture, persistent PTY runtime as core, or read-write P2P sharing.
- Paperclip's Postgres issue schema, automatic company import/export mutation, full plugin worker execution, or multi-company isolation layer.

## Validation Guidance For Future Work

For each implementation slice:

- Start with a dry-run or read-only endpoint when the feature touches memory, skills, topology, plugins, adapters, budgets, or execution environments.
- Add focused Go tests for backend composition/access filtering.
- Add web build or component checks for UI changes.
- Update `docs/MANUAL.md` and regenerate `docs/MANUAL.pdf` only when behavior, commands, UI, runtime, integrations, security posture, recovery, or operational workflow changes.
- For docs-only analysis like this file, no manual/PDF update is needed.

## References Reviewed

- Hermes Agent README and repo tree: https://github.com/NousResearch/hermes-agent
- Hermes docs index and CLI/messaging reference links from README: https://hermes-agent.nousresearch.com/docs
- WUPHF upstream README and repo tree: https://github.com/nex-crm/wuphf
- WUPHF wiki design note: https://raw.githubusercontent.com/nex-crm/wuphf/main/DESIGN-WIKI.md
- WUPHF architecture note: https://raw.githubusercontent.com/nex-crm/wuphf/main/ARCHITECTURE.md
- Vibeyard README and repo tree: https://github.com/elirantutia/vibeyard
- Vibeyard hooks/session-state note: https://raw.githubusercontent.com/elirantutia/vibeyard/main/HOOKS.md
- Vibeyard architecture notes in CLAUDE.md: https://raw.githubusercontent.com/elirantutia/vibeyard/main/CLAUDE.md
- Paperclip README and repo tree: https://github.com/paperclipai/paperclip
- Hermes commits reviewed on refresh: https://github.com/NousResearch/hermes-agent/commit/3188e63b05a1902baecfcd7c30da3301d74b8737, https://github.com/NousResearch/hermes-agent/commit/3082fa0829e0df4ce682358481fb59275b31a46e, https://github.com/NousResearch/hermes-agent/commit/f0d278412f8c14e94a11678be424f6a6ddb79fa2, https://github.com/NousResearch/hermes-agent/commit/56b4795115e309b8d65bc68729fc591e90e6ffaa
- WUPHF commits reviewed on refresh: https://github.com/nex-crm/wuphf/commit/ddcf7e9c46f18f8e761010e6ccef5074e5a26c61, https://github.com/nex-crm/wuphf/commit/91f9159a9fb5c5c2e8187f6963fbc65b7019d61f, https://github.com/nex-crm/wuphf/commit/d4848ae7a3064357327e3fe020cfb511afc1fade
- Vibeyard commits reviewed on refresh: https://github.com/elirantutia/vibeyard/commit/21c0f5fa0c6669443855d8d4532201954e97ba40, https://github.com/elirantutia/vibeyard/commit/4d31ca2427d6a4c896ded47b4461ae52ecb2cfe7, https://github.com/elirantutia/vibeyard/commit/17c3218f3aa1f7aa5d6eb26f689a4c8880fa94f2
- Paperclip commits reviewed on refresh: https://github.com/paperclipai/paperclip/commit/d6d7a7cea645ed8035ae2c5aa953c22e26601159, https://github.com/paperclipai/paperclip/commit/9578dc3da733907b818b0ec40b0b0df4d56e84c7, https://github.com/paperclipai/paperclip/commit/3c73ed26b514144b831f02361de809008742194d, https://github.com/paperclipai/paperclip/commit/ea7f53fd7dc2a438dac45b4447fb06de238373f7
- Local Hermes absorption plan: `docs/hermes-agent-absorption-plan.md`
- Local Paperclip absorption plan: `docs/paperclip-absorption-plan.md`
- Local MaestrIA manual: `docs/MANUAL.md`
