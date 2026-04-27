$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$dbPath = if ($env:WUPHF_MEGAMEMORY_DB_PATH) {
  $env:WUPHF_MEGAMEMORY_DB_PATH
} else {
  Join-Path $repoRoot '.megamemory/knowledge.db'
}
$package = if ($env:WUPHF_MEGAMEMORY_PACKAGE) {
  $env:WUPHF_MEGAMEMORY_PACKAGE
} else {
  'megamemory@1.6.1'
}
$dbDir = Split-Path -Parent $dbPath

if ($dbDir -and -not (Test-Path -LiteralPath $dbDir)) {
  New-Item -ItemType Directory -Force -Path $dbDir | Out-Null
}

$env:MEGAMEMORY_DB_PATH = $dbPath

& npx "-y" $package
