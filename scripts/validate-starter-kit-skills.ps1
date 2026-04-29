param(
  [string]$StarterKitPath = "templates/starter-kit"
)

$ErrorActionPreference = "Stop"

function Add-ValidationError {
  param(
    [System.Collections.Generic.List[string]]$Errors,
    [string]$Message
  )
  $Errors.Add($Message) | Out-Null
}

function Test-PortableRelativePath {
  param([string]$Path)
  $value = [string]$Path
  if ([string]::IsNullOrWhiteSpace($value)) {
    return $false
  }
  if ([System.IO.Path]::IsPathRooted($value)) {
    return $false
  }
  if ($value -match '(^|[\\/])\.\.([\\/]|$)') {
    return $false
  }
  if ($value -match '^[A-Za-z]:') {
    return $false
  }
  if ($value -match '[<>:"|?*]') {
    return $false
  }
  return $true
}

function Resolve-SkillFileCandidates {
  param(
    [string]$RepoRoot,
    [string]$StarterKitFullPath,
    [string]$SkillName,
    [string]$SkillPath
  )
  $candidates = New-Object System.Collections.Generic.List[string]
  $candidates.Add((Join-Path $StarterKitFullPath ("agents/skills/{0}/SKILL.md" -f $SkillName))) | Out-Null

  if (-not [string]::IsNullOrWhiteSpace($SkillPath)) {
    $relative = $SkillPath -replace '/', '\'
    $mapped = $relative
    if ($mapped.StartsWith(".agents\")) {
      $mapped = $mapped.Substring(".agents\".Length)
      $candidates.Add((Join-Path $StarterKitFullPath (Join-Path "agents" $mapped))) | Out-Null
    } elseif ($mapped.StartsWith("skills\")) {
      $mapped = $mapped.Substring("skills\".Length)
      $candidates.Add((Join-Path $StarterKitFullPath (Join-Path "agents\skills" $mapped))) | Out-Null
    } elseif ($mapped.StartsWith(".claude\skills\")) {
      $mapped = $mapped.Substring(".claude\skills\".Length)
      $candidates.Add((Join-Path $StarterKitFullPath (Join-Path "agents\skills" $mapped))) | Out-Null
    } elseif ($mapped.StartsWith("templates\starter-kit\")) {
      $candidates.Add((Join-Path $RepoRoot $mapped)) | Out-Null
    } else {
      $candidates.Add((Join-Path $StarterKitFullPath $mapped)) | Out-Null
    }
  }

  return $candidates | Select-Object -Unique
}

function Read-SkillFrontmatter {
  param([string]$Path)
  $lines = Get-Content -LiteralPath $Path -TotalCount 80
  if ($lines.Count -lt 3 -or $lines[0].Trim() -ne "---") {
    return $null
  }
  $frontmatter = @{}
  for ($i = 1; $i -lt $lines.Count; $i++) {
    $line = [string]$lines[$i]
    if ($line.Trim() -eq "---") {
      return $frontmatter
    }
    if ($line -match '^([A-Za-z0-9_-]+):\s*(.*)$') {
      $frontmatter[$matches[1]] = $matches[2].Trim()
    }
  }
  return $null
}

$repoRoot = (Resolve-Path ".").Path
$starterKitFullPath = (Resolve-Path $StarterKitPath).Path
$lockPath = Join-Path $starterKitFullPath "agents/skill-lock.json"
$errors = New-Object System.Collections.Generic.List[string]

if (-not (Test-Path -LiteralPath $lockPath)) {
  throw "Missing starter-kit skill lock: $lockPath"
}

$lock = Get-Content -LiteralPath $lockPath -Raw | ConvertFrom-Json
if (-not $lock.version -or [int]$lock.version -lt 1) {
  Add-ValidationError $errors "skill-lock.json must include a positive version."
}
if ($null -eq $lock.skills) {
  Add-ValidationError $errors "skill-lock.json must include a skills object."
}

$lockedNames = New-Object 'System.Collections.Generic.HashSet[string]'
$skillProperties = @()
if ($null -ne $lock.skills) {
  $skillProperties = @($lock.skills.PSObject.Properties)
}

foreach ($property in $skillProperties) {
  $name = [string]$property.Name
  $entry = $property.Value
  $lockedNames.Add($name) | Out-Null

  if ($name -notmatch '^[a-z0-9][a-z0-9_-]{1,63}$') {
    Add-ValidationError $errors "Invalid skill name '$name'."
  }
  foreach ($field in @("source", "sourceType", "skillPath")) {
    if ([string]::IsNullOrWhiteSpace([string]$entry.$field)) {
      Add-ValidationError $errors "Skill '$name' is missing '$field'."
    }
  }
  if ($entry.sourceType -notin @("github", "curated", "local")) {
    Add-ValidationError $errors "Skill '$name' has unsupported sourceType '$($entry.sourceType)'."
  }
  if (-not (Test-PortableRelativePath ([string]$entry.skillPath))) {
    Add-ValidationError $errors "Skill '$name' has a non-portable skillPath '$($entry.skillPath)'."
  }
  if ($entry.sourceType -eq "github" -and [string]$entry.sourceUrl -notmatch '^https://github\.com/[^/]+/[^/]+') {
    Add-ValidationError $errors "Skill '$name' github sourceUrl must point to github.com."
  }

  $skillFile = $null
  foreach ($candidate in Resolve-SkillFileCandidates $repoRoot $starterKitFullPath $name ([string]$entry.skillPath)) {
    if (Test-Path -LiteralPath $candidate) {
      $skillFile = $candidate
      break
    }
  }
  if ($null -eq $skillFile) {
    Add-ValidationError $errors "Skill '$name' does not resolve to a packaged SKILL.md."
    continue
  }

  $frontmatter = Read-SkillFrontmatter $skillFile
  if ($null -eq $frontmatter) {
    Add-ValidationError $errors "Skill '$name' SKILL.md is missing frontmatter."
    continue
  }
  if ([string]$frontmatter["name"] -ne $name) {
    Add-ValidationError $errors "Skill '$name' frontmatter name is '$($frontmatter["name"])'."
  }
  if ([string]::IsNullOrWhiteSpace([string]$frontmatter["description"])) {
    Add-ValidationError $errors "Skill '$name' frontmatter is missing description."
  }
}

$packagedSkillsRoot = Join-Path $starterKitFullPath "agents/skills"
Get-ChildItem -LiteralPath $packagedSkillsRoot -Directory | ForEach-Object {
  $skillFile = Join-Path $_.FullName "SKILL.md"
  if (-not (Test-Path -LiteralPath $skillFile)) {
    return
  }
  if (-not $lockedNames.Contains($_.Name)) {
    Add-ValidationError $errors "Packaged skill '$($_.Name)' is missing from skill-lock.json."
  }
}

if ($errors.Count -gt 0) {
  Write-Error ("Starter-kit skill validation failed:`n- " + ($errors -join "`n- "))
  exit 1
}

Write-Host ("Validated {0} starter-kit skill manifests." -f $skillProperties.Count)
