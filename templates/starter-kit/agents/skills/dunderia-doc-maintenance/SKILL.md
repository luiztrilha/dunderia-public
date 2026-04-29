---
name: dunderia-doc-maintenance
description: >
  Audit DunderIA documentation against recent changes and update only factual
  drift. Use after notable feature work, release prep, or when asked whether
  README, architecture, starter-kit, or development docs still match reality.
---

# DunderIA Doc Maintenance

Detect documentation drift and fix it with minimal edits.

## Target Documents

Start with:

- `README.md`
- `ARCHITECTURE.md`
- `DEVELOPMENT.md`
- `PUBLIC_RELEASE.md`
- `templates/starter-kit/README.md`
- `templates/starter-kit/EXCLUSIONS.md`

Add other docs only when the changed code clearly affects them.

## Workflow

1. Inspect `git status --short` and avoid unrelated user changes.
2. Review recent commits or the current diff to identify user-visible behavior changes.
3. Classify changes:
   - feature or capability;
   - changed command or install path;
   - removed/renamed behavior;
   - security, release, or onboarding implication.
4. Audit target docs for:
   - shipped features still described as future work;
   - quickstart or command drift;
   - public release guidance that would ship private state;
   - optional integrations described as required;
   - stale provider/runtime claims.
5. Apply the smallest factual edits.
6. Run `git diff --check` on touched docs.

## Editing Rules

- Fix the fact, not the voice.
- Do not reorganize docs during a maintenance pass.
- Do not add new broad sections unless the user asked for a doc expansion.
- Preserve DunderIA public naming and `wuphf` CLI compatibility wording.
- Keep Nex, Telegram, Composio, and other integrations optional unless the code proves otherwise.
- Never add private paths, credentials, live office state, or customer context.

## Final Output

Report:

- what drift was found;
- which files changed;
- what verification ran;
- any larger documentation work deliberately deferred.
