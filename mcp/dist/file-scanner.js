/**
 * Core file scanner — walks project directories, detects changed files,
 * and ingests them into WUPHF via the developer API.
 */
import { readdirSync, statSync, readFileSync } from "node:fs";
import { join, relative, extname, resolve, sep } from "node:path";
import { readManifest, writeManifest, isChanged, markIngested } from "./file-manifest.js";
function isWithinWorkspace(root, candidate) {
    const relativePath = relative(root, candidate);
    return relativePath === "" || (!relativePath.startsWith("..") && !relativePath.includes(`..${sep}`) && !/^[a-zA-Z]:/.test(relativePath));
}
function walkDir(dir, cwd, config, depth, results) {
    if (depth > config.scanDepth)
        return;
    const resolvedDir = resolve(dir);
    if (!isWithinWorkspace(cwd, resolvedDir))
        return;
    let entries;
    try {
        entries = readdirSync(resolvedDir, { withFileTypes: true });
    }
    catch {
        return;
    }
    for (const entry of entries) {
        const fullPath = resolve(join(resolvedDir, entry.name));
        if (!isWithinWorkspace(cwd, fullPath))
            continue;
        if (entry.isDirectory()) {
            if (config.ignoreDirs.includes(entry.name))
                continue;
            walkDir(fullPath, cwd, config, depth + 1, results);
        }
        else if (entry.isFile()) {
            const ext = extname(entry.name).toLowerCase();
            if (!config.extensions.includes(ext))
                continue;
            try {
                const stat = statSync(fullPath);
                results.push({ absolutePath: fullPath, relativePath: relative(cwd, fullPath), stat });
            }
            catch {
                // stat failed — skip
            }
        }
    }
}
export async function scanAndIngest(client, rateLimiter, cwd, config) {
    const result = { scanned: 0, ingested: 0, skipped: 0, errors: 0 };
    if (!config.enabled)
        return result;
    cwd = resolve(cwd);
    const manifest = readManifest();
    const candidates = [];
    walkDir(cwd, cwd, config, 0, candidates);
    result.scanned = candidates.length;
    const changed = candidates
        .filter((f) => isChanged(f.absolutePath, f.stat, manifest))
        .sort((a, b) => b.stat.mtimeMs - a.stat.mtimeMs)
        .slice(0, config.maxFilesPerScan);
    result.skipped = candidates.length - changed.length;
    for (const file of changed) {
        if (!rateLimiter.canProceed()) {
            process.stderr.write(`[wuphf-scan] Rate limited — stopping after ${result.ingested} files\n`);
            result.skipped += changed.length - result.ingested - result.errors;
            break;
        }
        try {
            let content = readFileSync(file.absolutePath, "utf-8");
            if (content.length > config.maxFileSize) {
                content = content.slice(0, config.maxFileSize) + "\n[...truncated]";
            }
            const context = `file-scan:${file.relativePath}`;
            await client.post("/v1/context/text", { content, context });
            rateLimiter.recordRequest();
            markIngested(file.absolutePath, file.stat, context, manifest);
            result.ingested++;
        }
        catch (err) {
            process.stderr.write(`[wuphf-scan] Failed to ingest ${file.relativePath}: ${err instanceof Error ? err.message : String(err)}\n`);
            result.errors++;
        }
    }
    writeManifest(manifest);
    return result;
}
//# sourceMappingURL=file-scanner.js.map
