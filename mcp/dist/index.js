#!/usr/bin/env node
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { createServer } from "./server.js";
import { createServer as createHttpServer } from "node:http";
import { loadApiKey } from "./config.js";
const apiKey = loadApiKey();
if (!apiKey) {
    console.error("No API key found (checked WUPHF_API_KEY env and ~/.wuphf/config.json). Starting in registration-only mode. Use the 'register' tool to create an account and get an API key. Once registered, all context, search, and scan tools become available.");
}
const transport = process.env.MCP_TRANSPORT ?? "stdio";
function readHttpToken() {
    return process.env.WUPHF_MCP_HTTP_TOKEN || process.env.MCP_HTTP_TOKEN || "";
}
function isLoopbackHost(host) {
    return host === "127.0.0.1" || host === "::1" || host === "localhost";
}
function authorizeHttpRequest(req, expectedToken) {
    const authHeader = req.headers.authorization || "";
    return authHeader === `Bearer ${expectedToken}`;
}
async function main() {
    const server = createServer(apiKey);
    if (transport === "http") {
        const port = parseInt(process.env.MCP_PORT ?? "3001", 10);
        const host = process.env.MCP_HOST || process.env.WUPHF_MCP_HOST || "127.0.0.1";
        const httpToken = readHttpToken();
        if (!isLoopbackHost(host) && process.env.WUPHF_MCP_HTTP_ALLOW_REMOTE !== "1") {
            throw new Error(`Refusing to bind MCP HTTP server to non-loopback host '${host}'. Set MCP_HOST=127.0.0.1 or WUPHF_MCP_HTTP_ALLOW_REMOTE=1 if this is intentional.`);
        }
        if (!httpToken) {
            throw new Error("MCP HTTP transport requires WUPHF_MCP_HTTP_TOKEN (or MCP_HTTP_TOKEN). Set a dedicated local token before starting MCP_TRANSPORT=http.");
        }
        const httpTransport = new StreamableHTTPServerTransport({
            sessionIdGenerator: undefined,
        });
        const httpServer = createHttpServer(async (req, res) => {
            const url = new URL(req.url ?? "/", `http://${host}:${port}`);
            if (url.pathname === "/mcp") {
                if (!authorizeHttpRequest(req, httpToken)) {
                    res.writeHead(401, { "Content-Type": "application/json" });
                    res.end(JSON.stringify({ error: "unauthorized" }));
                    return;
                }
                await httpTransport.handleRequest(req, res);
            }
            else if (url.pathname === "/health") {
                res.writeHead(200, { "Content-Type": "application/json" });
                res.end(JSON.stringify({ status: "ok" }));
            }
            else {
                res.writeHead(404);
                res.end("Not found");
            }
        });
        await server.connect(httpTransport);
        httpServer.listen(port, host, () => {
            console.error(`WUPHF MCP server running on http://${host}:${port}/mcp`);
        });
    }
    else {
        const stdioTransport = new StdioServerTransport();
        await server.connect(stdioTransport);
        console.error("WUPHF MCP server running on stdio");
    }
}
main().catch((err) => {
    console.error("Fatal error:", err);
    process.exit(1);
});
//# sourceMappingURL=index.js.map
