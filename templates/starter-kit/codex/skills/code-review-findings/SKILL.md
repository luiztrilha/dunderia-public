---
name: code-review-findings
description: Code review workflow that prioritizes findings over summaries. Use when the user asks for a review, PR review, patch audit, or wants bugs, regressions, risks, and missing tests identified instead of implementation.
---

# Code Review Findings

## Overview

Use this skill to review changes with a bug-finding mindset. The primary output is a ranked list of findings with concrete references, not a general walkthrough of the patch.

## Review Priorities

1. Correctness bugs
2. Behavioral regressions
3. Missing or weak validation
4. Security or data integrity risks when relevant
5. Performance or maintainability issues only when materially important

## Workflow

1. Identify the review scope: diff, PR, branch, or selected files.
2. Read the changed code and the nearby context needed to understand behavior.
3. Look for ways the change can fail in real usage, not just style deviations.
4. Verify whether tests cover the changed behavior or leave obvious gaps.
5. Report only findings that have a clear technical basis.

## Finding Format

- State the problem.
- Explain why it matters.
- Point to the specific file and line.
- Mention the triggering condition or scenario when useful.
- Keep it concise and falsifiable.

## Output Contract

- Present findings first, ordered by severity.
- Keep summaries brief and secondary.
- If no findings are found, say so explicitly and mention residual risks or testing gaps.
- Avoid style-only nits unless the user asked for them.
