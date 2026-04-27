$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$workspaceRoot = if ($env:WUPHF_MCP_FILESYSTEM_ROOT) {
  $env:WUPHF_MCP_FILESYSTEM_ROOT
} else {
  $repoRoot
}
$package = if ($env:WUPHF_MCP_FILESYSTEM_PACKAGE) {
  $env:WUPHF_MCP_FILESYSTEM_PACKAGE
} else {
  '@modelcontextprotocol/server-filesystem@2026.1.14'
}

& npx "-y" $package $workspaceRoot
