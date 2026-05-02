# Validated Operating Policies

These policies are distilled from the active local global instructions, repo `AGENTS.md`, and current release-safety rules.

## Command Summaries

- Do not call `distill` automatically.
- Only use `distill` when the user explicitly asks for it.
- When `distill` is used, the prompt must be explicit and should request a strict output contract.
- If `distill` is unavailable, use raw command output as the fallback.

## Skills

- Prefer local skills when the task naturally matches them.
- Keep skills as aids, not ceremony.
- Skip skill overhead for trivial requests.
- Use `verification-before-close` before claiming work is complete.
- Use `code-review-findings` when the user asks for a review or audit.
- Use `systematic-debugging-lite` for bugs and failing tests when the root cause is not proven.
- Use `implementation-planning-lite` for larger, ambiguous, or multi-file tasks.

## Public Release Safety

- Do not publish auth files, tokens, cloud credentials, ADC files, private keys, `.env*`, local session logs, sqlite state, or command history.
- Do not publish live office state: `company.json`, `broker-state.json`, onboarding state, channel history, saved workflows, task receipts, or cloud backup bootstrap state.
- Do not publish private customer or employer context.
- Keep public skills generic unless a domain-specific skill has been explicitly sanitized.

## DunderIA Office Topology

- Do not create, delete, rename, reorder, reassign, or reconfigure agents or channels without explicit user authorization in the current conversation.
- Treat topology changes as protected even when they happen indirectly through config files, onboarding, reset flows, blueprints, broker restores, or web actions.
- If authorization is absent, stop and ask before changing topology.
- Keep `game-master` manual-only and owner-invoked; do not route work to it automatically.

## Engineering

- Treat Nex, GBrain, Telegram, and Composio as optional integrations.
- Preserve the core runtime promise: local broker, fresh per-turn runners, scoped MCP, and isolated worktrees.
- Prefer repo-local patterns over new abstractions.
- Keep edits scoped to the task.
- Do not revert unrelated dirty worktree changes.
- Preserve Windows-safe tracked paths.
- Validate with the smallest meaningful command or inspection before declaring success.

## Local Engineering References

- The active local profile can consult a local engineering reference mirror.
- Public users should treat this as optional and replace `<LOCAL_ENGINEERING_REFERENCES>` with their own local reference path if they have one.
