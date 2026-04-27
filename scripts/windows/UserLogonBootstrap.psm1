Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-DunderIAUserLogonBootstrapTaskName {
    return 'DunderIA User Logon Bootstrap'
}

function Resolve-DunderIAUserLogonBootstrapRepoRoot {
    param(
        [string]$ScriptRoot = $PSScriptRoot
    )

    return Split-Path -Parent (Split-Path -Parent $ScriptRoot)
}

function Resolve-DunderIAUserLogonBootstrapRuntimeRoot {
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

function Get-DunderIAUserLogonBootstrapLogRoot {
    param(
        [string]$RepoRoot
    )

    if (-not [string]::IsNullOrWhiteSpace($env:WUPHF_LOGON_BOOTSTRAP_LOG_ROOT)) {
        return $env:WUPHF_LOGON_BOOTSTRAP_LOG_ROOT
    }

    return Join-Path (Resolve-DunderIAUserLogonBootstrapRuntimeRoot) 'user-logon-bootstrap'
}

function Ensure-DunderIAUserLogonBootstrapDirectory {
    param(
        [string]$Path
    )

    if ([string]::IsNullOrWhiteSpace($Path)) {
        return
    }

    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Path $Path -Force | Out-Null
    }
}

function Write-DunderIAUserLogonBootstrapLog {
    param(
        [string]$LogRoot,
        [hashtable]$Record
    )

    Ensure-DunderIAUserLogonBootstrapDirectory -Path $LogRoot

    $payload = [ordered]@{
        timestamp = (Get-Date).ToString('o')
    }
    foreach ($key in $Record.Keys) {
        $payload[$key] = $Record[$key]
    }

    $line = $payload | ConvertTo-Json -Depth 8 -Compress
    Add-Content -LiteralPath (Join-Path $LogRoot 'bootstrap.jsonl') -Value $line -Encoding UTF8
}

function Resolve-DunderIABinaryPath {
    param(
        [string[]]$Candidates
    )

    foreach ($candidate in @($Candidates)) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }

        if (Test-Path -LiteralPath $candidate) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }

        $command = Get-Command $candidate -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $command -and -not [string]::IsNullOrWhiteSpace($command.Source)) {
            return $command.Source
        }
    }

    return ''
}

function Resolve-DunderIACodexLBPath {
    return Resolve-DunderIABinaryPath -Candidates @(
        $env:WUPHF_LOGON_BOOTSTRAP_CODEX_LB_EXE,
        'codex-lb',
        'codex-lb.exe'
    )
}

function Resolve-DunderIAOllamaPath {
    $defaultPath = ''
    if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        $defaultPath = Join-Path $env:LOCALAPPDATA 'Programs\Ollama\ollama.exe'
    }

    return Resolve-DunderIABinaryPath -Candidates @(
        $env:WUPHF_LOGON_BOOTSTRAP_OLLAMA_EXE,
        $defaultPath,
        'ollama',
        'ollama.exe'
    )
}

function Resolve-DunderIAGoogleDriveFSPath {
    param(
        [string]$InstallRoot = 'C:\Program Files\Google\Drive File Stream'
    )

    if (-not [string]::IsNullOrWhiteSpace($env:WUPHF_LOGON_BOOTSTRAP_GOOGLE_DRIVE_EXE)) {
        return Resolve-DunderIABinaryPath -Candidates @($env:WUPHF_LOGON_BOOTSTRAP_GOOGLE_DRIVE_EXE)
    }

    if (-not (Test-Path -LiteralPath $InstallRoot)) {
        return ''
    }

    $candidates = @()
    foreach ($dir in Get-ChildItem -LiteralPath $InstallRoot -Directory -ErrorAction SilentlyContinue) {
        try {
            $version = [version]$dir.Name
        } catch {
            continue
        }

        $exePath = Join-Path $dir.FullName 'GoogleDriveFS.exe'
        if (-not (Test-Path -LiteralPath $exePath)) {
            continue
        }

        $candidates += [pscustomobject]@{
            Version = $version
            Path    = $exePath
        }
    }

    if ($candidates.Count -eq 0) {
        return ''
    }

    return ($candidates |
        Sort-Object -Property @{ Expression = 'Version'; Descending = $true }, @{ Expression = 'Path'; Descending = $true } |
        Select-Object -First 1).Path
}

function Resolve-DunderIAWuphfPath {
    param(
        [string]$RepoRoot
    )

    return Resolve-DunderIABinaryPath -Candidates @(
        $env:WUPHF_LOGON_BOOTSTRAP_WUPHF_EXE,
        (Join-Path $RepoRoot 'wuphf.exe')
    )
}

function Get-DunderIAUserLogonBootstrapProcessSnapshot {
    $out = @()
    foreach ($process in Get-CimInstance Win32_Process) {
        $out += [pscustomobject]@{
            Name           = [string]$process.Name
            ProcessId      = [int]$process.ProcessId
            ExecutablePath = [string]$process.ExecutablePath
            CommandLine    = [string]$process.CommandLine
        }
    }

    return $out
}

function Get-DunderIACommandLineRemainder {
    param(
        [string]$CommandLine
    )

    $trimmed = [string]::Empty
    if (-not [string]::IsNullOrWhiteSpace($CommandLine)) {
        $trimmed = $CommandLine.Trim()
    }

    if ($trimmed -eq '') {
        return ''
    }

    if ($trimmed.StartsWith('"')) {
        $endQuote = $trimmed.IndexOf('"', 1)
        if ($endQuote -ge 0) {
            return $trimmed.Substring($endQuote + 1).Trim()
        }
    }

    $parts = $trimmed.Split(@(' '), 2, [System.StringSplitOptions]::None)
    if ($parts.Count -ge 2) {
        return $parts[1].Trim()
    }

    return ''
}

function Test-DunderIAOfficeProcess {
    param(
        [string]$CommandLine
    )

    $remainder = Get-DunderIACommandLineRemainder -CommandLine $CommandLine
    if ($remainder -eq '') {
        return $true
    }

    if ($remainder -match '^(mcp-team|init|shred|import|log|repair-channel-memory)\b') {
        return $false
    }

    if ($remainder -match '^--cmd(\s|$)') {
        return $false
    }

    if ($remainder.StartsWith('--')) {
        return $true
    }

    return $false
}

function Test-DunderIAProcessRunning {
    param(
        [object[]]$RunningProcesses,
        [string]$TargetId
    )

    foreach ($process in @($RunningProcesses)) {
        switch ($TargetId) {
            'codex-lb' {
                if ($process.Name -ieq 'codex-lb.exe' -or $process.Name -ieq 'codex-lb') {
                    return $true
                }
            }
            'ollama' {
                if ($process.Name -ieq 'ollama.exe' -or $process.Name -ieq 'ollama') {
                    return $true
                }
            }
            'google-drive' {
                if ($process.Name -ieq 'GoogleDriveFS.exe' -or $process.Name -ieq 'GoogleDriveFS') {
                    return $true
                }
            }
            'dunderia' {
                if (($process.Name -ieq 'wuphf.exe' -or $process.Name -ieq 'wuphf') -and (Test-DunderIAOfficeProcess -CommandLine $process.CommandLine)) {
                    return $true
                }
            }
        }
    }

    return $false
}

function New-DunderIAUserLogonBootstrapItem {
    param(
        [string]$Id,
        [string]$Name,
        [string]$Path,
        [string[]]$Arguments,
        [string]$WorkingDirectory,
        [string]$LogRoot,
        [bool]$IsRunning,
        [switch]$RedirectIO
    )

    $status = 'launch'
    $skipReason = ''
    if ($IsRunning) {
        $status = 'skipped'
        $skipReason = 'already_running'
    } elseif ([string]::IsNullOrWhiteSpace($Path)) {
        $status = 'unresolved'
        $skipReason = 'binary_not_found'
    }

    $stdoutLog = ''
    $stderrLog = ''
    if ($RedirectIO.IsPresent -and -not [string]::IsNullOrWhiteSpace($LogRoot)) {
        $stdoutLog = Join-Path $LogRoot ($Id + '.stdout.log')
        $stderrLog = Join-Path $LogRoot ($Id + '.stderr.log')
    }

    return [pscustomobject]@{
        Id               = $Id
        Name             = $Name
        Status           = $status
        SkipReason       = $skipReason
        Path             = $Path
        Arguments        = @($Arguments)
        WorkingDirectory = $WorkingDirectory
        StdOutLog        = $stdoutLog
        StdErrLog        = $stderrLog
    }
}

function Get-DunderIAUserLogonBootstrapPlan {
    param(
        [string]$RepoRoot = '',
        [string]$LogRoot = '',
        [object[]]$RunningProcesses = $null,
        [string]$CodexLBPath = '',
        [string]$OllamaPath = '',
        [string]$GoogleDrivePath = '',
        [string]$WuphfPath = ''
    )

    if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
        $RepoRoot = Resolve-DunderIAUserLogonBootstrapRepoRoot
    }
    if ([string]::IsNullOrWhiteSpace($LogRoot)) {
        $LogRoot = Get-DunderIAUserLogonBootstrapLogRoot -RepoRoot $RepoRoot
    }
    if ($null -eq $RunningProcesses) {
        $RunningProcesses = Get-DunderIAUserLogonBootstrapProcessSnapshot
    }
    if ([string]::IsNullOrWhiteSpace($CodexLBPath)) {
        $CodexLBPath = Resolve-DunderIACodexLBPath
    }
    if ([string]::IsNullOrWhiteSpace($OllamaPath)) {
        $OllamaPath = Resolve-DunderIAOllamaPath
    }
    if ([string]::IsNullOrWhiteSpace($GoogleDrivePath)) {
        $GoogleDrivePath = Resolve-DunderIAGoogleDriveFSPath
    }
    if ([string]::IsNullOrWhiteSpace($WuphfPath)) {
        $WuphfPath = Resolve-DunderIAWuphfPath -RepoRoot $RepoRoot
    }

    $items = @(
        (New-DunderIAUserLogonBootstrapItem `
            -Id 'codex-lb' `
            -Name 'codex-lb' `
            -Path $CodexLBPath `
            -Arguments @() `
            -WorkingDirectory $(if ($CodexLBPath) { Split-Path -Parent $CodexLBPath } else { '' }) `
            -LogRoot $LogRoot `
            -IsRunning (Test-DunderIAProcessRunning -RunningProcesses $RunningProcesses -TargetId 'codex-lb') `
            -RedirectIO),
        (New-DunderIAUserLogonBootstrapItem `
            -Id 'ollama' `
            -Name 'Ollama' `
            -Path $OllamaPath `
            -Arguments @('serve') `
            -WorkingDirectory $(if ($OllamaPath) { Split-Path -Parent $OllamaPath } else { '' }) `
            -LogRoot $LogRoot `
            -IsRunning (Test-DunderIAProcessRunning -RunningProcesses $RunningProcesses -TargetId 'ollama') `
            -RedirectIO),
        (New-DunderIAUserLogonBootstrapItem `
            -Id 'google-drive' `
            -Name 'Google DriveFS' `
            -Path $GoogleDrivePath `
            -Arguments @() `
            -WorkingDirectory $(if ($GoogleDrivePath) { Split-Path -Parent $GoogleDrivePath } else { '' }) `
            -LogRoot $LogRoot `
            -IsRunning (Test-DunderIAProcessRunning -RunningProcesses $RunningProcesses -TargetId 'google-drive')),
        (New-DunderIAUserLogonBootstrapItem `
            -Id 'dunderia' `
            -Name 'DunderIA' `
            -Path $WuphfPath `
            -Arguments @('--no-open') `
            -WorkingDirectory $RepoRoot `
            -LogRoot $LogRoot `
            -IsRunning (Test-DunderIAProcessRunning -RunningProcesses $RunningProcesses -TargetId 'dunderia') `
            -RedirectIO)
    )

    return [pscustomobject]@{
        TaskName = Get-DunderIAUserLogonBootstrapTaskName
        RepoRoot = $RepoRoot
        LogRoot  = $LogRoot
        Items    = $items
    }
}

function Invoke-DunderIAUserLogonBootstrap {
    param(
        [object]$Plan
    )

    if ($null -eq $Plan) {
        $Plan = Get-DunderIAUserLogonBootstrapPlan
    }

    Ensure-DunderIAUserLogonBootstrapDirectory -Path $Plan.LogRoot

    $results = @()
    foreach ($item in @($Plan.Items)) {
        if ($item.Status -eq 'skipped') {
            Write-DunderIAUserLogonBootstrapLog -LogRoot $Plan.LogRoot -Record @{
                target = $item.Id
                event  = 'skipped'
                reason = $item.SkipReason
            }
            $results += [pscustomobject]@{
                Id        = $item.Id
                Name      = $item.Name
                Status    = 'skipped'
                ProcessId = $null
                Error     = ''
            }
            continue
        }

        if ($item.Status -eq 'unresolved') {
            Write-DunderIAUserLogonBootstrapLog -LogRoot $Plan.LogRoot -Record @{
                target = $item.Id
                event  = 'unresolved'
                reason = $item.SkipReason
            }
            $results += [pscustomobject]@{
                Id        = $item.Id
                Name      = $item.Name
                Status    = 'unresolved'
                ProcessId = $null
                Error     = ''
            }
            continue
        }

        try {
            $startProcessArgs = @{
                FilePath     = $item.Path
                WindowStyle  = 'Hidden'
                WorkingDirectory = $item.WorkingDirectory
                PassThru     = $true
            }
            if ($item.Arguments.Count -gt 0) {
                $startProcessArgs.ArgumentList = $item.Arguments
            }
            if (-not [string]::IsNullOrWhiteSpace($item.StdOutLog)) {
                $startProcessArgs.RedirectStandardOutput = $item.StdOutLog
            }
            if (-not [string]::IsNullOrWhiteSpace($item.StdErrLog)) {
                $startProcessArgs.RedirectStandardError = $item.StdErrLog
            }

            $process = Start-Process @startProcessArgs
            Write-DunderIAUserLogonBootstrapLog -LogRoot $Plan.LogRoot -Record @{
                target      = $item.Id
                event       = 'started'
                process_id  = $process.Id
                executable  = $item.Path
                arguments   = @($item.Arguments)
            }
            $results += [pscustomobject]@{
                Id        = $item.Id
                Name      = $item.Name
                Status    = 'started'
                ProcessId = $process.Id
                Error     = ''
            }
        } catch {
            Write-DunderIAUserLogonBootstrapLog -LogRoot $Plan.LogRoot -Record @{
                target = $item.Id
                event  = 'failed'
                error  = $_.Exception.Message
            }
            $results += [pscustomobject]@{
                Id        = $item.Id
                Name      = $item.Name
                Status    = 'failed'
                ProcessId = $null
                Error     = $_.Exception.Message
            }
        }
    }

    return [pscustomobject]@{
        TaskName = $Plan.TaskName
        RepoRoot = $Plan.RepoRoot
        LogRoot  = $Plan.LogRoot
        Results  = $results
    }
}

function Get-DunderIAUserLogonBootstrapTaskDefinition {
    param(
        [string]$StartScriptPath = (Join-Path $PSScriptRoot 'start_user_logon_bootstrap.ps1'),
        [string]$TaskName = (Get-DunderIAUserLogonBootstrapTaskName)
    )

    $quotedStartScript = '"' + $StartScriptPath + '"'

    return [pscustomobject]@{
        TaskName    = $TaskName
        Execute     = 'powershell.exe'
        Argument    = '-NoLogo -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File ' + $quotedStartScript
        Description = 'Launch codex-lb, Ollama, Google DriveFS, and repo wuphf.exe at current-user logon.'
    }
}

function Register-DunderIAUserLogonBootstrapTask {
    param(
        [string]$StartScriptPath = (Join-Path $PSScriptRoot 'start_user_logon_bootstrap.ps1'),
        [string]$TaskName = (Get-DunderIAUserLogonBootstrapTaskName),
        [string]$UserId = ''
    )

    if ([string]::IsNullOrWhiteSpace($UserId)) {
        $UserId = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
    }

    $definition = Get-DunderIAUserLogonBootstrapTaskDefinition -StartScriptPath $StartScriptPath -TaskName $TaskName
    $action = New-ScheduledTaskAction -Execute $definition.Execute -Argument $definition.Argument
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $UserId
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew
    $principal = New-ScheduledTaskPrincipal -UserId $UserId -LogonType Interactive -RunLevel Limited

    Register-ScheduledTask `
        -TaskName $definition.TaskName `
        -Action $action `
        -Trigger $trigger `
        -Settings $settings `
        -Principal $principal `
        -Description $definition.Description `
        -Force | Out-Null

    return Get-ScheduledTask -TaskName $definition.TaskName
}

function Unregister-DunderIAUserLogonBootstrapTask {
    param(
        [string]$TaskName = (Get-DunderIAUserLogonBootstrapTaskName)
    )

    if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
    }
}

Export-ModuleMember -Function `
    Get-DunderIAUserLogonBootstrapTaskName, `
    Resolve-DunderIAUserLogonBootstrapRepoRoot, `
    Resolve-DunderIAGoogleDriveFSPath, `
    Test-DunderIAOfficeProcess, `
    Get-DunderIAUserLogonBootstrapPlan, `
    Invoke-DunderIAUserLogonBootstrap, `
    Get-DunderIAUserLogonBootstrapTaskDefinition, `
    Register-DunderIAUserLogonBootstrapTask, `
    Unregister-DunderIAUserLogonBootstrapTask
