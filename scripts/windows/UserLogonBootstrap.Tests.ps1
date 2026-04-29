$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$modulePath = Join-Path $here 'UserLogonBootstrap.psm1'

Import-Module $modulePath -Force

Describe 'UserLogonBootstrap' {
    It 'prefers the newest installed Google DriveFS version' {
        $root = Join-Path $TestDrive 'Drive File Stream'
        New-Item -ItemType Directory -Path (Join-Path $root '121.0.0.0') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $root '123.0.1.0') -Force | Out-Null
        New-Item -ItemType File -Path (Join-Path $root '121.0.0.0\GoogleDriveFS.exe') -Force | Out-Null
        New-Item -ItemType File -Path (Join-Path $root '123.0.1.0\GoogleDriveFS.exe') -Force | Out-Null

        $resolved = Resolve-DunderIAGoogleDriveFSPath -InstallRoot $root

        $resolved | Should Match ([regex]::Escape('123.0.1.0\GoogleDriveFS.exe'))
    }

    It 'skips running targets and does not treat wuphf mcp-team as the main office process' {
        $repoRoot = Join-Path $TestDrive 'dunderia'
        New-Item -ItemType Directory -Path $repoRoot -Force | Out-Null

        $wuphfPath = Join-Path $repoRoot 'wuphf.exe'
        $ollamaPath = Join-Path $TestDrive 'ollama.exe'
        $googleDrivePath = Join-Path $TestDrive 'GoogleDriveFS.exe'
        $codexLBPath = Join-Path $TestDrive 'codex-lb.exe'

        New-Item -ItemType File -Path $wuphfPath -Force | Out-Null
        New-Item -ItemType File -Path $ollamaPath -Force | Out-Null
        New-Item -ItemType File -Path $googleDrivePath -Force | Out-Null
        New-Item -ItemType File -Path $codexLBPath -Force | Out-Null

        $running = @(
            [pscustomobject]@{
                Name = 'codex-lb.exe'
                ProcessId = 101
                ExecutablePath = $codexLBPath
                CommandLine = '"' + $codexLBPath + '"'
            },
            [pscustomobject]@{
                Name = 'wuphf.exe'
                ProcessId = 202
                ExecutablePath = $wuphfPath
                CommandLine = '"' + $wuphfPath + '" mcp-team'
            }
        )

        $plan = Get-DunderIAUserLogonBootstrapPlan `
            -RepoRoot $repoRoot `
            -RunningProcesses $running `
            -CodexLBPath $codexLBPath `
            -OllamaPath $ollamaPath `
            -GoogleDrivePath $googleDrivePath `
            -WuphfPath $wuphfPath

        ($plan.Items | Where-Object { $_.Id -eq 'codex-lb' }).Status | Should Be 'skipped'
        ($plan.Items | Where-Object { $_.Id -eq 'codex-lb' }).SkipReason | Should Be 'already_running'
        ($plan.Items | Where-Object { $_.Id -eq 'ollama' }).Status | Should Be 'launch'
        ($plan.Items | Where-Object { $_.Id -eq 'google-drive' }).Status | Should Be 'launch'
        ($plan.Items | Where-Object { $_.Id -eq 'dunderia' }).Status | Should Be 'launch'
        (($plan.Items | Where-Object { $_.Id -eq 'dunderia' }).Arguments -join ' ') | Should Match '--no-open'
    }

    It 'builds a hidden scheduled-task action for the bootstrap runner' {
        $startScript = 'D:\Repos\dunderia\scripts\windows\start_user_logon_bootstrap.ps1'

        $definition = Get-DunderIAUserLogonBootstrapTaskDefinition -StartScriptPath $startScript

        $definition.TaskName | Should Be 'DunderIA User Logon Bootstrap'
        $definition.Execute | Should Be 'powershell.exe'
        $definition.Argument | Should Match '-WindowStyle Hidden'
        $definition.Argument | Should Match ([regex]::Escape($startScript))
    }

    It 'does not launch items marked as skipped' {
        $plan = [pscustomobject]@{
            TaskName = 'DunderIA User Logon Bootstrap'
            RepoRoot = $TestDrive
            LogRoot  = (Join-Path $TestDrive 'logs')
            Items    = @(
                [pscustomobject]@{
                    Id               = 'codex-lb'
                    Name             = 'codex-lb'
                    Status           = 'skipped'
                    SkipReason       = 'already_running'
                    Path             = 'C:\fake\codex-lb.exe'
                    Arguments        = @()
                    WorkingDirectory = 'C:\fake'
                    StdOutLog        = ''
                    StdErrLog        = ''
                }
            )
        }

        $result = Invoke-DunderIAUserLogonBootstrap -Plan $plan

        $result.Results.Count | Should Be 1
        ($result.Results | Select-Object -First 1).Status | Should Be 'skipped'
    }
}
