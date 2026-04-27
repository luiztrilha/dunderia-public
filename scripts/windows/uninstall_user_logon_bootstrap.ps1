[CmdletBinding()]
param(
    [string]$TaskName = ''
)

$ErrorActionPreference = 'Stop'

Import-Module (Join-Path $PSScriptRoot 'UserLogonBootstrap.psm1') -Force

if ([string]::IsNullOrWhiteSpace($TaskName)) {
    $TaskName = Get-DunderIAUserLogonBootstrapTaskName
}

Unregister-DunderIAUserLogonBootstrapTask -TaskName $TaskName

[pscustomobject]@{
    TaskName = $TaskName
    Removed  = $true
} | ConvertTo-Json -Depth 3
