param(
  [ValidateSet("list", "stop")]
  [string]$Action = "list",
  [int[]]$Ports = @(7890, 7891, 30000),
  [switch]$Force
)

$ErrorActionPreference = "Stop"

function Get-DunderIAProcess {
  param([int[]]$Ports)

  $byPid = @{}
  foreach ($port in $Ports) {
    try {
      Get-NetTCPConnection -LocalPort $port -ErrorAction Stop | ForEach-Object {
        if ($_.OwningProcess -and -not $byPid.ContainsKey([int]$_.OwningProcess)) {
          $byPid[[int]$_.OwningProcess] = [ordered]@{
            Id = [int]$_.OwningProcess
            Ports = New-Object System.Collections.Generic.List[int]
          }
        }
        if ($_.OwningProcess) {
          $byPid[[int]$_.OwningProcess].Ports.Add([int]$port) | Out-Null
        }
      }
    } catch {
      # Some Windows editions or shells may not expose Get-NetTCPConnection.
    }
  }

  $repo = (Resolve-Path ".").Path
  Get-CimInstance Win32_Process | ForEach-Object {
    $commandLine = [string]$_.CommandLine
    $matchesRepo = $commandLine -like "*$repo*"
    $matchesRuntime = $commandLine -match '(?i)\b(wuphf|dunderia|vite|npm|node|go\.exe|go)\b'
    if ($matchesRepo -and $matchesRuntime -and -not $byPid.ContainsKey([int]$_.ProcessId)) {
      $byPid[[int]$_.ProcessId] = [ordered]@{
        Id = [int]$_.ProcessId
        Ports = New-Object System.Collections.Generic.List[int]
      }
    }
  }

  foreach ($item in $byPid.Values) {
    $process = Get-CimInstance Win32_Process -Filter ("ProcessId={0}" -f $item.Id) -ErrorAction SilentlyContinue
    if ($null -eq $process) {
      continue
    }
    [pscustomobject]@{
      Id = $item.Id
      Name = [string]$process.Name
      Ports = (($item.Ports | Select-Object -Unique | Sort-Object) -join ",")
      CommandLine = [string]$process.CommandLine
    }
  }
}

$processes = @(Get-DunderIAProcess -Ports $Ports | Sort-Object Name, Id)

if ($Action -eq "list") {
  if ($processes.Count -eq 0) {
    Write-Host "No DunderIA dev services found."
    exit 0
  }
  $processes | Format-Table -AutoSize Id, Name, Ports, CommandLine
  exit 0
}

if ($processes.Count -eq 0) {
  Write-Host "No DunderIA dev services found."
  exit 0
}

foreach ($process in $processes) {
  if (-not $Force) {
    Write-Host ("Would stop {0} ({1}) ports=[{2}]. Re-run with -Force to stop." -f $process.Name, $process.Id, $process.Ports)
    continue
  }
  Stop-Process -Id $process.Id -Force
  Write-Host ("Stopped {0} ({1}) ports=[{2}]." -f $process.Name, $process.Id, $process.Ports)
}
