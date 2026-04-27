# Dunderia Remediation Program

This `openspec` tree tracks broad change programs that should not be executed as one opaque patch.

Current active program:

- `codebase-hardening-remediation`

Source review:

- `CODEBASE_REVIEW_2026-04-21.md`

Execution rule:

- security and runtime-contract fixes land before backend isolation, web UX correctness, and cleanup-only refactors
- each milestone should stay small enough to verify with targeted tests and builds
- later milestones must reuse the findings and decisions from the earlier ones rather than re-opening already-closed contracts
