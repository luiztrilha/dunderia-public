[CmdletBinding()]
param(
    [string]$TaskName = '',
    [string]$StartScriptPath = ''
)

$ErrorActionPreference = 'Stop'

Import-Module (Join-Path $PSScriptRoot 'UserLogonBootstrap.psm1') -Force

if ([string]::IsNullOrWhiteSpace($TaskName)) {
    $TaskName = Get-DunderIAUserLogonBootstrapTaskName
}
if ([string]::IsNullOrWhiteSpace($StartScriptPath)) {
    $StartScriptPath = Join-Path $PSScriptRoot 'start_user_logon_bootstrap.ps1'
}

$task = Register-DunderIAUserLogonBootstrapTask -TaskName $TaskName -StartScriptPath $StartScriptPath

[pscustomobject]@{
    TaskName = $task.TaskName
    TaskPath = $task.TaskPath
    State    = [string]$task.State
} | ConvertTo-Json -Depth 5
