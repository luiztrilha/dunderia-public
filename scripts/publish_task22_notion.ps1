param(
  [Parameter(Mandatory = $true)]
  [string]$PageId,
  [string]$PageUrl,
  [string]$ProbeBlockId,
  [string]$ConfigPath = (Join-Path $HOME '.wuphf\config.json'),
  [string]$StatePath = (Join-Path $HOME '.wuphf\team\broker-state.json.last-good'),
  [string]$MessageId = 'msg-315',
  [string]$OneSecret = $env:ONE_SECRET,
  [string]$ConnectionKey = $env:WUPHF_ONE_CONNECTION_KEY,
  [string]$AppendActionId = $env:WUPHF_ONE_NOTION_APPEND_ACTION_ID,
  [string]$DeleteActionId = $env:WUPHF_ONE_NOTION_DELETE_ACTION_ID,
  [switch]$DryRun
)

$ErrorActionPreference = 'Stop'

function Read-JsonFile {
  param([string]$Path)

  if ([string]::IsNullOrWhiteSpace($Path)) {
    throw 'Path is required.'
  }
  if (-not (Test-Path -LiteralPath $Path)) {
    throw "File not found: $Path"
  }
  return (Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json)
}

if (-not $OneSecret -and (Test-Path -LiteralPath $ConfigPath)) {
  $config = Read-JsonFile -Path $ConfigPath
  $OneSecret = $config.one_api_key
}
if (-not $OneSecret) {
  throw 'OneSecret is required. Pass -OneSecret or configure ONE_SECRET / config.one_api_key.'
}
if (-not $ConnectionKey) {
  throw 'ConnectionKey is required. Pass -ConnectionKey or configure WUPHF_ONE_CONNECTION_KEY.'
}
if (-not $AppendActionId) {
  throw 'AppendActionId is required. Pass -AppendActionId or configure WUPHF_ONE_NOTION_APPEND_ACTION_ID.'
}
if ($ProbeBlockId -and -not $DeleteActionId) {
  throw 'DeleteActionId is required when ProbeBlockId is provided.'
}

$apiBase = 'https://api.withone.ai/v1/passthrough'

$state = Read-JsonFile -Path $StatePath
$message = $state.messages | Where-Object { $_.id -eq $MessageId } | Select-Object -First 1
if (-not $message) {
  throw "Message '$MessageId' was not found in state file '$StatePath'."
}
$content = $message.content
if ([string]::IsNullOrWhiteSpace($content)) {
  throw "Message '$MessageId' has no content."
}
$lines = $content -split "`r?`n"

function New-RichTextItem {
  param([string]$Text)
  return @{
    type = 'text'
    text = @{
      content = $Text
    }
  }
}

function Split-TextChunks {
  param(
    [string]$Text,
    [int]$Max = 1800
  )

  $chunks = New-Object System.Collections.Generic.List[string]
  $remaining = $Text.Trim()
  while ($remaining.Length -gt $Max) {
    $cut = $remaining.LastIndexOf(' ', $Max)
    if ($cut -lt 200) {
      $cut = $Max
    }
    $chunks.Add($remaining.Substring(0, $cut).Trim())
    $remaining = $remaining.Substring($cut).Trim()
  }
  if ($remaining.Length -gt 0) {
    $chunks.Add($remaining)
  }
  return $chunks
}

function New-ParagraphBlocks {
  param([string]$Text)
  $items = New-Object System.Collections.Generic.List[object]
  foreach ($chunk in Split-TextChunks -Text $Text) {
    $items.Add(@{
        object    = 'block'
        type      = 'paragraph'
        paragraph = @{
          rich_text = @(New-RichTextItem -Text $chunk)
          color     = 'default'
        }
      })
  }
  return $items
}

function New-BulletBlocks {
  param(
    [string]$Text,
    [bool]$Todo = $false
  )
  $items = New-Object System.Collections.Generic.List[object]
  foreach ($chunk in Split-TextChunks -Text $Text) {
    if ($Todo) {
      $items.Add(@{
          object = 'block'
          type   = 'to_do'
          to_do  = @{
            rich_text = @(New-RichTextItem -Text $chunk)
            checked   = $false
            color     = 'default'
          }
        })
    } else {
      $items.Add(@{
          object             = 'block'
          type               = 'bulleted_list_item'
          bulleted_list_item = @{
            rich_text = @(New-RichTextItem -Text $chunk)
            color     = 'default'
          }
        })
    }
  }
  return $items
}

function New-HeadingBlock {
  param(
    [string]$Text,
    [ValidateSet(1, 2)]
    [int]$Level
  )
  $type = if ($Level -eq 1) { 'heading_1' } else { 'heading_2' }
  $payload = @{
    rich_text     = @(New-RichTextItem -Text $Text)
    color         = 'default'
    is_toggleable = $false
  }
  $block = @{
    object = 'block'
    type   = $type
  }
  $block[$type] = $payload
  return $block
}

function New-CodeBlock {
  param([string]$Text)
  return @{
    object = 'block'
    type   = 'code'
    code   = @{
      rich_text = @(New-RichTextItem -Text $Text)
      language  = 'markdown'
      caption   = @()
    }
  }
}

$blocks = New-Object System.Collections.Generic.List[object]
$currentSection = ''
$tableBuffer = New-Object System.Collections.Generic.List[string]
$firstHeadingSkipped = $false

function Flush-TableBuffer {
  param(
    [System.Collections.Generic.List[string]]$Buffer,
    [System.Collections.Generic.List[object]]$BlockList
  )
  if ($Buffer.Count -gt 0) {
    $tableText = ($Buffer -join "`n")
    $BlockList.Add((New-CodeBlock -Text $tableText))
    $Buffer.Clear()
  }
}

foreach ($line in $lines) {
  $trimmed = $line.TrimEnd()
  if ($trimmed -match '^\|') {
    $tableBuffer.Add($trimmed)
    continue
  }

  Flush-TableBuffer -Buffer $tableBuffer -BlockList $blocks

  if ([string]::IsNullOrWhiteSpace($trimmed)) {
    continue
  }

  if ($trimmed.StartsWith('# ')) {
    if (-not $firstHeadingSkipped) {
      $firstHeadingSkipped = $true
      continue
    }
    $currentSection = $trimmed.Substring(2).Trim()
    $blocks.Add((New-HeadingBlock -Text $currentSection -Level 1))
    continue
  }

  if ($trimmed.StartsWith('## ')) {
    $currentSection = $trimmed.Substring(3).Trim()
    $blocks.Add((New-HeadingBlock -Text $currentSection -Level 2))
    continue
  }

  if ($trimmed.StartsWith('- ')) {
    $todo = $currentSection -eq 'Immediate Next Actions'
    foreach ($block in New-BulletBlocks -Text $trimmed.Substring(2).Trim() -Todo:$todo) {
      $blocks.Add($block)
    }
    continue
  }

  foreach ($block in New-ParagraphBlocks -Text $trimmed) {
    $blocks.Add($block)
  }
}

Flush-TableBuffer -Buffer $tableBuffer -BlockList $blocks

if ($DryRun) {
  [ordered]@{
    page_id          = $PageId
    url              = $PageUrl
    block_count      = $blocks.Count
    connection_key   = $ConnectionKey
    append_action_id = $AppendActionId
    delete_action_id = $DeleteActionId
    state_path       = $StatePath
    message_id       = $MessageId
    dry_run          = $true
  } | ConvertTo-Json -Depth 8
  return
}

$batchSize = 12
$batchResults = New-Object System.Collections.Generic.List[object]

for ($i = 0; $i -lt $blocks.Count; $i += $batchSize) {
  $upper = [Math]::Min($i + $batchSize - 1, $blocks.Count - 1)
  $batch = @($blocks[$i..$upper])
  $body = @{
    children = $batch
  } | ConvertTo-Json -Depth 12 -Compress
  $headers = @{
    'x-one-secret' = $OneSecret
    'x-one-connection-key' = $ConnectionKey
    'x-one-action-id' = $AppendActionId
    'Content-Type' = 'application/json'
  }
  $parsed = Invoke-RestMethod -Method Patch -Uri "$apiBase/blocks/$PageId/children" -Headers $headers -Body $body
  $batchResults.Add(@{
      batch        = ($i / $batchSize) + 1
      count        = $batch.Count
      request_id   = $parsed.request_id
      result_count = $parsed.results.Count
    })
}

$deleteRequestId = $null
if ($ProbeBlockId) {
  $deleteHeaders = @{
    'x-one-secret' = $OneSecret
    'x-one-connection-key' = $ConnectionKey
    'x-one-action-id' = $DeleteActionId
    'Content-Type' = 'application/json'
  }
  $deleteResponse = Invoke-RestMethod -Method Delete -Uri "$apiBase/blocks/$ProbeBlockId" -Headers $deleteHeaders
  if ($deleteResponse -and $deleteResponse.request_id) {
    $deleteRequestId = $deleteResponse.request_id
  }
}

[ordered]@{
  page_id        = $PageId
  url            = $PageUrl
  block_count    = $blocks.Count
  batches        = $batchResults
  delete_request = $deleteRequestId
} | ConvertTo-Json -Depth 8
