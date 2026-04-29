# DunderIA Runtime Contract

This document captures the operating rules the DunderIA broker, runners, tasks, and UI should preserve. It borrows useful control-plane ideas from Paperclip while keeping DunderIA's local-first model: single binary, local broker, fresh per-turn runners, scoped MCP, and isolated worktrees.

## Source Of Truth

For implementation details, use the code. For product posture, use this document together with:

- `ARCHITECTURE.md`
- `DEVELOPMENT.md`
- `AGENTS.md`
- `templates/starter-kit/`

Do not use this document to justify mutating protected office topology. Agent rosters, channels, saved workflows, and live broker state still require explicit user authorization.

## Core Runtime Model

DunderIA separates these concepts:

- **Structure:** channels, agents, tasks, threads, and worktrees.
- **Ownership:** who is responsible for the next move.
- **Execution:** whether a runner is currently doing work or can be woken.
- **Visibility:** whether the human can see what is moving, waiting, blocked, or done.

The product should be able to answer: "what moves this forward next?" without requiring the operator to reconstruct intent from logs.

## Wake Semantics

DunderIA is push-driven. Idle agents should not poll.

Valid wake paths:

- a direct human message or mention;
- a task assignment or task lane that needs work;
- a scheduler/runtime event that is explicitly configured;
- a recovery/watchdog event that names the owner and reason;
- a human-approved integration callback.

If a runner is already active for the same work, new wake signals should coalesce or surface as queued follow-up instead of launching duplicate work blindly.

## Task State Semantics

Task state is not just a UI label. It implies what the system and operator should expect.

| State | Meaning | Required next-path expectation |
|---|---|---|
| `open` / `pending` | Work exists but has not clearly started | assign, clarify, or intentionally leave in backlog |
| `in_progress` | Work is actively owned | active runner, queued continuation, or visible owner |
| `review` / `in_review` | Execution is paused for review or approval | named reviewer, pending request, or human owner |
| `blocked` | Work cannot proceed safely | explicit blocker, owner, requested decision, or recovery action |
| `done` | Work is complete | no runner should continue it |
| `canceled` | Work intentionally stopped | no runner should continue it |

Avoid leaving agent-owned `in_progress` work in a silent state with no active runner, queued wake, explicit blocker, or visible human handoff.

## Ownership And Blockers

Prefer explicit ownership over ambient responsibility.

- A blocked task should say what it is waiting on and who can unblock it.
- A review task should name the reviewer or approval surface.
- Parent/child or channel relationships explain structure; they are not the same thing as dependency.
- If dependency matters, represent it as a blocker or as a visible task/request relationship.

When the system cannot infer the next safe action, it should surface ambiguity instead of guessing.

## Recovery Contract

Recovery should be conservative.

Allowed automatic recovery:

- re-surface a task after restart when ownership is clear;
- replay unfinished task receipts to the responsible agent;
- create or update a visible watchdog/recovery record when a runner is silent or failed;
- keep the existing owner when retrying continuity.

Avoid automatic recovery that:

- reassigns work to a different agent without an explicit rule;
- marks work done from prose alone;
- modifies channels, agents, company state, or blueprints without current user approval;
- hides repeated failures behind endless retry loops.

After one failed continuity attempt, prefer `blocked` plus a visible comment/request that names the owner and action needed.

## Run Liveness

A successful runner exit is not automatically useful progress. DunderIA classifies headless turns with these liveness states:

- `completed`: the task reached a durable terminal/review state or recorded external execution.
- `advanced`: the task mutated, the workspace changed, or the task type allows narrative research/planning progress.
- `blocked`: the agent or task reported a blocker.
- `failed`: the runtime failed before useful progress.
- `empty_response`: the runtime exited successfully without substantive output or evidence.
- `plan_only`: the agent only described future work without changing task state or leaving evidence.
- `needs_followup`: reserved for ambiguous turns that need explicit continuation.

For coding, local-worktree, and live-external tasks, the existing durable-state guard remains authoritative. For office-mode tasks, liveness prevents "I will do this next" and empty successful exits from being treated as silent progress.

The latest liveness verdict should be visible in runtime observability surfaces. It is diagnostic metadata, not a replacement for task state: task `status`, `review_state`, blockers, and recorded external evidence remain the durable contract.

## Cost And Usage Visibility

Usage is part of the runtime contract, not a nice-to-have.

DunderIA should continue surfacing:

- per-message or per-run token usage when available;
- per-agent/session totals;
- cost estimates when providers expose enough data;
- attention states for unusually expensive or repeated work.

Future budget controls should be explicit and scoped. A hard stop should pause new execution and surface a human decision path rather than silently dropping work.

## Runtime Skills And Context

Runtime context should be scoped and intentional.

- Load only the skills and MCP tools needed by the runner and mode.
- Keep reusable workflows in `templates/starter-kit/` or repo-local skill folders.
- Treat generated skills as durable artifacts only when the user asked for a reusable workflow.
- Never smuggle secrets or machine-local paths into skills, prompts, or templates.

## Adapter And Integration Boundary

DunderIA supports multiple providers and optional integrations. New provider or integration work should preserve these boundaries:

- core broker behavior remains provider-neutral;
- provider-specific behavior lives behind provider/adapter modules;
- optional integrations remain opt-in;
- UI should discover capabilities from runtime state where practical instead of duplicating hardcoded lists;
- mutating real-world actions remain gated by human approval unless the user explicitly opts into a broader mode.

## Operator Handoff

Every meaningful agent action should leave enough trail for the operator:

- what was attempted;
- what changed;
- what remains blocked or needs review;
- where evidence/logs/artifacts live;
- what was verified versus assumed.

The operator should not need to babysit terminals, but should always be able to audit the work.
