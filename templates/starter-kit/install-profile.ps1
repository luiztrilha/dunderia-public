param(
  [string]$HomeDir = $HOME,
  [switch]$OverwriteConfig
)

$ErrorActionPreference = "Stop"

$kit = Split-Path -Parent $MyInvocation.MyCommand.Path
$codexHome = Join-Path $HomeDir ".codex"
$agentsHome = Join-Path $HomeDir ".agents"
$claudeHome = Join-Path $HomeDir ".claude"
$opencodeHome = Join-Path (Join-Path $HomeDir ".config") "opencode"

function Copy-Tree($Source, $Destination) {
  if (-not (Test-Path -LiteralPath $Source)) {
    return
  }
  New-Item -ItemType Directory -Force (Split-Path -Parent $Destination) | Out-Null
  if (Test-Path -LiteralPath $Destination) {
    Remove-Item -LiteralPath $Destination -Recurse -Force
  }
  Copy-Item -LiteralPath $Source -Destination $Destination -Recurse -Force
}

function Copy-Children($Source, $Destination) {
  if (-not (Test-Path -LiteralPath $Source)) {
    return
  }
  New-Item -ItemType Directory -Force $Destination | Out-Null
  Get-ChildItem -LiteralPath $Source | ForEach-Object {
    Copy-Tree $_.FullName (Join-Path $Destination $_.Name)
  }
}

New-Item -ItemType Directory -Force $codexHome | Out-Null
New-Item -ItemType Directory -Force $agentsHome | Out-Null
New-Item -ItemType Directory -Force $claudeHome | Out-Null
New-Item -ItemType Directory -Force $opencodeHome | Out-Null

Copy-Children (Join-Path $kit "codex\skills") (Join-Path $codexHome "skills")
Copy-Children (Join-Path $kit "codex\superpowers\skills") (Join-Path $codexHome "superpowers\skills")
Copy-Children (Join-Path $kit "prompts") (Join-Path $codexHome "prompts")
Copy-Children (Join-Path $kit "rules") (Join-Path $codexHome "rules")
Copy-Children (Join-Path $kit "agents\skills") (Join-Path $agentsHome "skills")
Copy-Children (Join-Path $kit "claude\commands") (Join-Path $claudeHome "commands")
Copy-Children (Join-Path $kit "opencode\skills") (Join-Path $opencodeHome "skills")

Copy-Item -LiteralPath (Join-Path $kit "codex\AGENTS.validated.md") -Destination (Join-Path $codexHome "AGENTS.md") -Force
Copy-Item -LiteralPath (Join-Path $kit "agents\skill-lock.json") -Destination (Join-Path $agentsHome ".skill-lock.json") -Force
Copy-Item -LiteralPath (Join-Path $kit "policies.validated.md") -Destination (Join-Path $codexHome "policies.validated.md") -Force

$configTarget = Join-Path $codexHome "config.toml"
if ($OverwriteConfig -or -not (Test-Path -LiteralPath $configTarget)) {
  Copy-Item -LiteralPath (Join-Path $kit "codex\config.sanitized.toml") -Destination $configTarget -Force
} else {
  Write-Host "Skipped existing config.toml. Review codex/config.sanitized.toml and merge manually."
}

Write-Host "Installed DunderIA validated profile into $HomeDir"
