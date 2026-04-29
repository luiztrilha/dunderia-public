param(
  [string[]]$Package = @("./..."),
  [string[]]$GoTestArg = @(),
  [switch]$KeepTemp
)

$ErrorActionPreference = "Stop"

$repoRoot = (& git rev-parse --show-toplevel 2>$null).Trim()
if (-not $repoRoot) {
  throw "Not inside a git repository."
}

$stableRoot = Join-Path $repoRoot (".tmp-stable-test-" + [guid]::NewGuid().ToString("N"))
$homeDir = Join-Path $stableRoot "home"
$taskLogRoot = Join-Path $stableRoot "task-logs"
$goTmp = Join-Path $stableRoot "go-tmp"
$goCache = Join-Path $stableRoot "go-build"

New-Item -ItemType Directory -Force -Path $homeDir, $taskLogRoot, $goTmp, $goCache | Out-Null

$oldRuntimeHome = $env:WUPHF_RUNTIME_HOME
$oldTaskLogRoot = $env:WUPHF_TASK_LOG_ROOT
$oldGoTmp = $env:GOTMPDIR
$oldGoCache = $env:GOCACHE
$oldWuphfBrokerToken = $env:WUPHF_BROKER_TOKEN
$oldNexBrokerToken = $env:NEX_BROKER_TOKEN

try {
  $env:WUPHF_RUNTIME_HOME = $homeDir
  $env:WUPHF_TASK_LOG_ROOT = $taskLogRoot
  $env:GOTMPDIR = $goTmp
  $env:GOCACHE = $goCache
  $env:WUPHF_BROKER_TOKEN = "stable-test-token"
  $env:NEX_BROKER_TOKEN = ""

  Write-Output "Stable test home: $homeDir"
  & go test @Package @GoTestArg
  exit $LASTEXITCODE
} finally {
  $env:WUPHF_RUNTIME_HOME = $oldRuntimeHome
  $env:WUPHF_TASK_LOG_ROOT = $oldTaskLogRoot
  $env:GOTMPDIR = $oldGoTmp
  $env:GOCACHE = $oldGoCache
  $env:WUPHF_BROKER_TOKEN = $oldWuphfBrokerToken
  $env:NEX_BROKER_TOKEN = $oldNexBrokerToken

  if ($KeepTemp) {
    Write-Output "Kept stable test directory: $stableRoot"
  } elseif (Test-Path -LiteralPath $stableRoot) {
    Remove-Item -LiteralPath $stableRoot -Recurse -Force
  }
}
