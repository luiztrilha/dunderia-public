/**
 * Ingests Claude Code context files (CLAUDE.md + memory files) into WUPHF.
 *
 * Reads from both global and project-level locations:
 * - ~/.claude/CLAUDE.md (global instructions)
 * - {cwd}/CLAUDE.md (project instructions)
 * - ~/.claude/projects/{project-key}/memory/*.md (memory files)
 *
 * Uses the file manifest for change detection — unchanged files are skipped.
 */
import { existsSync, readdirSync, statSync, readFileSync } from "node:fs";
import { join, extname, basename, resolve, relative, sep } from "node:path";
import { readManifest, writeManifest, isChanged, markIngested } from "./file-manifest.js";
const INGEST_TIMEOUT_MS = 10_000;
const MAX_FILE_SIZE = 100_000;
function isWithinWorkspace(root, candidate) {
    const relativePath = relative(root, candidate);
    return relativePath === "" || (!relativePath.startsWith("..") && !relativePath.includes(`..${sep}`) && !/^[a-zA-Z]:/.test(relativePath));
}
function addContextFile(files, root, path, contextTag) {
    const resolvedPath = resolve(path);
    if (!isWithinWorkspace(root, resolvedPath))
        return;
    if (existsSync(resolvedPath)) {
        files.push({ path: resolvedPath, contextTag });
    }
}
function collectContextFiles(cwd) {
    cwd = resolve(cwd);
    const files = [];
    addContextFile(files, cwd, join(cwd, "CLAUDE.md"), "claude-md:project");
    addContextFile(files, cwd, join(cwd, "AGENTS.md"), "agents-md:project");
    const memoryDir = resolve(join(cwd, ".wuphf", "memory"));
    if (existsSync(memoryDir)) {
        if (!isWithinWorkspace(cwd, memoryDir))
            return files;
        try {
            const entries = readdirSync(memoryDir, { withFileTypes: true });
            for (const entry of entries) {
                if (!entry.isFile())
                    continue;
                if (extname(entry.name).toLowerCase() !== ".md")
                    continue;
                const fullPath = resolve(join(memoryDir, entry.name));
                if (!isWithinWorkspace(cwd, fullPath))
                    continue;
                const name = basename(entry.name, ".md");
                files.push({ path: fullPath, contextTag: `workspace-memory:${name}` });
            }
        }
        catch {
            // memoryDir unreadable — skip
        }
    }
    return files;
}
export async function ingestContextFiles(client, rateLimiter, cwd) {
    const result = { ingested: 0, skipped: 0, errors: 0, files: [] };
    const manifest = readManifest();
    const candidates = collectContextFiles(cwd);
    let dirty = false;
    for (const { path, contextTag } of candidates) {
        try {
            const stat = statSync(path);
            if (!isChanged(path, stat, manifest)) {
                result.skipped++;
                continue;
            }
            if (!rateLimiter.canProceed()) {
                process.stderr.write("[wuphf-context-files] Rate limited — stopping context file ingest\n");
                result.skipped += candidates.length - result.ingested - result.skipped - result.errors;
                break;
            }
            let content = readFileSync(path, "utf-8");
            if (content.length > MAX_FILE_SIZE) {
                content = content.slice(0, MAX_FILE_SIZE) + "\n[...truncated]";
            }
            await client.post("/v1/context/text", { content, context: contextTag });
            rateLimiter.recordRequest();
            markIngested(path, stat, contextTag, manifest);
            result.ingested++;
            result.files.push(contextTag);
            dirty = true;
        }
        catch (err) {
            process.stderr.write(`[wuphf-context-files] Failed to ingest ${contextTag}: ${err instanceof Error ? err.message : String(err)}\n`);
            result.errors++;
        }
    }
    if (dirty) {
        writeManifest(manifest);
    }
    return result;
}
//# sourceMappingURL=context-files.js.map
