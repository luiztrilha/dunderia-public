# Codebase Hardening Remediation

## Why

`CODEBASE_REVIEW_2026-04-21.md` identified a mix of P0-P3 issues across MCP tooling, broker/backend runtime, web contracts, and low-risk redundancy. The repo needs a phased remediation program instead of one oversized patch.

## What Changes

This change program is split into four milestones:

1. Security and runtime contracts
2. Backend isolation and correctness
3. Web correctness and UX contracts
4. Simplification and redundancy removal

Milestone 1 is the first executable slice.

## Scope Of Milestone 1

- contain MCP sync paths to the intended root
- sanitize unsafe markdown link schemes
- stop persisting secrets in browser-local drafts
- require auth/trusted access for onboarding mutations
- realign MCP team tools with the canonical runtime env contract
- repair reaction payload contract drift across broker and web clients

## Reference Design

See `docs/superpowers/specs/2026-04-21-codebase-hardening-design.md`.

## Out Of Scope For This Change Slice

- execution graph ID generation changes
- workspace/worktree isolation fixes
- web polling/hash-router/search cleanup
- dead-code and consistency-only removals
