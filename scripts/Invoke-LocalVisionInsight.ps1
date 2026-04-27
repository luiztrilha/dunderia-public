param(
    [Parameter(Mandatory = $true)]
    [string]$InputPath,
    [string]$WorkingDirectory = '',
    [int]$Dpi = 200,
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

$ocrScript = Join-Path $PSScriptRoot 'invoke_local_ocr.py'
$promptScript = Join-Path $PSScriptRoot 'Get-LocalAiSummaryPrompt.ps1'
$pythonExe = Resolve-ExecutablePath -Candidates @(
    $env:WUPHF_LOCAL_PYTHON,
    $env:PYTHON_EXE,
    'python',
    'py'
) -ErrorMessage 'Python runtime nao encontrado. Defina WUPHF_LOCAL_PYTHON/PYTHON_EXE ou adicione python ao PATH.'
Import-Module (Join-Path $PSScriptRoot 'DunderIA.LocalAi.psm1') -Force

if ([string]::IsNullOrWhiteSpace($WorkingDirectory)) {
    $WorkingDirectory = Join-Path $env:TEMP 'dunderia-local-ai'
}
if (-not (Test-Path -LiteralPath $InputPath)) {
    throw "Arquivo de entrada nao encontrado: $InputPath"
}
if (-not (Test-Path -LiteralPath $ocrScript)) {
    throw "Script OCR nao encontrado: $ocrScript"
}

New-Item -ItemType Directory -Force -Path $WorkingDirectory | Out-Null

$source = Resolve-Path -LiteralPath $InputPath
$basename = [System.IO.Path]::GetFileNameWithoutExtension($source.Path)
$timestamp = "{0}-{1}" -f (Get-Date -Format 'yyyyMMdd-HHmmss-fff'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$ocrPath = Join-Path $WorkingDirectory "$basename-$timestamp.ocr.json"
$imagesDir = Join-Path $WorkingDirectory "$basename-$timestamp.pages"
$summaryPath = Join-Path $WorkingDirectory "$basename-$timestamp.summary.json"

& $pythonExe $ocrScript --input $source.Path --output $ocrPath --images-dir $imagesDir --dpi $Dpi
$ocr = Get-Content -Raw -LiteralPath $ocrPath | ConvertFrom-Json

$summary = $null
if (-not $SkipSummary) {
    $prompt = & $promptScript -Mode $SummaryMode -SourceLabel 'um texto extraido por OCR de imagem ou PDF' -Text $ocr.text | Out-String
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
    ocrFile = $ocrPath
    renderedImages = $ocr.images
    ocrText = $ocr.text
    summaryFile = $summaryPath
    summaryModel = $SummaryModel
    summaryMode = $SummaryMode
    summaryResponse = if ($summary) { $summary.response } else { $null }
    summaryJson = $summaryParsed
} | ConvertTo-Json -Depth 6
