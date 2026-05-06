# Hermes Agent Absorption Plan

This note captures ideas worth learning from NousResearch's Hermes Agent while preserving MaestrIA's local-first office runtime: broker push, fresh per-turn runners, scoped MCP, isolated worktrees, explicit task contracts, and protected topology.

Sources reviewed:

- https://github.com/NousResearch/hermes-agent
- https://hermes-agent.nousresearch.com/docs/user-guide/features/skills
- https://hermes-agent.nousresearch.com/docs/user-guide/features/memory
- https://hermes-agent.nousresearch.com/docs/user-guide/features/tools
- https://hermes-agent.nousresearch.com/docs/developer-guide/architecture

## Non-Negotiable Boundaries

- Do not import Hermes as a parallel runtime or replace the broker/task/worktree model.
- Do not mutate agents, channels, blueprints, `company.json`, or `broker-state.json` without explicit current user authorization.
- Keep integrations optional. Hermes' broad gateway/platform scope is inspiration, not a mandate to make new external accounts required.
- Prefer MaestrIA-native Go and broker-backed surfaces over Python session-runtime concepts.
- Keep generated learning artifacts reviewable, scoped, and security-scanned before they influence future runs.

## What MaestrIA Already Has

- Push-driven local broker and event-based wake paths.
- Fresh per-turn provider runners for Claude Code, Codex, Gemini, and Ollama.
- Scoped MCP per agent and mode.
- Isolated worktrees and workspace inventory.
- Telegram integration, action providers, scheduler runtime, runtime doctor, smoke checks, release readiness, adapter registry, skill trust, and knowledge surfaces.
- Learning promotion from completed tasks into reusable skills.
- Resume packs and execution traces for fresh-runner continuity.

These overlap with Hermes' core promises enough that the useful move is not a port. The useful move is to tighten MaestrIA's learning loop and operator ergonomics.

## High-Value Ideas To Absorb

### 1. Agent-Curated Learning Loop

Hermes treats skills as procedural memory: after a complex task, correction, or discovered workflow, the agent can create or patch a skill. MaestrIA already has learning promotion, but the trigger can become more systematic:

- detect completed tasks with enough evidence, failures, corrections, or repeated command sequences;
- create a `learning_candidate` preview, not an automatic skill write;
- require explicit apply/approval before the skill changes future behavior;
- record provenance back to task, artifacts, execution trace, and reviewer.

Recommended MaestrIA shape: add a read-only `/learning/candidates` endpoint first, then a confirmation-gated apply path that reuses the existing preview/apply pattern.

### 2. Progressive Skill Disclosure

Hermes exposes skills in levels: list metadata first, view full skill only when needed, then view a specific support file. MaestrIA skills are visible and invokable, but future agent prompts should receive a compact index plus just-in-time retrieval instead of broad skill content.

Recommended MaestrIA shape:

- keep skill metadata in the broker;
- expose MCP tools such as `team_skill_list`, `team_skill_view`, and `team_skill_file`;
- keep `team_skill_run` as the audited invocation path;
- include capability, trust, health, and channel scope in metadata;
- avoid loading full skill bodies into every runner prompt.

### 3. External Skill Directories With Local Precedence

Hermes can scan shared read-only skill directories while writing agent-created skills only to its own local skill home. This maps neatly to MaestrIA's runtime-profile guidance:

- `templates/starter-kit/` remains public bootstrap;
- `~/.codex/skills` and `~/.agents/skills` can be discovered as optional local sources;
- repo/local skills should win over shared snapshots;
- missing external paths should be skipped without noise;
- private exported runtime profiles remain reference-only, never wholesale-installed.

Recommended MaestrIA shape: make skill source precedence explicit in the Skills app and `/skills` API.

### 4. Skill Hub Security Model

Hermes separates builtin, trusted, official, and community skill sources, then scans third-party skills for injection, exfiltration, destructive commands, and supply-chain patterns.

MaestrIA already has skill trust scoring and capability previews. The missing piece is a more formal install/update lifecycle:

- provenance fields for source, source type, upstream hash, installed hash, last scan, and trust level;
- security scan result that can block dangerous skills;
- drift check for installed remote skills;
- `--force`-style override only for non-dangerous warnings, never for dangerous findings.

Recommended MaestrIA shape: extend `/skills/trust` and `/skills/metadata-preview` before adding any installer.

### 5. Bounded Memory As A Frozen Prompt Snapshot

Hermes uses small bounded memory files and injects them as a frozen session-start snapshot to preserve prompt caching. MaestrIA currently keeps shared org memory disabled and relies on channel history, private notes, resume packs, and knowledge indexes.

The absorbable pattern is not "turn shared memory back on". It is:

- keep agent private notes bounded and compact;
- show capacity and consolidation pressure;
- scan memory entries for prompt-injection and secret-like content;
- freeze memory context per runner so the prompt prefix remains stable during a turn;
- make live writes visible through tool responses, while applying prompt impact only to later turns.

Recommended MaestrIA shape: add bounded private-note quotas and scanner results before reintroducing shared memory.

### 6. Session Search With Summarized Recall

Hermes stores sessions in SQLite/FTS5 and summarizes matching sessions through a cheap auxiliary model. MaestrIA has broker history, `/knowledge`, `/resume-pack`, and `repair-channel-memory`, but recall is still mostly task/work-product oriented.

Recommended MaestrIA shape:

- add a broker-backed message/task search endpoint with channel access filtering;
- rank matches by task, channel, actor, artifact, and recency;
- return concise summaries with links to source messages/tasks/artifacts;
- keep raw transcript dumps out of agent prompts unless explicitly requested.

### 7. Backend-Agnostic Execution Environments

Hermes supports local, Docker, SSH, Modal, Daytona, Singularity, and Vercel sandbox backends for terminal execution. MaestrIA should not copy all of this, but it can absorb the abstraction:

- define an execution-environment interface for task workspaces;
- keep local worktrees as the default and source of truth;
- add `docker` or `ssh` as a future governed adapter, not a core default;
- surface environment health in runtime doctor and adapter checks.

### 8. Shared Command Registry

Hermes centralizes slash command metadata so CLI, gateway help, platform menus, and autocomplete derive from one registry. MaestrIA has several command and UI surfaces; command drift is a predictable risk.

Recommended MaestrIA shape:

- introduce a command manifest for slash commands and CLI-like operator actions;
- generate help text and UI metadata from that manifest;
- keep mutating actions routed through existing broker approval/contracts.

## Proposed Phases

### Phase 1 - Discovery And Preview Only

- Add `/learning/candidates` as a read-only preview over completed tasks, corrections, failures, artifacts, and repeated workflows.
- Add tests for candidate filtering, channel access, provenance, and "no automatic skill mutation".
- UI: show candidates inside Skills or Operator diagnostics.

### Phase 2 - Progressive Skills API

- Add `team_skill_list` and `team_skill_view` MCP tools.
- Keep `team_skill_run` as the only audited invocation action.
- Update runner prompt assembly to include compact skill indexes instead of broad bodies where practical.

Status: further absorbed. `/skills/{name}/files-preview` now exposes skill support material as virtual files, listing metadata first and returning content only for the selected file or explicit include request. This completes the progressive disclosure shape without reading arbitrary filesystem paths.

### Phase 3 - Skill Provenance And Trust Lifecycle

- Extend skill records with source/provenance/hash/scan metadata.
- Extend `/skills/trust` to separate builtin, local, starter-kit, external, and remote/community trust.
- Add drift preview, not update apply.

Status: further absorbed. Skill records now expose source, hash, installed/scanned timestamps, and local scan status. `/skills/trust` includes provenance fields, `/skills/provenance-preview` reports dry-run legacy upgrades, and `/marketplace/manifest-preview` defines a read-only signed-manifest/drift contract before any install or update surface exists.

### Phase 4 - Bounded Private Memory

- Add per-agent private-note limits and consolidation warnings.
- Add prompt-injection/secret-like scans before accepting private notes.
- Surface memory capacity in agent/session observability.

Status: partially absorbed. `/memory/curation-preview` now reports read-only channel memory candidates from messages, tasks, and decisions, with recommended actions, utility signals, secret-like risk signals, and no automatic persistence.

### Phase 5 - Summarized Recall

- Add channel-scoped broker search over messages, tasks, artifacts, and promoted knowledge.
- Return structured source references and short summaries.
- Optionally add an auxiliary summarizer later, behind provider availability and cost visibility.

Status: partially absorbed. `/recall/search-preview` now searches visible messages, tasks, task artifacts, decisions, and promoted learning knowledge, returns short summaries with source references, ranking signals, and risk signals, and never persists recall results or injects raw transcripts automatically.

### Phase 6 - Scheduled Skills Readiness

- Allow scheduler jobs to declare multiple skill bindings.
- Add a read-only preview that checks missing, risky, proposed, or low-trust skills before scheduled execution.
- Flag scheduled skills that reference scheduler mutation as requiring review before broader automation.

Status: partially absorbed. Scheduler jobs now normalize `skill_names` alongside legacy `skill_name`, and `/scheduler/skill-preview` reports read-only readiness, trust, provenance, scan status, and scheduler-mutation risk for bound skills.

### Phase 7 - Toolset Profiles

- Add a read-only profile view for effective tools and capabilities per agent/channel.
- Compare declared `allowed_tools` to runtime MCP, adapter, and skill capabilities.
- Flag mutating, external, secret-bearing, and scheduler-mutating capabilities before enforcing any policy.

Status: partially absorbed. `/toolsets/profile-preview` now reports read-only effective toolsets, capability sources, drift, risk level, and suggested action per agent/channel without changing office topology.

### Phase 8 - Command Manifest

- Add a broker-backed manifest for slash commands and operator actions.
- Include category, route, mutability, confirmation requirement, topology sensitivity, and signals.
- Use it as the future source for help text, command palette metadata, autocomplete, and docs drift checks.

Status: partially absorbed. `/commands/manifest` now returns read-only command metadata for web slash commands, including mutating and topology-sensitive markers.

### Phase 9 - Execution Environment Preview

- Surface local, external, and future execution environments before enabling new backends.
- Keep local worktrees and explicit external workspaces as current supported paths.
- Mark Docker/SSH as planned/blocked until governed adapters, policies, secrets, and doctor checks exist.

Status: partially absorbed. `/runtime/execution-environments-preview` now summarizes current execution modes and workspace health while keeping Docker and SSH as blocked future adapters. The preview also reports Docker/SSH binary presence and missing workspace, secret, network, cleanup, audit, approval, host, and key policies without starting any backend or opening remote connections.

## Not Recommended

- Do not replace MaestrIA's broker with Hermes' session-centric AIAgent loop.
- Do not make multi-platform gateways a product requirement beyond current optional integrations.
- Do not let agents directly create/delete skills without review; MaestrIA's protected topology and skill capability model should stay stricter than Hermes.
- Do not copy Hermes' native path layout or Python plugin runtime wholesale.
- Do not introduce remote sandboxes before the doctor, adapter registry, secrets, and approval model can explain exactly what will run where.

## Best First Implementation Slice

The smallest useful slice is `/learning/candidates`.

Why:

- It absorbs Hermes' strongest idea, the closed learning loop, without granting new mutation power.
- It reuses existing task evidence, artifacts, outcomes, promoted learning, skill trust, and preview/apply patterns.
- It is safe with protected topology because it is read-only.
- It gives the operator a concrete place to decide which experiences deserve to become reusable skills.

Expected validation:

- focused Go tests for candidate generation and access filtering;
- `go test ./internal/team -run LearningCandidate`;
- docs update if the endpoint or UI becomes user-visible;
- regenerate `docs/MANUAL.pdf` only when behavior/user docs change.
