---
name: dunderia-plan-to-tasks
description: >
  Convert a plan into executable DunderIA tasks or task lanes. Use when asked to
  break down work for the office, turn strategy into assignments, or prepare
  parallel agent work while preserving explicit blockers, owners, and human
  approval boundaries.
---

# DunderIA Plan To Tasks

Use this skill to translate a plan into work the DunderIA office can actually execute.

## Rules

- Do not create, delete, rename, reorder, or reassign agents or channels unless the user explicitly authorized that topology change in the current conversation.
- Do not hide missing capability. If no current agent or tool fits a task, surface the gap as a decision or follow-up.
- Assign work by specialty and context, not by convenience.
- Keep dependencies explicit. If a task cannot start until another task finishes, name the blocker.
- Independent branches should be allowed to run in parallel.
- If the next step is small and safe, do it instead of over-planning.

## Workflow

1. Identify the outcome, constraints, and success evidence.
2. List the concrete deliverables that would prove progress.
3. Map each deliverable to the best current owner or mark it as a gap.
4. Declare blockers and review points explicitly.
5. Split only where it helps parallel work or reduces risk.
6. Leave a short execution note: what can start now, what waits, and what needs the human.

## Task Shape

Prefer this shape when proposing DunderIA tasks:

```text
Task: <verb + object>
Owner: <agent/user/current role>
Channel: <existing channel, if known>
Evidence: <file, command, screenshot, artifact, or decision>
Blockers: <none or explicit task/request>
Review: <who checks it and what they check>
```

## Good Breakdown

Good tasks are:

- concrete enough that the owner can begin without re-asking;
- small enough to verify;
- tied to evidence;
- assigned to a real current owner;
- explicit about blockers and human decisions.

Weak tasks are:

- "improve the app";
- "research everything";
- "someone should check";
- "blocked by context" with no owner or next action;
- topology-changing work without current authorization.

## Final Output

When asked to produce a plan, return:

- execution-ready tasks;
- parallelization notes;
- blockers and review gates;
- gaps or missing capabilities;
- the first safe action to take.
