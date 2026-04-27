# Codex global instructions

Distill is optional and must only be used when the user explicitly asks for it.

Global rule
- Do not call `distill` automatically.
- Only use `distill` when the user explicitly asks for it.
- When used, the `distill` prompt must be explicit. Use strict output contracts.

How to use in PowerShell
- pipe command output: <command> | distill "Return only: ..."
- examples:
  - `git diff | distill "Return only the changed files and a one-line summary for each file."`
  - `rg -n "TODO|FIXME" . | distill "List file paths only, one per line."`
  - `Get-ChildItem -Recurse | distill "Return only file paths, one per line."`
  - `npm test 2>&1 | distill "Return PASS or FAIL, then failing test names if any."`

Do not use distill unless:
- the user explicitly asks for it.

If distill is not installed:
- install with `npm i -g @samuelfaj/distill`.
- use raw output only as fallback.

## Local workflow skills

When a task naturally matches one of the local skills in `~/.codex/skills`, prefer using it as guidance.

- `doc`: for `.docx` work where layout fidelity, styles, numbering, tracked changes, comments, fields, or section behavior matter
- `xlsx`: for Excel or spreadsheet work where formulas, dates, types, formatting, recalculation, or workbook preservation matter
- `pptx`: for PowerPoint decks where layouts, placeholders, notes, charts, template fidelity, or visual QA matter
- `systematic-debugging-lite`: for bugs, failing tests, regressions, flaky behavior, and runtime errors where the cause is not yet proven
- `self-improvement-lite`: when a task uncovers a verified durable fact, command, workflow rule, or behavior that may deserve persistence in daily memory, `MEMORY.md`, `TOOLS.md`, `AGENTS.md`, or `SOUL.md`
- `verification-before-close`: before claiming a task is complete; separate verified evidence from unverified assumptions
- `code-review-findings`: when the user asks for review, audit, or bug-finding on an existing patch
- `implementation-planning-lite`: for larger or ambiguous tasks, or when the user explicitly asks for a plan

These skills are aids, not mandatory ceremony. Skip them for trivial requests when they would only add overhead.

## Local engineering references

Use the local mirror `<LOCAL_ENGINEERING_REFERENCES>` as a global consultative reference for software engineering judgment when a task benefits from book-inspired guidance.

Practical rule:
- Start from `unified-software-engineering/codex/AGENTS.md` as the broad default lens when no narrower local rule applies.
- Add at most one focused lens for the task, such as `refactoring`, `working-effectively-with-legacy-code`, `release-it`, `designing-data-intensive-applications`, DDD, or architecture.
- Do not enable or paste all rule sets at once.
- Repo-local `AGENTS.md`, workspace rules, explicit user instructions, local skills, and real validation take precedence.
- Treat this mirror as guidance, not as an installed runtime dependency.

## Preset de comandos para Windows PowerShell (distill)

- `git diff | distill "Return only the changed files and a one-line summary for each file."`
- `git status --short | distill "Return only the file paths, one per line."`
- `rg -n "TODO|FIXME" . | distill "Return only file paths, one per line."`
- `Get-ChildItem -Recurse | distill "Return only file paths, one per line."`
- `npm test 2>&1 | distill "Return PASS or FAIL, followed by failing test names if any."`
- `bun test 2>&1 | distill "Return PASS or FAIL, followed by failing test names if any."`
- `terraform plan 2>&1 | distill "Return ONLY: SAFE, REVIEW, or UNSAFE, followed by exact risky changes."`
- `npm audit 2>&1 | distill "Extract vulnerabilities and return valid JSON only."`
- `git log --oneline -20 | distill "Summarize the risk of the last 20 commits in 5 bullets, in Portuguese."`
- `Get-Process | distill "Return only process names and IDs as JSON only."`

