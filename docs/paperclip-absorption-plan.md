# Paperclip Absorption Plan

This plan captures the Paperclip ideas that are worth bringing inta MaestrIA while preserving MaestrIA's local-first Go runtime and protected office topology.

## Non-negotiable Boundaries

- Do not mutate `company.json`, `broker-state.json`, onboarding state, saved workflows, agent rosters, channel lists, or blueprints that change rosters/channels without explicit current user authorization.
- Prefer MaestrIA-native Go/runtime patterns over copying Paperclip's TypeScript database/control-plane architecture.
- Port behavior contracts, checks, and narrow runtime utilities first; only port heavier subsystems after migration and rollback paths exist.
- Keep Nex, GBrain, Telegram, Composio, and external adapters optional.

## Phase 1 - Absorbed Now

- Runtime contract: codify wake paths, task states, ownership, blockers, recovery, usage visibility, skills, adapter boundaries, and operator handoff in `RUNTIME_CONTRACT.md`.
- Workflow skills: add MaestrIA-adapted plan-to-tasks, doc-maintenance, and contribution-report skills to the starter kit.
- Contribution governance: require a clearer PR story, verification, risks, model disclosure, and UI evidence when relevant.
- Sensitive-area ownership map: define CODEOWNERS patterns for release, workflows, config, topology/state code, starter-kit assets, lockfiles, and runtime broker surfaces.
- Public-release scanner: add a local script that checks tracked files for dynamic/local forbidden tokens before packaging.
- Stable test runner: add a PowerShell runner that isolates `WUPHF_RUNTIME_HOME`, task logs, Go cache, and temp directories for repeatable local test runs.
- Agent behavior eval seed: document the first prompt/eval scenarios for task pickup, approval gates, blockers, company boundaries, and durable state changes.
- Log-read hardening: expose paginated task-log reads with optional SHA-256 so large logs can be inspected without loading the whole file.

## Phase 2 - Runtime Liveness Patch Absorbed

- Added a MaestrIA-native run liveness classifier with states equivalent to: `advanced`, `completed`, `blocked`, `failed`, `empty_response`, `plan_only`, and `needs_followup`.
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
- Keep plugin execution out of process until a sandbox and capability model is designed for MaestrIA.

## Phase 4 - Release And Operations Maturity

- Add a release-smoke matrix for `npx wuphf`, installer scripts, and public starter-kit packaging.
- Add CODEOWNERS-backed review expectations for release workflows, config, topology protection, dependency locks, and starter-kit distribution.
- Add behavior evals that can run locally without provider keys, then optional provider-backed evals for real agent prompts.
- Added a Windows-first `scripts/dev-services.ps1` helper for `list` and dry-run/forced `stop` of local broker/web development processes.

## Phase 5 - Operator Outcome Contract

- Added an operator completion contract for tasks that have an expected outcome, worktree/live execution, or a delivery-oriented type.
- Completion now requires outcome evidence or a durable artifact before the task can be marked done.
- Rejected completion attempts emit `task_completion_blocked`; accepted completions emit `task_completed`, making the operator audit trail closer to Paperclip's explicit progress ledger.
- `/tasks` now decorates operator-visible tasks with `completion_evidence_required`, `completion_evidence_satisfied`, `completion_blocker`, `goal_path`, and `goal_summary`.
- The task detail UI shows the completion contract and a compact "why this exists" chain so agents and humans can see the outcome/context before closing work.
- The company-goal context is read only from the local config file on this hot path, avoiding cloud fallback latency in the task board.

## Phase 6 - Plugin, Queue, Planning, And Learning Slice

- Added MaestrIA-native plugin/capability metadata to skills: `plugin_id`, `plugin_kind`, static `capabilities`, and health fields.
- Skill invocation now honors capability-gated skills while preserving legacy skills with no declared capabilities.
- `/skills` can filter by capability or plugin id, and capability registration emits an audit action.
- Operator tasks now expose queue metadata beyond `queue_key`: label, reason, priority, and SLA anchor, making work queues more explicit without replacing the existing board.
- Plan revisions now carry status and approval metadata; task detail can approve the latest plan and operator task payloads expose `plan_required`, `plan_status`, `plan_blocker`, and `latest_plan_summary`.
- Completed tasks with evidence, artifacts, or plans can be promoted into reusable learning skills via `promote_learning`, backed by `dunderia-learning` capability metadata.
- Budget hard-stops were intentionally not expanded in this phase per operator direction.

## Phase 7 - Adapter Registry, Organization Proposals, And Release Contract

- Added a MaestrIA-native `/adapters` registry with builtin capability cards for the local broker, fresh runner, scoped MCP, GitHub publication, and learning registry.
- Custom adapters can be upserted with provider, kind, health, config reference, and normalized capability metadata; the registry is filterable by kind, provider, status, or capability.
- Added `/org-proposals` as a self-organization object model. Proposals can be approved or rejected, but approval records authorization only and does not mutate protected members, channels, routing, or topology.
- Added `/ceo/convert` so a CEO conversation can become an operational task, decision, or human request with audit trail back to the conversation context.
- Added the `release-pr-checklist` task template, requiring build/test evidence, diff hygiene, PR/release summary, risk statement, and rollback notes.
- The Skills surface now groups adapters, self-organization proposals, and reusable skills so capabilities are visible without adding another crowded navigation tab.
- Multiple human users and permission layers were intentionally not ported in this phase per operator direction.

## Phase 8 - Atomic Execution, Work Products, Routines, And Activity Ledger

- Added per-task execution locks with acquire, heartbeat, and release actions. Locks carry run id, owner, TTL, heartbeat, and expiry metadata so runners can avoid double-work.
- Enriched task artifacts into stronger work products with result role, preview URL, MIME type, checksum, validation metadata, and state.
- Added scheduler concurrency and catch-up policy fields: `skip_if_running`, `allow_parallel`, `enqueue`, `run_once`, `skip_missed`, and `replay`.
- Scheduler runtime now marks due jobs as running before processing so concurrency metadata is visible and `skip_if_running` jobs are withheld while already active.
- Added `/activity` as a unified activity ledger over actions, decisions, signals, watchdogs, routines, and work products.
- The Artifacts surface now reads the unified ledger for recent activity and shows a compact Results/work-products section.
- Budget hard-stops were intentionally not expanded in this phase per operator direction.

## Phase 9 - Managed Runtime Doctor, Instance Guard, And Scoped Human Surfaces

- Added a live runtime doctor snapshot exposed at `/runtime/doctor` and embedded into the Studio summary.
- Runtime doctor checks now report duplicate web runtime processes, expected MCP helper processes, compiled web asset source/hash, missing/stale web build symptoms, and runtime status (`ok`, `degraded`, or `blocked`).
- Added a `wuphf doctor` CLI command that combines setup capability checks with the live runtime doctor when the local broker is reachable.
- Added a lightweight instance-quarantine guard: running from task worktrees/temp clones, `WUPHF_RUNTIME_HOME` pointing at temporary worktrees, and duplicate active task worktrees now surface as runtime doctor signals instead of silently contaminating the office.
- Human request surfaces are scoped by channel by default; global surfaces such as the Requests app/sidebar opt into `all_channels`. Blocking requests now block only their own channel's composer/API post path.
- These changes intentionally diagnose and gate behavior without mutating protected topology or broker state.

## Phase 10 - Planning, Knowledge, Queues, Reviews, And Template Preview

- Added `/planning/deep` as a dry-run hierarchical planning preview that validates planned tasks with the existing strict task-plan rules, returns milestones, evidence gates, and review gates, and does not persist tasks.
- Added `/knowledge` as a local knowledge index over completed task evidence/artifacts/plans and promoted `dunderia-learning` skills, with channel access filtering and lightweight search.
- Added `/work-queues` as a real operational queue summary over current tasks, including priority ordering, owners, channels, queue reasons, and next work items.
- Added `/review/checklist` so task approvals can be inspected through a deterministic checklist for outcome evidence, deep plan readiness, review findings, human blockers, and structured review state.
- Added `/templates/preview` as a topology-safe starter/template dry-run. It reports would-create/would-reuse agents, channels, and skills while blocking protected topology mutations until explicit authorization exists.
- The Tasks surface now consumes `/work-queues` for a compact board summary and next-work strip, replacing the noisier local-only queue badges when the backend view is available.

## Phase 11 - Instance Hygiene, Smoke, Secrets, Backup, And Human Session

- Added runtime restart advice to the doctor. It flags missing/stale web build symptoms, web builds newer than the running process, and duplicate web runtimes with a clear restart step.
- Added `/runtime/worktree-preview` as a dry-run repair preview for task-worktree contamination and duplicate active worktrees. It proposes isolation/restart actions without mutating tasks, channels, agents, or state.
- Added `/security/secret-audit` and embedded the result in runtime doctor. It reports plaintext config secret field names and secret env presence without exposing values; `WUPHF_SECRET_STRICT=1` turns plaintext config secrets into a failing doctor check.
- Added `/backup/policy` and embedded backup policy in runtime doctor, exposing broker-state history retention, local snapshot counts/bytes, cloud provider status, and cloud backup runtime state.
- Added `/humans/session` as a read-only human identity/capability snapshot for the current viewer/channel, giving the UI a path toward multi-human permissions without changing protected topology.
- Added `/humans/permissions-preview` as a read-only viewer/channel capability matrix. It separates read, request-answer, task-review, action-approval, and topology-mutation capabilities while keeping `topology.mutate` blocked and avoiding user/channel creation.
- Added `/runtime/smoke` plus `wuphf doctor --smoke` for a cheap release/browser smoke contract covering doctor status, web dist, work queues, backup policy, and secret audit.
- Studio now shows restart advice, secret audit counts, backup policy summary, and human permission preview signals inside existing operator/diagnostic surfaces.

## Phase 12 - Resume Packs, Governance Trace, Template Risk, Operator Mode, And Skill Trust

- Added `/resume-pack` as a read-only task/agent handoff pack. It summarizes the current objective, goal path, queue status, plan/evidence state, execution lock, recent artifacts, warnings, and next steps so fresh runners can resume with less drift.
- Added `/governance/history` as a compact governance trace over organization proposals and governance-sensitive actions, including topology sensitivity and rollback guidance without applying any protected changes.
- Strengthened `/templates/preview` with duplicate/conflict detection, secret-reference warnings, required review labels, risk score/level, and rollback guidance while keeping the endpoint dry-run only.
- Added `/operator/overview` as a compact operator snapshot over health, blockers, next work, human requests, governance, skill trust, and the top resume pack.
- Added `/skills/trust` to score active skills/plugins using declared capabilities, plugin metadata, health, invocation capability, failed executions, and secret-like content indicators without exposing secret values.
- Studio now opens on an `Operador` tab that presents the compact overview first, leaving the fuller diagnostic cards under `Resumo`.
- Budget hard-stops remain intentionally unexpanded in this phase per operator direction.

## Phase 13 - Triage, Follow-Up Dedupe, Replay Dry-Run, Skill Migration Preview, And Release Readiness

- Added `/operator/triage` to classify operational debt as `noise`, `actionable`, `blocked_human`, `blocked_dependency`, or `environment`, so the compact operator surface can separate real work from background churn.
- Added a CEO/watchdog follow-up coalescing guard: if a new CEO follow-up targets the same channel/thread as an active follow-up, the existing task is updated instead of creating another duplicate lane.
- Added `/governance/replay` as a dry-run rollback/replay preview for governance-sensitive events, including required review labels and would-revert actions without mutating broker state.
- Added `/skills/metadata-preview` to suggest `plugin_id`, `plugin_kind`, `capabilities`, and `health_status` for legacy skills without applying the migration automatically.
- Added `/release/readiness` as a compact readiness score over runtime smoke, doctor, web dist, plaintext secrets, backup snapshots, worktree preview, and git status.
- These surfaces continue the Paperclip-style control-plane direction while preserving MaestrIA's local-first runtime and protected topology boundary.

## Phase 14 - Assisted Cleanup, Studio Readiness, Metadata Preview, And Runbook

- Added `/operator/noise-cleanup-preview` as a dry-run-only cleanup view for background follow-up tasks, related watchdogs, and scheduler entries. It reports safe vs review-required candidates without mutating broker state.
- Added `/operator/runbook` as an automatic operational checklist that combines triage, cleanup preview, and release readiness into concrete commands or read-only endpoints.
- Studio Operator now embeds release readiness, skill metadata preview, assisted cleanup, and runbook cards so these Paperclip-style control-plane checks are visible without adding more tabs.
- The skill metadata path remains preview-only; applying inferred metadata still requires a separate explicit implementation step.
- The cleanup path intentionally does not close tasks, resolve watchdogs, cancel scheduler jobs, or alter protected topology.

## Phase 15 - Confirmed Preview Apply, Channel Invariants, Studio Modes, And Release Gate

- Added `/operator/apply-preview` for explicit, audited application of safe previews. It requires `confirm=true`, `confirmation=APPLY_PREVIEW`, selected item ids, actor, and reason before any state is persisted.
- The apply path supports safe noise cleanup and skill metadata migration previews. It skips review-required cleanup candidates and returns a rollback plan in the response.
- Added channel-scope coverage for operator previews so global views cannot leak inaccessible channel candidates to scoped agents.
- Studio Operator now has `Operação` and `Diagnóstico` modes. Daily operation keeps next work, blockers, requests, and runbook visible; diagnostic checks move behind the second mode.
- Added `wuphf release-check` as a local release gate over focused Go tests, web build, runtime smoke, release readiness, and git status. The command supports `--json` and `--skip-build`.
- Budget hard-stops remain intentionally unexpanded in this phase per operator direction.

## Phase 16 - Evals, Capability Governance, Adapter Checks, Intake Queues, Release Artifacts, And Desktop Tray

- Added `wuphf evals` and `/evals/behavior` as a local behavior-contract runner for task pickup, blockers, evidence gates, channel isolation, preview confirmation, capability review, adapter checks, and intake triage.
- Added `/skills/capability-upgrade-preview` so inferred capability additions are visible with risk score, required review labels, and approval requirement before skills/plugins widen their declared powers.
- Added `/adapters/checks` to test adapter environment readiness for local broker, runner tools, scoped MCP, GitHub publication, learning registry, and custom adapter health metadata.
- Added `/intake/queues` to group continuous operational intake across blockers, human requests, reviews, follow-ups, routines, and backlog without adding another primary navigation surface.
- Added `/release/artifact` and `wuphf release-check --artifact <path>` so release/readiness evidence can be emitted as a verifiable JSON work product with state and checksum.
- Studio Operator now surfaces intake, evals, adapter checks, and capability-upgrade previews inside the existing operation/diagnostics modes rather than creating more tabs.
- The Electron desktop shell now includes a tray menu for opening MaestrIA, reloading, jumping to Runtime Doctor, and keeping the app available while the main window is closed.

## Phase 17 - Plugin Runtime, Secret Refs, Workspace Inventory, Outcomes, And Adapter Actions

- Added `/plugins/runtime` as a unified inventory for adapter-like integrations, skill/plugin metadata, scheduled plugin jobs, and recent plugin-related action history.
- Added `/adapters/config-checks` to validate `secret:`, `env:`, and `config:` references while redacting raw secret-looking values from operator output.
- Added `/adapters/actions` as a governed adapter-action bridge. It requires declared capabilities, explicit confirmation, and a reason, then records an audited request without executing arbitrary local processes.
- Added `actor_type` to the activity ledger so events can distinguish human, agent, system, and adapter-originated activity.
- Added `/workspaces` to consolidate task worktrees and linked repos into a channel-scoped workspace inventory with health and git dirty counts.
- Added `/outcomes` to classify task results into `merged_code`, `published_doc`, `answered_request`, `explicit_decision`, or `accepted_artifact`, keeping the control plane focused on real outcomes.
- Studio Operator now shows plugin runtime, config refs, workspaces, and outcome taxonomy in the existing Diagnóstico mode instead of adding more navigation.

## Phase 18 - Agent Sessions, Execution Trace, And Rollback Packages

- Added `/agent-sessions` as a channel-scoped persistent session view over members, agent activity, open tasks, execution locks, heartbeats, workspaces, usage totals, and next-action hints.
- Added `/execution-trace` to build a task timeline from task creation, execution locks, plan revisions, actions, execution nodes, thread messages, artifacts, evals, feedback, and outcome updates.
- Added `/governance/rollback-packages` as a confirmation-gated rollback package surface. It converts governance events into required reviews, compensating changes, snapshot hints, and an audited rollback request path.
- Rollback package POST requires `confirm=true`, `confirmation=ROLLBACK_PACKAGE`, actor, and reason. It records `governance_rollback_requested` without automatically mutating topology-sensitive or stateful objects.
- Studio Operator Diagnóstico now surfaces agent sessions, execution traces, and rollback packages next to the existing plugin/config/workspace/outcome checks.

## Phase 19 - Browser Evidence And Plugin Sandbox Preview

- Added `browser_inspection` task artifacts so frontend/browser evidence can carry page URL, selector, observed text, viewport, and screenshot path inside the normal task evidence contract.
- Added `/plugins/sandbox-preview` as a read-only sandbox readiness view over adapters, skill/plugin-like items, and a built-in `worker:noop-health` class. It reports required policies, missing policies, redacted config refs, risk signals, manifest metadata, health checks, filesystem/network/secret policy declarations, and next steps without executing arbitrary workers.
- Studio Operator Diagnóstico now shows plugin sandbox blockers next to plugin runtime inventory.
- Full plugin worker execution remains intentionally unported; only the health-only no-op worker is marked ready, and all action-running workers stay blocked until manifest, capability, filesystem, network, secret reference, and health policies are reviewable.

## Phase 20 - Marketplace Manifest Preview

- Added `/marketplace/manifest-preview` as a design-only marketplace contract over current skills, adapters, and the built-in no-op worker.
- The endpoint returns `persisted:false`, preview-only manifest signatures, source/expected hashes, trust score, capability drift, required reviews, missing policies, risk signals, and explicit `install_enabled:false` / `update_enabled:false`.
- Studio Operator Diagnóstico now shows marketplace manifest drift next to plugin sandbox status.
- Real marketplace install/update remains intentionally unported until trusted signatures, downloaded-content scanning, capability expansion review, rollback, and explicit apply paths exist.

## Phase 21 - Git-Native Wiki Promotion Preview

- Added `/knowledge/wiki-promotion-preview` as a read-only bridge from task/learning evidence to proposed wiki markdown diffs.
- The endpoint returns `persisted:false`, target wiki path, markdown, git-style diff, commit message, required reviews, lint findings, risk signals, and `reviewed_commit_only:true`.
- Studio Operator Diagnóstico now shows pending wiki promotion diffs next to marketplace/plugin diagnostics.
- The flow intentionally does not create wiki files, write shared memory, commit changes, or install a wiki backend. A future apply path must stay reviewed, git-native, and explicit.

## Phase 22 - Browser Inspection Handoff Preview

- Added `/browser/inspection-handoff-preview` as a read-only handoff package over existing `browser_inspection` task artifacts.
- The endpoint returns `persisted:false`, task/artifact references, page URL, selector, observed text, screenshot path, viewport, evidence summary, handoff prompt, readiness, missing fields, risk signals, and next step.
- Studio Operator Diagnóstico now shows browser handoffs next to marketplace and wiki diagnostics.
- The flow intentionally does not launch a browser, embed Browser Lab in Studio, create artifacts, or mutate tasks. It only packages evidence that was already recorded.

## Phase 23 - Long-Horizon Control Previews

- Added `/runtime/remote-sandbox-preview` as a read-only remote execution design surface. Docker, SSH, and self-hosted worker candidates expose required/missing policies, binary checks, risk signals, next steps, and `execution_enabled:false` without starting backends or connecting remotely.
- Added `/integrations/desktop/preview` so the optional desktop shell, tray shell, and Browser Lab ideas are visible as wrappers around canonical web Studio. The preview reports readiness and missing checks without launching Electron.
- Added `/companies/control-plane-preview` as the first multi-company control-plane slice. It reports the current company snapshot, scrubbed export items, isolation contracts, blocked mutations, missing policies, and disabled apply/topology mutation flags without changing `company.json`, `broker-state.json`, agents, or channels.
- Studio Operator Diagnóstico now shows remote sandbox, Desktop/IDE, and multi-company previews next to the existing marketplace/wiki/browser diagnostics.
- Remote execution, Desktop/IDE packaging expansion, and real multi-company isolation remain intentionally unported until there are explicit apply paths, review policies, rollback, and validation coverage.

## Phase 24 - Refreshed Roadmap Governance Previews

- Added `/scheduler/revisions-preview` as a read-only routine revision safety surface. It reports scheduler jobs, missing append-only revision/restore/conflict policies, disabled restore, blocked revision actions, and next steps without writing history or restoring routines.
- Added `/knowledge/wiki-editor-preview` to model source/rich wiki editor readiness, markdown round-trip, wikilink preservation, code-region safety, draft restore, conflict detection, and accessibility checks while keeping editing disabled.
- Hardened `/runtime/remote-sandbox-preview` with install-command governance fields: adapter install commands now expose policy, preview text, `install_command_enabled:false`, missing policy checks, and `install_command_disabled` risk without running installation steps.
- Added `/providers/compatibility-preview` for provider/CLI wire-format assumptions, including Codex stream hardening, Gemini CLI v0.38 stream-json parser risk, Claude hook assumptions, and Ollama local stream/error fixtures.
- Added `/studio/project-overview-preview` as a read-only widget manifest for readiness, provider tools, GitHub PRs/issues, workspaces, and active tasks with widget mutations and GitHub queries disabled.
- Added `/files/context-handoff-preview` so task artifacts, workspaces, and worktrees can be reviewed as file references before any drag/drop prompt injection or content read path exists.
- Studio Operator Diagnóstico now shows these previews next to the existing sandbox/wiki/browser/company diagnostics.

## Deliberately Not Ported Directly

- Paperclip's DB-backed issue schema: MaestrIA already has local broker state and task contracts.
- Paperclip's full plugin worker runtime: useful but still too broad beyond the current health-only sandbox preview and capability design.
- Company import/export topology automation: conflicts with MaestrIA protected topology unless explicitly requested by the user.
- Paperclip's web UI architecture: MaestrIA has a separate React app and TUI surface with existing tests.
