$ErrorActionPreference = 'Stop'

$package = if ($env:WUPHF_BRAVE_MCP_PACKAGE) {
  $env:WUPHF_BRAVE_MCP_PACKAGE
} else {
  '@modelcontextprotocol/server-brave-search@0.6.2'
}

$configPath = if ($env:WUPHF_CONFIG_PATH) {
  $env:WUPHF_CONFIG_PATH
} else {
  Join-Path $HOME '.wuphf\config.json'
}

$provider = if ($env:WUPHF_WEB_SEARCH_PROVIDER) {
  $env:WUPHF_WEB_SEARCH_PROVIDER
} elseif ($env:WEB_SEARCH_PROVIDER) {
  $env:WEB_SEARCH_PROVIDER
} elseif (Test-Path -LiteralPath $configPath) {
  try {
    $cfg = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
    [string]$cfg.web_search_provider
  } catch {
    ''
  }
} else {
  ''
}

if ([string]::IsNullOrWhiteSpace($provider)) {
  $provider = 'none'
}

if ($provider -ne 'brave') {
  Write-Error "Brave MCP launcher esta desabilitado porque web_search_provider=$provider."
  exit 1
}

$key = if ($env:WUPHF_BRAVE_API_KEY) {
  $env:WUPHF_BRAVE_API_KEY
} elseif ($env:BRAVE_API_KEY) {
  $env:BRAVE_API_KEY
} elseif (Test-Path -LiteralPath $configPath) {
  try {
    $cfg = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
    [string]$cfg.brave_api_key
  } catch {
    ''
  }
} else {
  ''
}

if ([string]::IsNullOrWhiteSpace($key)) {
  Write-Error 'Brave MCP launcher nao encontrou BRAVE_API_KEY nem brave_api_key no config.json.'
  exit 1
}

$env:BRAVE_API_KEY = $key

& npx "-y" $package
