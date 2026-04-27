---
name: self-improvement-lite
description: Minimal learning-capture and promotion guardrails for the local Codex/OpenClaw workspace. Use when a task reveals a verified, reusable fact, command, workflow rule, or behavioral constraint that may deserve persistence in existing canonical files such as daily memory, MEMORY.md, TOOLS.md, AGENTS.md, or SOUL.md. Do not use for routine failures, speculative patterns, or hook-based auto-logging.
---

# Self-Improvement Lite

Use this skill to decide whether a learning should persist, where it belongs, and when it should stay out of memory entirely.

## Core Rule

Prefer the existing canonical files. Do not create a parallel `.learnings/` workflow, do not install reminder hooks, and do not promote content just because it feels useful in the moment.

## Signal Quality

Treat these as strong candidate signals:

- Explicit user correction: "No, that's not right", "Actually, it should be...", "Stop doing X", "I told you before..."
- Explicit durable preference: "Always do X", "Never do Y", "My style is...", "For this workspace, use..."
- Verified self-reflection after meaningful work: a concrete lesson that changed how the task should be done next time
- Repeated operational evidence: the same workflow, command, or constraint proving useful across multiple tasks

Treat these as weak signals unless stronger evidence appears:

- One-time instructions for the current task
- Context-specific notes that do not generalize
- Hypotheticals, brainstorming, or untested ideas
- Silence, lack of complaint, or vague approval without a reusable rule

## Use This Skill To Decide

1. Is the signal verified, reusable, and likely to matter again?
2. Is it local to the day, durable across sessions, operational, behavioral, or governance-related?
3. Which existing file is the narrowest correct destination?
4. Is no write the right answer because the signal is too weak?

## Write Targets

Use exactly one primary destination unless there is a strong reason to update more than one file.

| Target | Write here when | Do not write here when |
|---|---|---|
| `memory/YYYY-MM-DD.md` | The note is useful today, tied to current execution, or still provisional | The fact is already stable and belongs in curated memory |
| `MEMORY.md` | The fact is durable, verified, and likely to help future sessions | It is only a one-off fix, noisy log, or same-day scratch note |
| `TOOLS.md` | The value is a command, path, script, switch, or tool-specific gotcha | It is a policy, behavior rule, or broad project fact |
| `AGENTS.md` | The value is a workflow, routing, autonomy, or execution contract | It is merely a tool tip, transient task note, or personal style preference |
| `SOUL.md` | The value is a durable behavior or communication principle | It is task-specific process or local operational detail |

## Strict Filters

Write nothing when any of these are true:

- The learning is speculative or not yet verified.
- The signal is only a raw error log with no reusable takeaway.
- The note duplicates a canonical file that already says the same thing.
- The note would create a second source of truth beside the workspace contract.
- The note is only useful to explain the current task result.
- The only "evidence" is silence, inferred preference, or a one-off instruction.
- The candidate lesson is scoped to a single file, branch, or transient incident and does not generalize.

## Promotion Rules

Promote only after evidence exists.

- Promote from daily memory to `MEMORY.md` only when the fact is still true after the task ends.
- Update `AGENTS.md` only for explicit workflow or governance changes.
- Update `SOUL.md` only for durable behavior changes, not convenience heuristics.
- Update `TOOLS.md` only for commands, paths, scripts, or integration gotchas.
- If a user correction changes one local answer but not future workflow, keep it in the task output and stop there.
- Repeated evidence can justify review, but recurrence alone is not promotion. Treat "3 similar corrections/patterns" as a signal to evaluate, not an automatic write.
- If the pattern is still ambiguous, keep it in daily memory or do not write it at all.

## Minimal Workflow

1. Restate the candidate learning in one sentence.
2. Classify the source: `explicit_correction`, `explicit_preference`, `self_reflection`, `repeated_operational_evidence`, or `weak_signal`.
3. Decide `write` or `do_not_write`.
4. If `write`, choose the single narrowest destination.
5. Write a short factual note with concrete paths, commands, or files when relevant.
6. Avoid long incident narratives unless the target file already uses them.

## Output Contract

When using this skill, think in this shape before editing:

```text
Decision: write | do_not_write
Signal: explicit_correction | explicit_preference | self_reflection | repeated_operational_evidence | weak_signal
Reason: <one sentence>
Target: memory/YYYY-MM-DD.md | MEMORY.md | TOOLS.md | AGENTS.md | SOUL.md | none
Payload: <short factual note if writing>
```

## Local Constraints

- No automatic hooks.
- No `.learnings/` dependency.
- No promotion based on recurrence counters alone.
- No copying raw transcripts or secret-bearing command output into memory files.
- No workspace-wide rule changes without evidence that the rule is durable.
- No learning from silence.
- No broad namespace scans when a narrow canonical file is enough.
- Prefer merge, summarize, or demote over deleting established memory.
