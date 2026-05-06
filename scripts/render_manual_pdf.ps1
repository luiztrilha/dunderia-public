param(
  [string]$InputPath,
  [string]$OutputPath
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)

if ([string]::IsNullOrWhiteSpace($InputPath)) {
  $InputPath = Join-Path $repoRoot 'docs\MANUAL.md'
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
  $OutputPath = Join-Path $repoRoot 'docs\MANUAL.pdf'
}

$inputFullPath = (Resolve-Path -LiteralPath $InputPath).Path
$outputFullPath = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputPath)
$renderDir = Join-Path $repoRoot '.tmp-manual-pdf-render'
$htmlPath = Join-Path $renderDir 'MANUAL.html'

New-Item -ItemType Directory -Force -Path $renderDir | Out-Null

$markdown = ConvertFrom-Markdown -Path $inputFullPath
$title = [System.Net.WebUtility]::HtmlEncode((Split-Path -Leaf $inputFullPath))

$html = @"
<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8" />
  <title>$title</title>
  <style>
    @page { margin: 18mm 16mm; }
    body {
      color: #181818;
      font-family: "Segoe UI", Arial, sans-serif;
      font-size: 12px;
      line-height: 1.55;
      margin: 0 auto;
      max-width: 860px;
    }
    h1, h2, h3, h4 { line-height: 1.25; margin: 1.35em 0 0.45em; }
    h1 { font-size: 26px; border-bottom: 1px solid #d8d8d8; padding-bottom: 8px; }
    h2 { font-size: 20px; border-bottom: 1px solid #eeeeee; padding-bottom: 5px; break-after: avoid; }
    h3 { font-size: 15px; break-after: avoid; }
    p, li { break-inside: avoid; }
    pre {
      background: #f6f8fa;
      border: 1px solid #d8dee4;
      border-radius: 6px;
      overflow-wrap: break-word;
      padding: 10px;
      white-space: pre-wrap;
    }
    code {
      background: #f6f8fa;
      border-radius: 4px;
      font-family: "Cascadia Mono", Consolas, monospace;
      font-size: 0.92em;
      padding: 1px 4px;
    }
    pre code { background: transparent; padding: 0; }
    table {
      border-collapse: collapse;
      margin: 12px 0;
      width: 100%;
    }
    th, td {
      border: 1px solid #d8dee4;
      padding: 6px 8px;
      text-align: left;
      vertical-align: top;
    }
    th { background: #f6f8fa; }
    blockquote {
      border-left: 4px solid #d0d7de;
      color: #57606a;
      margin-left: 0;
      padding-left: 12px;
    }
  </style>
</head>
<body>
$($markdown.Html)
</body>
</html>
"@

Set-Content -LiteralPath $htmlPath -Value $html -Encoding UTF8

$chromeCandidates = @(
  (Join-Path $env:ProgramFiles 'Google\Chrome\Application\chrome.exe'),
  (Join-Path ([Environment]::GetFolderPath('ProgramFilesX86')) 'Google\Chrome\Application\chrome.exe'),
  (Join-Path $env:ProgramFiles 'Microsoft\Edge\Application\msedge.exe'),
  (Join-Path ([Environment]::GetFolderPath('ProgramFilesX86')) 'Microsoft\Edge\Application\msedge.exe')
)

$browser = $chromeCandidates | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } | Select-Object -First 1
if (-not $browser) {
  throw 'Nao encontrei Chrome ou Edge para imprimir o manual em PDF.'
}

$htmlUri = [System.Uri]::new($htmlPath).AbsoluteUri
$args = @(
  '--headless=new',
  '--disable-gpu',
  '--no-pdf-header-footer',
  "--print-to-pdf=$outputFullPath",
  $htmlUri
)

& $browser @args | Out-Null

if (-not (Test-Path -LiteralPath $outputFullPath -PathType Leaf)) {
  throw "PDF nao foi gerado em $outputFullPath"
}

Write-Host "Manual PDF gerado: $outputFullPath"
