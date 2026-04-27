param(
  [string]$Channel = 'ExampleWorkflow-legacy',
  [string]$BaseUrl = 'http://127.0.0.1:7890',
  [string]$Announcer = 'ceo',
  [string]$StatePath = '',
  [string]$LogPath = '',
  [string]$SnapshotPath = '',
  [string]$MessagesPath = '',
  [string]$TasksPath = '',
  [string[]]$PreferredWorkspaceRoots = @(),
  [int]$IntervalMinutes = 5,
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
  $StatePath = Join-Path $teamDir 'ExampleWorkflow-legacy-watch-state.json'
}
if ([string]::IsNullOrWhiteSpace($LogPath)) {
  $LogPath = Join-Path $teamDir 'ExampleWorkflow-legacy-watch.jsonl'
}
if ([string]::IsNullOrWhiteSpace($SnapshotPath)) {
  $SnapshotPath = Join-Path $teamDir 'ExampleWorkflow-legacy-snapshot.json'
}
if (-not $PreferredWorkspaceRoots -or $PreferredWorkspaceRoots.Count -eq 0) {
  $PreferredWorkspaceRoots = @(
    '<REPOS_ROOT>\LegacySystemOld',
    '<REPOS_ROOT>\LegacySystemOld'
  )
}

$repoRootPattern = [regex]::Escape(($repoRoot.TrimEnd('\') -replace '/', '\'))

$badWorkspacePatterns = @(
  '\\\.wuphf($|\\)',
  "$repoRootPattern($|\\)"
)

$workspaceMentionPattern = '(?i)[a-z]:\\[^\s"''`,;\)\]]+'

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

function Save-WatchSnapshot {
  param(
    [string]$Path,
    [hashtable]$Snapshot
  )

  $dir = Split-Path -Parent $Path
  if ($dir -and -not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
  }

  $Snapshot | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $Path -Encoding UTF8
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
  $targetPattern = [regex]::Escape(($target -replace '/', '\'))
  $targetName = [System.IO.Path]::GetFileNameWithoutExtension($target)
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

function Get-ChannelTasks {
  param(
    [string]$Root,
    [string]$TargetChannel,
    [string]$FixturePath
  )

  if ($FixturePath) {
    return Get-Content -LiteralPath $FixturePath -Raw | ConvertFrom-Json
  }

  return Invoke-BrokerGet -Root $Root -Path "/tasks?channel=$([uri]::EscapeDataString($TargetChannel))&viewer_slug=human&include_done=true"
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

function Test-SameWorkspacePath {
  param(
    [string]$Left,
    [string]$Right
  )

  $leftClean = [string]$Left
  $rightClean = [string]$Right
  if ([string]::IsNullOrWhiteSpace($leftClean) -or [string]::IsNullOrWhiteSpace($rightClean)) {
    return $false
  }

  $normalize = {
    param([string]$Value)
    return ($Value.TrimEnd('\') -replace '/', '\').ToLowerInvariant()
  }

  return (& $normalize $leftClean) -eq (& $normalize $rightClean)
}

function Get-GitWorkspaceRoot {
  param([string]$Path)

  $candidate = [string]$Path
  if ([string]::IsNullOrWhiteSpace($candidate)) {
    return ''
  }

  if (Test-Path -LiteralPath $candidate -PathType Leaf) {
    $candidate = Split-Path -Parent $candidate
  }

  while ($candidate) {
    if (Test-Path -LiteralPath (Join-Path $candidate '.git')) {
      return $candidate
    }
    $parent = Split-Path -Parent $candidate
    if (-not $parent -or $parent -eq $candidate) {
      break
    }
    $candidate = $parent
  }

  return ''
}

function Resolve-ExpectedWorkspacePath {
  param([object[]]$Messages)

  $orderedMessages = @($Messages)
  [array]::Reverse($orderedMessages)

  foreach ($message in $orderedMessages) {
    $content = [string]$message.content

    $matches = @([regex]::Matches($content, $workspaceMentionPattern))
    [array]::Reverse($matches)
    foreach ($match in $matches) {
      $candidate = ([string]$match.Value).TrimEnd('.', ':')
      if (-not (Test-Path -LiteralPath $candidate)) {
        continue
      }
      if (Split-Path -Leaf $candidate | Where-Object { $_ -like '.wuphf' -or $_ -like 'dunderia' }) {
        continue
      }
      $gitRoot = Get-GitWorkspaceRoot -Path $candidate
      if (-not $gitRoot) {
        continue
      }
      if (Test-Path -LiteralPath $candidate -PathType Container) {
        return $candidate
      }
      $dir = Split-Path -Parent $candidate
      if ($dir -and (Test-Path -LiteralPath $dir -PathType Container)) {
        return $dir
      }
      return $gitRoot
    }
  }

  foreach ($preferred in $PreferredWorkspaceRoots) {
    if (Test-Path -LiteralPath $preferred) {
      return $preferred
    }
  }

  return ''
}

function Test-BadWorkspacePath {
  param([string]$Path)

  $clean = [string]$Path
  if ([string]::IsNullOrWhiteSpace($clean)) {
    return $true
  }

  foreach ($pattern in $badWorkspacePatterns) {
    if ($clean -match $pattern) {
      return $true
    }
  }

  return -not (Get-GitWorkspaceRoot -Path $clean)
}

function Find-WorkspaceDriftTask {
  param(
    [object[]]$Tasks,
    [string]$ExpectedWorkspace
  )

  foreach ($task in @($Tasks)) {
    $status = [string]$task.status
    if ($status -in @('done', 'canceled', 'cancelled')) {
      continue
    }
    if ([string]$task.execution_mode -ne 'external_workspace') {
      continue
    }
    $taskWorkspace = [string]$task.workspace_path
    if (Test-BadWorkspacePath -Path $taskWorkspace) {
      return $task
    }
    if ($ExpectedWorkspace -and -not (Test-SameWorkspacePath -Left $taskWorkspace -Right $ExpectedWorkspace)) {
      return $task
    }
  }

  return $null
}

function Find-LoopPair {
  param([object[]]$Messages)

  $seen = @{}
  foreach ($message in @($Messages)) {
    $from = [string]$message.from
    if ($from -in @('human', 'you', 'nex', 'system', '')) {
      continue
    }
    $content = ([string]$message.content).Trim()
    if (-not $content) {
      continue
    }
    $signature = "$from|$([string]$message.reply_to)|$($content -replace '\s+', ' ')"
    if ($seen.ContainsKey($signature)) {
      return @($seen[$signature], $message)
    }
    $seen[$signature] = $message
  }

  return @()
}

function New-CorrectionMessage {
  param(
    [object]$Task,
    [string]$ExpectedWorkspace,
    [object[]]$LoopPair
  )

  $currentWorkspace = [string]$Task.workspace_path
  $lines = @(
    "Correção automática de execução:",
    "- a task ativa estava apontando para um workspace inválido ($currentWorkspace), o que gerou revisão no repo errado, bloqueio artificial e handoffs em falso.",
    "- o workspace correto para este slice é $ExpectedWorkspace.",
    "- reusem a task já aberta; não reabram a frente Base64 nem peçam novo alvo enquanto este slice não tiver diff real no legado."
  )

  if ($LoopPair.Count -gt 1) {
    $lines += "- foi detectada repetição idêntica de resposta no mesmo thread; parem de reenfileirar o mesmo parecer/status sem delta novo."
  }

  $lines += "Contrato operacional: mutate a task antes de narrar estado, trabalhe somente no repo legado correto e só chame revisão quando houver diff real."
  return ($lines -join "`n")
}

function Repair-WorkspaceTask {
  param(
    [string]$Root,
    [object]$Task,
    [string]$WorkspacePath
  )

  $owner = [string]$task.owner
  if ([string]::IsNullOrWhiteSpace($owner)) {
    $owner = 'builder'
  }

  $details = ([string]$task.details).Trim()
  $repairNote = "Automatic channel recovery: workspace_path corrigido para $WorkspacePath."
  if ($details -notmatch [regex]::Escape($repairNote)) {
    if ($details) {
      $details += "`n`n"
    }
    $details += $repairNote
  }

  return Invoke-BrokerPost -Root $Root -Path '/tasks' -Payload @{
    action = 'reassign'
    id = [string]$task.id
    channel = $Channel
    owner = $owner
    execution_mode = 'external_workspace'
    workspace_path = $WorkspacePath
    details = $details
    created_by = $Announcer
  }
}

function Invoke-WatchRun {
  $state = Get-WatchState -Path $StatePath
  $brokerRestarted = $false

  if ($AllowBrokerRestart -and -not $MessagesPath -and -not $TasksPath -and -not (Test-BrokerHealthy -Root $BaseUrl)) {
    $brokerRestarted = Restart-DunderIA
  }

  $messagePayload = Get-ChannelMessages -Root $BaseUrl -TargetChannel $Channel -FixturePath $MessagesPath
  $taskPayload = Get-ChannelTasks -Root $BaseUrl -TargetChannel $Channel -FixturePath $TasksPath
  $messages = @($messagePayload.messages)
  $tasks = @($taskPayload.tasks)
  $newMessages = Get-NewMessages -Messages $messages -LastMessageId ([string]$state.last_message_id)
  $latestMessageId = ''
  if ($messages.Count -gt 0) {
    $latestMessageId = [string]$messages[-1].id
  }

  $expectedWorkspace = Resolve-ExpectedWorkspacePath -Messages $messages
  $driftTask = Find-WorkspaceDriftTask -Tasks $tasks -ExpectedWorkspace $expectedWorkspace
  $loopPair = Find-LoopPair -Messages $newMessages
  $correction = $null
  $correctionPosted = $false
  $taskRepaired = $false

  if ($driftTask -and $expectedWorkspace) {
    $correction = New-CorrectionMessage -Task $driftTask -ExpectedWorkspace $expectedWorkspace -LoopPair $loopPair
    if (-not $DryRun -and $AllowMutations) {
      Repair-WorkspaceTask -Root $BaseUrl -Task $driftTask -WorkspacePath $expectedWorkspace | Out-Null
      $taskRepaired = $true
      Invoke-BrokerPost -Root $BaseUrl -Path '/messages' -Payload @{
        from = $Announcer
        channel = $Channel
        content = $correction
        reply_to = if ($messages.Count -gt 0) { [string]$messages[-1].id } else { '' }
        tagged = @('ceo','operator','builder','reviewer')
      } | Out-Null
      $correctionPosted = $true
    }
  } elseif ($loopPair.Count -gt 1) {
    $correction = New-CorrectionMessage -Task ([pscustomobject]@{ workspace_path = '' }) -ExpectedWorkspace $expectedWorkspace -LoopPair $loopPair
    if (-not $DryRun -and $AllowMutations) {
      Invoke-BrokerPost -Root $BaseUrl -Path '/messages' -Payload @{
        from = $Announcer
        channel = $Channel
        content = $correction
        reply_to = [string]$loopPair[-1].id
        tagged = @('ceo','operator','builder','reviewer')
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
    broker_restarted = $brokerRestarted
    restart_allowed = [bool]$AllowBrokerRestart
    workspace_repair_needed = ($null -ne $driftTask)
    workspace_repaired = $taskRepaired
    repair_actions_allowed = [bool]$AllowMutations
    repaired_task_id = if ($driftTask) { [string]$driftTask.id } else { '' }
    repaired_workspace_path = $expectedWorkspace
    loop_detected = ($loopPair.Count -gt 1)
    correction_message = $correction
    correction_posted = $correctionPosted
  }

  if (-not $DryRun) {
    Save-WatchSnapshot -Path $SnapshotPath -Snapshot @{
      channel = $Channel
      checked_at = $record.checked_at
      messages = $messages
      tasks = $tasks
    }
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
