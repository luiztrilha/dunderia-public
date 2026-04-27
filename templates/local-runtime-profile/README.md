# Local Runtime Profile

Public, sanitized snapshot of the local agent runtime profile.

This package keeps the portable pieces of the setup:

- `skills/`: installed agent skills and skill source material
- `orientation/`: reusable Markdown guidance, commands, prompts, and plugin docs
- `config/`: sanitized Codex config shape, public approval-rule examples, and skill lock metadata

It intentionally does not include private sessions, secrets, local database helpers,
machine-specific paths, credentials, browser state, logs, caches, or workspace memory.

## Using It

Treat this as a reference profile, not a drop-in restore of a private machine.

1. Review `EXCLUSIONS.md`.
2. Copy only the folders you want into your local runtime.
3. Replace placeholders in `config/codex/config.sanitized.toml`.
4. Create your own private approval rules from `config/rules/default.rules`.

## Safety Notes

The private runtime snapshot remains outside this public repo. This public package
was rebuilt from the snapshot with private SQL workflow files, local paths, and raw
machine config excluded.
