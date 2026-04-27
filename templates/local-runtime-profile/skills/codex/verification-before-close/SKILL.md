---
name: verification-before-close
description: Final verification checklist for coding and operational tasks. Use before claiming work is done, after applying a fix, or when preparing a final answer that depends on tests, builds, scripts, UI checks, migrations, or other validation evidence.
---

# Verification Before Close

## Overview

Use this skill at the end of execution to separate what is proven from what is only likely. The goal is to prevent false closure and make the final handoff evidentiary.

## Workflow

1. Re-read the requested outcome.
2. Confirm which files, commands, scripts, or UI paths were actually touched.
3. Run or inspect the most relevant validation that is practical in the current environment.
4. Separate passed checks, skipped checks, and blocked checks.
5. Identify any manual follow-up, deployment step, or environment-specific dependency.
6. Write the final answer so claims match evidence.

## Rules

- Do not say work is complete if the relevant validation was not run.
- Do not blur "not tested" into "working".
- Prefer the smallest validation that proves the requested outcome.
- If a full test suite is too expensive or unavailable, run the nearest meaningful check and state the gap.
- When the environment prevents validation, describe the blocker concretely.

## Evidence Categories

- Build or compile result
- Targeted test command
- Script output or exit code
- Manual UI or browser verification
- Diff inspection when execution is not possible
- Runtime/log evidence from the affected path

## Final Answer Contract

- Report what was actually verified.
- Name what was not run.
- Call out deployment, credentials, external services, or manual acceptance steps when they still matter.
- Avoid words like "done" or "fixed" unless the relevant evidence exists.
