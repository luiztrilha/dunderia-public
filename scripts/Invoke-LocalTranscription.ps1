param(
    [Parameter(Mandatory = $true)]
    [string]$InputPath,
    [string]$Model = 'tiny',
    [string]$Language = 'pt',
    [ValidateSet('cuda', 'cpu')]
    [string]$Device = 'cuda',
    [string]$ComputeType,
    [string]$OutputPath
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

$scriptPath = Join-Path $PSScriptRoot 'invoke_local_transcription.py'
$pythonExe = Resolve-ExecutablePath -Candidates @(
    $env:WUPHF_LOCAL_PYTHON,
    $env:PYTHON_EXE,
    'python',
    'py'
) -ErrorMessage 'Python runtime nao encontrado. Defina WUPHF_LOCAL_PYTHON/PYTHON_EXE ou adicione python ao PATH.'

if (-not (Test-Path -LiteralPath $scriptPath)) {
    throw "Script auxiliar nao encontrado em $scriptPath"
}

if (-not $ComputeType) {
    $ComputeType = if ($Device -eq 'cuda') { 'float16' } else { 'int8' }
}

$arguments = @(
    $scriptPath,
    '--input', $InputPath,
    '--model', $Model,
    '--language', $Language,
    '--device', $Device,
    '--compute-type', $ComputeType
)

if ($OutputPath) {
    $arguments += @('--output', $OutputPath)
}

& $pythonExe @arguments
