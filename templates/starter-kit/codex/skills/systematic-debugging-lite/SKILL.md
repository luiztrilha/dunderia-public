---
name: systematic-debugging-lite
description: Structured root-cause debugging for bugs, failing tests, regressions, flaky behavior, and unexpected runtime errors. Use when a fix is needed but the cause is not yet proven, when tests fail without an obvious reason, or when the first plausible patch would be guesswork.
---

# Systematic Debugging Lite

## Overview

Use this skill to force evidence before patching. The goal is to prove the real failure mode, narrow the scope, and only then change code.

## Workflow

1. Reproduce the issue as directly as possible.
2. Define the observed behavior, expected behavior, and exact evidence.
3. Narrow the failing surface before editing code.
4. Form a small set of concrete hypotheses.
5. Prove or disprove each hypothesis with logs, traces, targeted reads, or minimal experiments.
6. Fix only the confirmed root cause.
7. Re-run the most relevant validation and confirm the failure is actually gone.

## Rules

- Do not patch the first theory that sounds plausible.
- Do not broaden the change set until the failing condition is isolated.
- Prefer direct evidence over intuition: stack traces, failing assertions, log lines, config state, request payloads, and git history.
- If the issue cannot be reproduced, say so explicitly and switch to instrumentation or evidence gathering instead of pretending the cause is known.
- If multiple factors exist, separate trigger, root cause, and amplification effects.

## Practical Checks

- For failing tests: identify the first real failure, the minimal repro command, and whether the failure is deterministic.
- For runtime bugs: inspect recent changes, error payloads, environment/config differences, and boundary inputs.
- For regressions: compare the old working path against the changed path and look for the behavioral delta instead of scanning the entire module.
- For flaky behavior: look for timing, shared state, ordering, cache, external dependency, or test isolation issues.

## Output Contract

- State the proven root cause or state explicitly that it remains unproven.
- Name the validation that reproduced the issue and the validation that confirmed the fix.
- If anything remains uncertain, list it as a residual risk rather than hiding it.
