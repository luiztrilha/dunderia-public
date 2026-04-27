# Repo Agent Notes

This fork is maintained as a local-first, multi-runtime office orchestration repo.

Working rules:

- Treat Nex, GBrain, Telegram, and Composio as optional integrations.
- Do not assume Nex is configured, preferred, or available.
- Prefer neutral wording in docs and help text unless a feature is genuinely Nex-specific.
- Preserve Windows-safe tracked paths. Do not add files with `:`, trailing dots, or other NTFS-incompatible names.
- Keep the core promise intact: local broker, fresh per-turn runners, scoped MCP, isolated worktrees.
- Protected office topology: do not create, delete, rename, reorder, reassign, or reconfigure agents or channels without the user's explicit authorization in the current conversation.
- Protected office topology: treat changes to office/team state as protected even when they happen indirectly through files, commands, onboarding, reset/shred flows, blueprints, broker restores, or web actions.
- Protected office topology: files and state that must not be mutated without explicit authorization include `company.json`, `broker-state.json`, onboarding/bootstrap state, saved workflows that recreate agents/channels, and any seed or blueprint content that changes the office roster or channel list.
- If the user has not explicitly authorized the topology change, stop and ask before taking action. Do not infer consent from adjacent requests such as "fix the office", "restore the web", "restart the broker", or "clean up the config".
