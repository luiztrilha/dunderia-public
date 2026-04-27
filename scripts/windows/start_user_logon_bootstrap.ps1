[CmdletBinding()]
param(
    [string]$RepoRoot = '',
    [string]$LogRoot = '',
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'

Import-Module (Join-Path $PSScriptRoot 'UserLogonBootstrap.psm1') -Force

if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
    $RepoRoot = Resolve-DunderIAUserLogonBootstrapRepoRoot -ScriptRoot $PSScriptRoot
}

$plan = Get-DunderIAUserLogonBootstrapPlan -RepoRoot $RepoRoot -LogRoot $LogRoot
if ($DryRun) {
    $plan | ConvertTo-Json -Depth 8
    return
}

$result = Invoke-DunderIAUserLogonBootstrap -Plan $plan
$result | ConvertTo-Json -Depth 8
