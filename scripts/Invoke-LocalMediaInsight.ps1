param(
    [Parameter(Mandatory = $true)]
    [string]$InputPath,
    [string]$WorkingDirectory = '',
    [string]$WhisperModel = 'tiny',
    [string]$Language = 'pt',
    [ValidateSet('cuda', 'cpu')]
    [string]$Device = 'cuda',
    [string]$SummaryModel = 'qwen2.5-coder:3b',
    [ValidateSet('literal', 'action-items', 'classification')]
    [string]$SummaryMode = 'literal',
    [switch]$SkipSummary
)

$ErrorActionPreference = 'Stop'

function Resolve-ExecutablePath {
    param(
        [string[]]$Candidates,
        [string]$ErrorMessage
    )

    foreach ($candidate in $Candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }

        $command = Get-Command $candidate -ErrorAction SilentlyContinue
        if ($command -and $command.Source) {
            return $command.Source
        }

        if (Test-Path -LiteralPath $candidate) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }

    throw $ErrorMessage
}

$transcriptionScript = Join-Path $PSScriptRoot 'Invoke-LocalTranscription.ps1'
$promptScript = Join-Path $PSScriptRoot 'Get-LocalAiSummaryPrompt.ps1'
$ffmpegExe = Resolve-ExecutablePath -Candidates @(
    $env:WUPHF_FFMPEG_EXE,
    $env:FFMPEG_EXE,
    'ffmpeg'
) -ErrorMessage 'FFmpeg nao encontrado. Defina WUPHF_FFMPEG_EXE/FFMPEG_EXE ou adicione ffmpeg ao PATH.'
Import-Module (Join-Path $PSScriptRoot 'DunderIA.LocalAi.psm1') -Force

if ([string]::IsNullOrWhiteSpace($WorkingDirectory)) {
    $WorkingDirectory = Join-Path $env:TEMP 'dunderia-local-ai'
}
if (-not (Test-Path -LiteralPath $InputPath)) {
    throw "Arquivo de entrada nao encontrado: $InputPath"
}
if (-not (Test-Path -LiteralPath $transcriptionScript)) {
    throw "Script de transcricao nao encontrado: $transcriptionScript"
}

New-Item -ItemType Directory -Force -Path $WorkingDirectory | Out-Null

$source = Resolve-Path -LiteralPath $InputPath
$basename = [System.IO.Path]::GetFileNameWithoutExtension($source.Path)
$timestamp = "{0}-{1}" -f (Get-Date -Format 'yyyyMMdd-HHmmss-fff'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$wavPath = Join-Path $WorkingDirectory "$basename-$timestamp.wav"
$transcriptPath = Join-Path $WorkingDirectory "$basename-$timestamp.transcript.json"
$summaryPath = Join-Path $WorkingDirectory "$basename-$timestamp.summary.json"

$audioExtensions = @('.wav', '.mp3', '.m4a', '.aac', '.flac', '.ogg', '.opus', '.wma')
$sourceExtension = [System.IO.Path]::GetExtension($source.Path).ToLowerInvariant()
if ($audioExtensions -contains $sourceExtension) {
    & $ffmpegExe -loglevel error -y -i $source.Path -ac 1 -ar 16000 $wavPath | Out-Null
} else {
    & $ffmpegExe -loglevel error -y -i $source.Path -vn -ac 1 -ar 16000 $wavPath | Out-Null
}

& $transcriptionScript -InputPath $wavPath -Model $WhisperModel -Language $Language -Device $Device -OutputPath $transcriptPath | Out-Null
$transcript = Get-Content -Raw -LiteralPath $transcriptPath | ConvertFrom-Json

$summary = $null
if (-not $SkipSummary) {
    $prompt = & $promptScript -Mode $SummaryMode -SourceLabel 'a transcricao de uma midia' -Text $transcript.text | Out-String
    $prompt = $prompt.Trim()
    $summaryResult = Invoke-DunderIALocalAiGenerate -ProfileId 'default' -Model $SummaryModel -Prompt $prompt | ConvertTo-Json -Depth 4
    $summaryResult | Set-Content -LiteralPath $summaryPath -Encoding UTF8
    $summary = Get-Content -Raw -LiteralPath $summaryPath | ConvertFrom-Json
}

$summaryParsed = $null
if ($summary -and $summary.response) {
    $summaryText = [string]$summary.response
    $summaryText = $summaryText.Trim()
    if ($summaryText.StartsWith('```json')) { $summaryText = $summaryText.Substring(7).Trim() }
    elseif ($summaryText.StartsWith('```')) { $summaryText = $summaryText.Substring(3).Trim() }
    if ($summaryText.EndsWith('```')) { $summaryText = $summaryText.Substring(0, $summaryText.Length - 3).Trim() }
    try { $summaryParsed = $summaryText | ConvertFrom-Json } catch { $summaryParsed = $null }
}

[ordered]@{
    input = $source.Path
    extractedAudio = $wavPath
    transcriptFile = $transcriptPath
    transcriptText = $transcript.text
    summaryFile = $summaryPath
    summaryModel = $SummaryModel
    summaryMode = $SummaryMode
    summaryResponse = if ($summary) { $summary.response } else { $null }
    summaryJson = $summaryParsed
} | ConvertTo-Json -Depth 6
