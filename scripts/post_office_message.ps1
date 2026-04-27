param(
  [Parameter(Mandatory = $true)]
  [string]$Content,
  [string]$Channel = 'general',
  [string]$From = 'you',
  [string[]]$Tagged = @(),
  [string]$ReplyTo = '',
  [string]$BaseUrl = 'http://127.0.0.1:7891',
  [switch]$DryRun
)

$ErrorActionPreference = 'Stop'

Add-Type -AssemblyName System.Net.Http

$client = [System.Net.Http.HttpClient]::new()

function Get-TaggedSlugsFromContent {
  param(
    [string]$BaseUrl,
    [string]$Content,
    [string[]]$ExplicitTagged
  )

  if ($ExplicitTagged -and $ExplicitTagged.Count -gt 0) {
    return @($ExplicitTagged | ForEach-Object { "$_".Trim().ToLowerInvariant() } | Where-Object { $_ })
  }

  $membersResponse = $client.GetAsync("$BaseUrl/api/office-members").GetAwaiter().GetResult()
  if (-not $membersResponse.IsSuccessStatusCode) {
    return @()
  }

  $membersBody = $membersResponse.Content.ReadAsStringAsync().GetAwaiter().GetResult()
  $membersPayload = $membersBody | ConvertFrom-Json
  $known = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
  foreach ($member in @($membersPayload.members)) {
    $slug = "$($member.slug)".Trim().ToLowerInvariant()
    if ($slug) {
      [void]$known.Add($slug)
    }
  }

  $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
  $tagged = New-Object System.Collections.Generic.List[string]
  $matches = [regex]::Matches($Content, '@(\S+)')
  foreach ($match in $matches) {
    $slug = $match.Groups[1].Value.ToLowerInvariant() -replace '[^a-z0-9-]', ''
    if (-not $slug) { continue }
    if (-not $known.Contains($slug)) { continue }
    if (-not $seen.Add($slug)) { continue }
    $tagged.Add($slug)
  }

  return @($tagged)
}

$resolvedTagged = Get-TaggedSlugsFromContent -BaseUrl $BaseUrl -Content $Content -ExplicitTagged $Tagged

$payload = @{
  from    = $From
  channel = $Channel
  content = $Content
}
if ($resolvedTagged -and $resolvedTagged.Count -gt 0) {
  $payload.tagged = @($resolvedTagged)
}
if ($ReplyTo) {
  $payload.reply_to = $ReplyTo
}

$json = $payload | ConvertTo-Json -Depth 8 -Compress
$json = [string]$json

if ($DryRun) {
  [pscustomobject]@{
    json   = $json
    tagged = @($resolvedTagged)
  } | ConvertTo-Json -Depth 8
  return
}

$body = [System.Net.Http.StringContent]::new($json, [System.Text.Encoding]::UTF8, 'application/json')
$response = $client.PostAsync("$BaseUrl/api/messages", $body).GetAwaiter().GetResult()
$responseBody = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()

[pscustomobject]@{
  status = [int]$response.StatusCode
  ok     = $response.IsSuccessStatusCode
  body   = $responseBody
} | ConvertTo-Json -Depth 8
