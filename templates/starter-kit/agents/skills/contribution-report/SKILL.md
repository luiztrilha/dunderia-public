---
name: contribution-report
description: >
  Produce a maintainer-grade report for a PR, branch, patch, or external repo
  comparison. Use when asked to analyze a contribution deeply, explain design
  choices, compare systems, or recommend what to port or merge.
---

# Contribution Report

Use this skill to produce a deep but practical maintainer report.

## Default Posture

- Understand the change before judging it.
- Explain the system as built, not just the diff.
- Separate product direction, architecture, implementation, rollout, and documentation concerns.
- Keep the recommendation concrete: merge, narrow, port selected pieces, or keep as research.

## Workflow

1. Gather the target: PR, branch, repo, patch, or local diff.
2. Read the relevant docs and invariants for both source and destination systems.
3. Reconstruct the lifecycle:
   - install/setup;
   - runtime path;
   - trust boundary;
   - failure/recovery behavior;
   - UI/operator surface;
   - verification path.
4. Identify strengths worth keeping.
5. Identify risks, incompatibilities, and missing tests.
6. Decide what should be ported now versus left as a recommendation.
7. Verify any generated artifact or local changes.

## Report Structure

Use this shape unless the user requested another format:

1. Executive summary
2. What the source actually adds
3. Useful ideas to port
4. Risks and non-portable pieces
5. Recommended DunderIA changes
6. Verification or evidence

## Heuristics

Good things to port:

- explicit contracts;
- scoped runtime context;
- typed extension points;
- honest trust model;
- visible recovery paths;
- smaller onboarding loops;
- reusable skills or templates after safety review.

Be careful with:

- hidden global state;
- plugin systems that claim sandboxing but run trusted code;
- broad auth or secret access;
- topology mutations disguised as onboarding;
- code that assumes a SaaS backend when DunderIA is local-first.

## Final Output

Summarize:

- what was reviewed;
- what was ported;
- what was rejected or deferred;
- what was verified.
