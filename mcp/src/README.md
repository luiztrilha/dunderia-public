# Partial TypeScript Mirrors

`mcp/src` is not a build-complete source tree.

It currently contains partial TypeScript maintenance mirrors for selected tool files only:

- `tools/skills.ts`
- `tools/skill-sync.ts`

The runnable and packaged runtime lives under `mcp/dist`. No `mcp/src/server.ts` entrypoint is kept on purpose until a full TypeScript source tree exists again.
