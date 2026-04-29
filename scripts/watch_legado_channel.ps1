param(
  [string]$Channel = 'legado-para-novo',
  [string]$BaseUrl = 'http://127.0.0.1:7890',
  [string]$StatePath = '',
  [string]$LogPath = '',
  [string]$MessagesPath = '',
  [string[]]$AllowedPaths = @(),
  [int]$IntervalMinutes = 10,
  [switch]$AllowBrokerRestart,
  [switch]$AllowMutations,
  [switch]$Watch,
  [switch]$DryRun
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot

function Get-WuphfRuntimeRoot {
  if (-not [string]::IsNullOrWhiteSpace($env:WUPHF_RUNTIME_HOME)) {
    return Join-Path $env:WUPHF_RUNTIME_HOME '.wuphf'
  }
  if (-not [string]::IsNullOrWhiteSpace($env:WUPHF_HOME)) {
    return $env:WUPHF_HOME
  }
  if (-not [string]::IsNullOrWhiteSpace($HOME)) {
    return Join-Path $HOME '.wuphf'
  }
  if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
    return Join-Path (Join-Path $env:LOCALAPPDATA 'DunderIA') '.wuphf'
  }
  return Join-Path (Join-Path $env:TEMP 'DunderIA') '.wuphf'
}

$runtimeRoot = Get-WuphfRuntimeRoot
$teamDir = Join-Path $runtimeRoot 'team'
$processLogDir = Join-Path $teamDir 'process-logs'

if ([string]::IsNullOrWhiteSpace($StatePath)) {
  $StatePath = Join-Path $teamDir 'legado-channel-watch-state.json'
}
if ([string]::IsNullOrWhiteSpace($LogPath)) {
  $LogPath = Join-Path $teamDir 'legado-channel-watch.jsonl'
}
if (-not $AllowedPaths -or $AllowedPaths.Count -eq 0) {
  $AllowedPaths = @(
    'D:\Repos\ConveniosWebBNB_Antigo\BNB',
    'D:\Repos\ConveniosWebBNB_Antigo\WSConvenio',
    'D:\Repos\ConveniosWebBNB_Novo',
    'D:\Repos\ConveniosWebExterno'
  )
}

$repoRootPattern = [regex]::Escape(($repoRoot.TrimEnd('\') -replace '/', '\'))

$outOfScopePatterns = @(
  $repoRootPattern,
  '\bDunderIA\b',
  '\bWUPHF\b',
  'index\.legacy\.html',
  '\bStudioApp\b',
  '\bChannelHeader\b',
  '\bRuntimeStrip\b',
  'web/src'
)

function Get-WuphfExecutablePath {
  if ($env:WUPHF_WATCH_WUPHF_EXE) {
    return $env:WUPHF_WATCH_WUPHF_EXE
  }

  $currentExe = Join-Path $repoRoot 'wuphf-current.exe'
  if (Test-Path -LiteralPath $currentExe) {
    return $currentExe
  }

  return (Join-Path $repoRoot 'wuphf.exe')
}

function Get-WatchState {
  param([string]$Path)

  if (-not (Test-Path -LiteralPath $Path)) {
    return [ordered]@{
      last_message_id = ''
      last_run_at = ''
    }
  }

  return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
}

function Save-WatchState {
  param(
    [string]$Path,
    [hashtable]$State
  )

  $dir = Split-Path -Parent $Path
  if ($dir -and -not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
  }

  $State | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Write-WatchLog {
  param(
    [string]$Path,
    [hashtable]$Record
  )

  $dir = Split-Path -Parent $Path
  if ($dir -and -not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
  }

  ($Record | ConvertTo-Json -Depth 8 -Compress) + [Environment]::NewLine | Add-Content -LiteralPath $Path -Encoding UTF8
}

function Get-BrokerToken {
  param([string]$Root)
  return (Invoke-RestMethod -Uri ($Root.TrimEnd('/') + '/web-token')).token
}

function Invoke-BrokerGet {
  param(
    [string]$Root,
    [string]$Path
  )

  $token = Get-BrokerToken -Root $Root
  $headers = @{ Authorization = "Bearer $token" }
  return Invoke-RestMethod -Headers $headers -Uri ($Root.TrimEnd('/') + $Path)
}

function Invoke-BrokerPost {
  param(
    [string]$Root,
    [string]$Path,
    [hashtable]$Payload
  )

  $token = Get-BrokerToken -Root $Root
  $headers = @{
    Authorization = "Bearer $token"
    'Content-Type' = 'application/json'
  }
  $body = $Payload | ConvertTo-Json -Depth 10
  return Invoke-RestMethod -Headers $headers -Method Post -Uri ($Root.TrimEnd('/') + $Path) -Body $body
}

function Test-BrokerHealthy {
  param([string]$Root)
  try {
    $health = Invoke-RestMethod -Uri ($Root.TrimEnd('/') + '/health')
    return $health.status -eq 'ok'
  } catch {
    return $false
  }
}

function Restart-DunderIA {
  $target = Get-WuphfExecutablePath
  if (-not (Test-Path -LiteralPath $target)) {
    throw "Executavel WUPHF nao encontrado: $target"
  }

  $targetName = [System.IO.Path]::GetFileNameWithoutExtension($target)
  $targetPattern = [regex]::Escape(($target -replace '/', '\'))
  $redirectDir = Join-Path $processLogDir $Channel
  if (-not (Test-Path -LiteralPath $redirectDir)) {
    New-Item -ItemType Directory -Path $redirectDir -Force | Out-Null
  }
  $stdout = Join-Path $redirectDir "$targetName.stdout.log"
  $stderr = Join-Path $redirectDir "$targetName.stderr.log"

  Get-CimInstance Win32_Process |
    Where-Object { $_.Name -like 'wuphf*.exe' -and $_.CommandLine -match $targetPattern } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force }

  Start-Process -FilePath $target -ArgumentList '--no-open' -WorkingDirectory $repoRoot -RedirectStandardOutput $stdout -RedirectStandardError $stderr | Out-Null
  Start-Sleep -Seconds 4
  return (Test-BrokerHealthy -Root $BaseUrl)
}

function Get-ChannelMessages {
  param(
    [string]$Root,
    [string]$TargetChannel,
    [string]$FixturePath
  )

  if ($FixturePath) {
    return Get-Content -LiteralPath $FixturePath -Raw | ConvertFrom-Json
  }

  return Invoke-BrokerGet -Root $Root -Path "/messages?channel=$([uri]::EscapeDataString($TargetChannel))&viewer_slug=human&limit=100"
}

function Get-NewMessages {
  param(
    [object[]]$Messages,
    [string]$LastMessageId
  )

  if (-not $LastMessageId) {
    return @($Messages)
  }

  $found = $false
  $result = New-Object 'System.Collections.Generic.List[object]'
  foreach ($message in @($Messages)) {
    if ($found) {
      $result.Add($message)
      continue
    }
    if ("$($message.id)" -eq $LastMessageId) {
      $found = $true
    }
  }

  if (-not $found) {
    return @($Messages)
  }

  return @($result.ToArray())
}

function Get-DriftMessage {
  param([object[]]$Messages)

  foreach ($message in @($Messages)) {
    $content = [string]$message.content
    $from = [string]$message.from
    if ($from -eq 'nex' -and $content.StartsWith('Correção automática de escopo:')) {
      continue
    }
    foreach ($pattern in $outOfScopePatterns) {
      if ($content -match $pattern) {
        return $message
      }
    }
  }

  return $null
}

function New-CorrectionMessage {
  $paths = ($AllowedPaths -join "`n- ")
  return @"
Correção automática de escopo: este canal deve tratar somente a conversão entre estes caminhos:
- $paths

Desconsiderem referências a DunderIA, WUPHF, StudioApp, index.legacy.html, web/src ou qualquer outro repositório fora desses quatro diretórios. Toda análise e todo plano aqui precisam citar explicitamente qual desses caminhos foi inspecionado.
"@.Trim()
}

function Invoke-WatchRun {
  $state = Get-WatchState -Path $StatePath
  $brokerRestarted = $false

  if ($AllowBrokerRestart -and -not $MessagesPath -and -not (Test-BrokerHealthy -Root $BaseUrl)) {
    $brokerRestarted = Restart-DunderIA
  }

  $payload = Get-ChannelMessages -Root $BaseUrl -TargetChannel $Channel -FixturePath $MessagesPath
  $messages = @($payload.messages)
  $newMessages = Get-NewMessages -Messages $messages -LastMessageId ([string]$state.last_message_id)
  $latestMessageId = ''
  if ($messages.Count -gt 0) {
    $latestMessageId = [string]$messages[-1].id
  }

  $driftMessage = Get-DriftMessage -Messages $newMessages
  $correction = $null
  $correctionPosted = $false

  if ($driftMessage) {
    $correction = New-CorrectionMessage
    if (-not $DryRun -and $AllowMutations) {
      Invoke-BrokerPost -Root $BaseUrl -Path '/messages' -Payload @{
        from = 'nex'
        channel = $Channel
        content = $correction
        reply_to = [string]$driftMessage.id
        tagged = @('ceo','planner','repo-auditor','workflow-architect','builder','fe','pm','reviewer')
      } | Out-Null
      $correctionPosted = $true
    }
  }

  $record = [ordered]@{
    ok = $true
    channel = $Channel
    checked_at = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    latest_message_id = $latestMessageId
    new_message_count = @($newMessages).Count
    drift_detected = ($null -ne $driftMessage)
    drift_message_id = if ($driftMessage) { [string]$driftMessage.id } else { '' }
    correction_message = $correction
    correction_posted = $correctionPosted
    broker_restarted = $brokerRestarted
    restart_allowed = [bool]$AllowBrokerRestart
    repair_actions_allowed = [bool]$AllowMutations
  }

  if (-not $DryRun) {
    Save-WatchState -Path $StatePath -State @{
      last_message_id = $latestMessageId
      last_run_at = $record.checked_at
    }
    Write-WatchLog -Path $LogPath -Record $record
  }

  return $record
}

if ($Watch) {
  while ($true) {
    try {
      $result = Invoke-WatchRun
      $result | ConvertTo-Json -Depth 8 -Compress
    } catch {
      $failure = [ordered]@{
        ok = $false
        channel = $Channel
        checked_at = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
        error = $_.Exception.Message
      }
      if (-not $DryRun) {
        Write-WatchLog -Path $LogPath -Record $failure
      }
      $failure | ConvertTo-Json -Depth 8 -Compress
    }
    Start-Sleep -Seconds ([Math]::Max(120, ($IntervalMinutes * 60)))
  }
}

Invoke-WatchRun | ConvertTo-Json -Depth 8
