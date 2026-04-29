param(
  [string[]]$Token = @(),
  [string]$TokenFile = "",
  [switch]$IncludeDynamicUserTokens
)

$ErrorActionPreference = "Stop"

function Add-Token {
  param([System.Collections.Generic.List[string]]$List, [string]$Value)
  if ($null -eq $Value) {
    $Value = ""
  }
  $trimmed = $Value.Trim()
  if ($trimmed.Length -ge 3 -and -not $List.Contains($trimmed)) {
    [void]$List.Add($trimmed)
  }
}

$repoRoot = (& git rev-parse --show-toplevel 2>$null).Trim()
if (-not $repoRoot) {
  throw "Not inside a git repository."
}

$tokens = [System.Collections.Generic.List[string]]::new()
foreach ($item in $Token) {
  Add-Token $tokens $item
}

if ($TokenFile) {
  $resolvedTokenFile = Resolve-Path -LiteralPath $TokenFile
  foreach ($line in Get-Content -LiteralPath $resolvedTokenFile) {
    $value = $line.Trim()
    if ($value -and -not $value.StartsWith("#")) {
      Add-Token $tokens $value
    }
  }
} else {
  $gitDir = (& git rev-parse --git-dir).Trim()
  $defaultTokenFile = Join-Path (Join-Path $repoRoot $gitDir) "hooks\forbidden-tokens.txt"
  if (Test-Path -LiteralPath $defaultTokenFile) {
    foreach ($line in Get-Content -LiteralPath $defaultTokenFile) {
      $value = $line.Trim()
      if ($value -and -not $value.StartsWith("#")) {
        Add-Token $tokens $value
      }
    }
  }
}

if ($IncludeDynamicUserTokens) {
  Add-Token $tokens $env:USERNAME
  Add-Token $tokens $env:USER
  Add-Token $tokens ([System.Environment]::UserName)
}

if ($tokens.Count -eq 0) {
  Write-Output "No forbidden tokens configured. Add .git/hooks/forbidden-tokens.txt or pass -Token."
  exit 0
}

$found = $false
foreach ($forbidden in $tokens) {
  $matches = & git grep -in --no-color --fixed-strings -- $forbidden -- `
    ':!.git' `
    ':!web/package-lock.json' `
    ':!web/e2e/bun.lock' `
    ':!go.sum' 2>$null
  if ($LASTEXITCODE -eq 0 -and $matches) {
    if (-not $found) {
      Write-Error "Forbidden public-release tokens found in tracked files:" -ErrorAction Continue
    }
    $found = $true
    foreach ($match in $matches) {
      Write-Error "  $match" -ErrorAction Continue
    }
  } elseif ($LASTEXITCODE -gt 1) {
    throw "git grep failed while checking token '$forbidden'."
  }
}

if ($found) {
  Write-Error "Public release check failed. Remove or redact the token(s) above."
  exit 1
}

Write-Output "No forbidden public-release tokens found."
