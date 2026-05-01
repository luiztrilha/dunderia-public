param(
  [string]$PrototypeDir,
  [switch]$NoOpen
)

$ErrorActionPreference = 'Stop'

function Resolve-OpenCoDesignExecutable {
  if ($env:WUPHF_OPEN_CODESIGN_EXE) {
    if (Test-Path -LiteralPath $env:WUPHF_OPEN_CODESIGN_EXE -PathType Leaf) {
      return (Resolve-Path -LiteralPath $env:WUPHF_OPEN_CODESIGN_EXE).Path
    }
    Write-Warning "WUPHF_OPEN_CODESIGN_EXE aponta para um arquivo inexistente: $env:WUPHF_OPEN_CODESIGN_EXE"
  }

  $commands = @('open-codesign', 'open-codesign.exe')
  foreach ($command in $commands) {
    $resolved = Get-Command $command -ErrorAction SilentlyContinue
    if ($resolved -and $resolved.Source) {
      return $resolved.Source
    }
  }

  $programFilesX86 = [Environment]::GetFolderPath('ProgramFilesX86')
  $candidates = @(
    (Join-Path $env:LOCALAPPDATA 'Programs\open-codesign\Open CoDesign.exe'),
    (Join-Path $env:LOCALAPPDATA 'Programs\Open CoDesign\Open CoDesign.exe'),
    (Join-Path $env:ProgramFiles 'Open CoDesign\Open CoDesign.exe'),
    (Join-Path $programFilesX86 'Open CoDesign\Open CoDesign.exe'),
    (Join-Path $HOME 'scoop\shims\open-codesign.exe'),
    (Join-Path $HOME 'scoop\apps\open-codesign\current\Open CoDesign.exe')
  )

  foreach ($candidate in $candidates) {
    if (Test-Path -LiteralPath $candidate -PathType Leaf) {
      return (Resolve-Path -LiteralPath $candidate).Path
    }
  }

  return $null
}

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)

if ([string]::IsNullOrWhiteSpace($PrototypeDir)) {
  $PrototypeDir = Join-Path $repoRoot 'temp\open-codesign'
}

New-Item -ItemType Directory -Force -Path $PrototypeDir | Out-Null
$resolvedPrototypeDir = (Resolve-Path -LiteralPath $PrototypeDir).Path

$exe = Resolve-OpenCoDesignExecutable
if (-not $exe) {
  Write-Host 'Open CoDesign nao foi encontrado nesta maquina.'
  Write-Host ''
  Write-Host 'Instale manualmente uma das opcoes abaixo e rode este script novamente:'
  Write-Host '  winget install OpenCoworkAI.OpenCoDesign'
  Write-Host '  scoop bucket add opencoworkai https://github.com/OpenCoworkAI/scoop-bucket'
  Write-Host '  scoop install open-codesign'
  Write-Host ''
  Write-Host 'Download direto: https://github.com/OpenCoworkAI/open-codesign/releases'
  Write-Host ''
  Write-Host "Pasta de handoff preparada: $resolvedPrototypeDir"
  Write-Host 'Use uma chave descartavel/de baixo limite ou Ollama para os primeiros testes.'
  exit 2
}

Write-Host "Open CoDesign: $exe"
Write-Host "Pasta de handoff: $resolvedPrototypeDir"
Write-Host 'Recomendacao: exporte HTML, Markdown, PDF ou PPTX para essa pasta antes de portar algo para a MaestrIA.'

if (-not $NoOpen) {
  Start-Process -FilePath $exe -WorkingDirectory $resolvedPrototypeDir -WindowStyle Normal
}
