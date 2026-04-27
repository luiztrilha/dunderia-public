---
name: implementation-planning-lite
description: Lightweight implementation planning for larger, ambiguous, or cross-file coding tasks. Use when the user explicitly asks for a plan, when multiple plausible approaches exist, or when the work benefits from a concise file map and validation strategy. Do not trigger for trivial one-file edits.
---

# Implementation Planning Lite

## Overview

Use this skill to make a short plan that improves execution instead of delaying it. The plan should make the next steps clearer, bound the write scope, and identify validation early.

## When To Plan

- The task spans multiple modules or repos.
- The user explicitly asks for a plan.
- The request is ambiguous enough that coding immediately would create avoidable churn.
- There are multiple viable approaches with different tradeoffs.

If none of these are true, skip the formal plan and start executing.

## Workflow

1. Restate the objective and non-negotiable constraints.
2. Inspect the repo enough to identify the likely write set.
3. List the smallest sequence of steps that reaches the outcome.
4. Attach a validation approach to the steps that matter.
5. Note open questions or decision points only when they change implementation.

## Plan Shape

- Keep plans short: usually 3 to 7 steps.
- Prefer concrete file or module references when known.
- Make each step testable or at least inspectable.
- Avoid ceremonial substeps unless the work is genuinely risky.

## Output Contract

- The plan must help execution start immediately after it is written.
- Include validation, not just implementation steps.
- Mention assumptions separately from committed steps.
