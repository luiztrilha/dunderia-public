$ErrorActionPreference = 'Stop'

$package = if ($env:WUPHF_GITHUB_MCP_PACKAGE) {
    $env:WUPHF_GITHUB_MCP_PACKAGE
} else {
    '@modelcontextprotocol/server-github@2025.4.8'
}

$token = if ($env:WUPHF_GITHUB_MCP_TOKEN) {
    $env:WUPHF_GITHUB_MCP_TOKEN
} elseif ($env:GITHUB_MCP_TOKEN) {
    $env:GITHUB_MCP_TOKEN
} elseif ($env:GITHUB_PERSONAL_ACCESS_TOKEN) {
    $env:GITHUB_PERSONAL_ACCESS_TOKEN
} else {
    ''
}

if ([string]::IsNullOrWhiteSpace($token) -and $env:WUPHF_GITHUB_MCP_USE_GH_TOKEN -eq '1') {
    $gh = Get-Command gh -ErrorAction SilentlyContinue
    if (-not $gh) {
        Write-Error 'GitHub MCP launcher nao encontrou o comando gh no PATH.'
        exit 1
    }
    $token = (& gh auth token 2>$null | Out-String).Trim()
}

if ([string]::IsNullOrWhiteSpace($token)) {
    Write-Error 'GitHub MCP launcher requer um token dedicado em WUPHF_GITHUB_MCP_TOKEN ou GITHUB_MCP_TOKEN. Para fallback explicito via gh auth token, defina WUPHF_GITHUB_MCP_USE_GH_TOKEN=1.'
    exit 1
}

$env:GITHUB_PERSONAL_ACCESS_TOKEN = $token

& npx "-y" $package
