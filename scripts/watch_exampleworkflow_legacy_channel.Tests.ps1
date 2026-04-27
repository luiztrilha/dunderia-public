$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$scriptPath = Join-Path $here 'watch_ExampleWorkflow_legacy_channel.ps1'

Describe 'watch_ExampleWorkflow_legacy_channel' {
    It 'stays observe-only by default even outside dry-run when drift is detected' {
        $statePath = Join-Path $TestDrive 'watch-state.json'
        $logPath = Join-Path $TestDrive 'watch.jsonl'
        $snapshotPath = Join-Path $TestDrive 'snapshot.json'
        $messagesPath = Join-Path $TestDrive 'messages.json'
        $tasksPath = Join-Path $TestDrive 'tasks.json'

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "messages": [
    {
      "id": "msg-1",
      "from": "reviewer",
      "channel": "ExampleWorkflow-legacy",
      "content": "Bloqueio objetivo: o office precisa apontar para <REPOS_ROOT>\\LegacySystemOld para revisar RecursoHumano.Cadastro.aspx.cs.",
      "timestamp": "2026-04-19T20:11:39Z"
    }
  ]
}
'@ | Set-Content -LiteralPath $messagesPath -Encoding UTF8

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "tasks": [
    {
      "id": "task-1934",
      "channel": "ExampleWorkflow-legacy",
      "title": "Hotfix legado",
      "status": "blocked",
      "owner": "builder",
      "execution_mode": "external_workspace",
      "workspace_path": "<REPOS_ROOT>\\.wuphf"
    }
  ]
}
'@ | Set-Content -LiteralPath $tasksPath -Encoding UTF8

        $raw = & $scriptPath `
            -StatePath $statePath `
            -LogPath $logPath `
            -SnapshotPath $snapshotPath `
            -MessagesPath $messagesPath `
            -TasksPath $tasksPath

        $result = $raw | ConvertFrom-Json

        $result.ok | Should Be $true
        $result.workspace_repair_needed | Should Be $true
        $result.workspace_repaired | Should Be $false
        $result.correction_posted | Should Be $false
        $result.repair_actions_allowed | Should Be $false
        $result.restart_allowed | Should Be $false
        Test-Path -LiteralPath $snapshotPath | Should Be $true
    }

    It 'requires explicit flags for restart and mutation-capable behavior' {
        $scriptText = Get-Content -LiteralPath $scriptPath -Raw

        $scriptText | Should Match 'AllowBrokerRestart'
        $scriptText | Should Match 'AllowMutations'
        $scriptText | Should Not Match 'WindowStyle Hidden'
    }

    It 'detects wrong workspace and proposes repair to the legacy repo' {
        $statePath = Join-Path $TestDrive 'watch-state.json'
        $messagesPath = Join-Path $TestDrive 'messages.json'
        $tasksPath = Join-Path $TestDrive 'tasks.json'

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "messages": [
    {
      "id": "msg-1",
      "from": "reviewer",
      "channel": "ExampleWorkflow-legacy",
      "content": "Bloqueio objetivo: o office precisa apontar para <REPOS_ROOT>\\LegacySystemOld para revisar RecursoHumano.Cadastro.aspx.cs.",
      "timestamp": "2026-04-19T20:11:39Z"
    }
  ]
}
'@ | Set-Content -LiteralPath $messagesPath -Encoding UTF8

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "tasks": [
    {
      "id": "task-1934",
      "channel": "ExampleWorkflow-legacy",
      "title": "Hotfix legado",
      "status": "blocked",
      "owner": "builder",
      "execution_mode": "external_workspace",
      "workspace_path": "<REPOS_ROOT>\\.wuphf"
    }
  ]
}
'@ | Set-Content -LiteralPath $tasksPath -Encoding UTF8

        $raw = & $scriptPath `
            -StatePath $statePath `
            -MessagesPath $messagesPath `
            -TasksPath $tasksPath `
            -DryRun

        $result = $raw | ConvertFrom-Json

        $result.ok | Should Be $true
        $result.workspace_repair_needed | Should Be $true
        $result.workspace_repaired | Should Be $false
        $result.repaired_task_id | Should Be 'task-1934'
        $result.repaired_workspace_path | Should Match 'LegacySystemOld'
        $result.correction_message | Should Match 'workspace correto'
        $result.correction_message | Should Match '\.wuphf'
        $result.correction_message | Should Not Match '\$\('
    }

    It 'flags repeated identical replies in the same thread' {
        $statePath = Join-Path $TestDrive 'watch-state.json'
        $messagesPath = Join-Path $TestDrive 'messages.json'
        $tasksPath = Join-Path $TestDrive 'tasks.json'

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "messages": [
    {
      "id": "msg-2",
      "from": "reviewer",
      "channel": "ExampleWorkflow-legacy",
      "reply_to": "msg-root",
      "content": "Sem patch novo para reavaliar; meu parecer continua em vermelho.",
      "timestamp": "2026-04-19T20:10:15Z"
    },
    {
      "id": "msg-3",
      "from": "reviewer",
      "channel": "ExampleWorkflow-legacy",
      "reply_to": "msg-root",
      "content": "Sem patch novo para reavaliar; meu parecer continua em vermelho.",
      "timestamp": "2026-04-19T20:10:22Z"
    }
  ]
}
'@ | Set-Content -LiteralPath $messagesPath -Encoding UTF8

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "tasks": []
}
'@ | Set-Content -LiteralPath $tasksPath -Encoding UTF8

        $raw = & $scriptPath `
            -StatePath $statePath `
            -MessagesPath $messagesPath `
            -TasksPath $tasksPath `
            -DryRun

        $result = $raw | ConvertFrom-Json

        $result.ok | Should Be $true
        $result.loop_detected | Should Be $true
        $result.correction_message | Should Match 'repetição idêntica'
    }

    It 'prefers the most recent explicit workspace mention and treats mismatched git roots as drift' {
        $statePath = Join-Path $TestDrive 'watch-state.json'
        $messagesPath = Join-Path $TestDrive 'messages.json'
        $tasksPath = Join-Path $TestDrive 'tasks.json'

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "messages": [
    {
      "id": "msg-20",
      "from": "reviewer",
      "channel": "ExampleWorkflow-legacy",
      "content": "Contexto antigo: usar <REPOS_ROOT>\\LegacySystemOld.",
      "timestamp": "2026-04-19T20:10:15Z"
    },
    {
      "id": "msg-21",
      "from": "workflow-architect",
      "channel": "ExampleWorkflow-legacy",
      "content": "Correção mais recente: antes estava <REPOS_ROOT>\\LegacySystemOld, mas o root desta sessão é <REPOS_ROOT>\\LegacySystemOld\\WSExampleAgreement.",
      "timestamp": "2026-04-19T20:10:30Z"
    }
  ]
}
'@ | Set-Content -LiteralPath $messagesPath -Encoding UTF8

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "tasks": [
    {
      "id": "task-20",
      "channel": "ExampleWorkflow-legacy",
      "title": "Workspace drift",
      "status": "in_progress",
      "owner": "builder",
      "execution_mode": "external_workspace",
      "workspace_path": "<REPOS_ROOT>\\LegacySystemOld"
    }
  ]
}
'@ | Set-Content -LiteralPath $tasksPath -Encoding UTF8

        $raw = & $scriptPath `
            -StatePath $statePath `
            -MessagesPath $messagesPath `
            -TasksPath $tasksPath `
            -DryRun

        $result = $raw | ConvertFrom-Json

        $result.ok | Should Be $true
        $result.workspace_repair_needed | Should Be $true
        $result.repaired_task_id | Should Be 'task-20'
        $result.repaired_workspace_path | Should Be '<REPOS_ROOT>\LegacySystemOld\WSExampleAgreement'
    }

    It 'persists a recoverable snapshot when not running in dry-run mode' {
        $statePath = Join-Path $TestDrive 'watch-state.json'
        $logPath = Join-Path $TestDrive 'watch.jsonl'
        $snapshotPath = Join-Path $TestDrive 'snapshot.json'
        $messagesPath = Join-Path $TestDrive 'messages.json'
        $tasksPath = Join-Path $TestDrive 'tasks.json'

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "messages": [
    {
      "id": "msg-10",
      "from": "ceo",
      "channel": "ExampleWorkflow-legacy",
      "content": "Persist this snapshot.",
      "timestamp": "2026-04-19T20:10:15Z"
    }
  ]
}
'@ | Set-Content -LiteralPath $messagesPath -Encoding UTF8

        @'
{
  "channel": "ExampleWorkflow-legacy",
  "tasks": [
    {
      "id": "task-10",
      "channel": "ExampleWorkflow-legacy",
      "title": "Persisted snapshot task",
      "status": "in_progress",
      "owner": "builder",
      "execution_mode": "external_workspace",
      "workspace_path": "<REPOS_ROOT>\\LegacySystemOld"
    }
  ]
}
'@ | Set-Content -LiteralPath $tasksPath -Encoding UTF8

        $raw = & $scriptPath `
            -StatePath $statePath `
            -LogPath $logPath `
            -SnapshotPath $snapshotPath `
            -MessagesPath $messagesPath `
            -TasksPath $tasksPath

        $result = $raw | ConvertFrom-Json
        $snapshot = Get-Content -LiteralPath $snapshotPath -Raw | ConvertFrom-Json

        $result.ok | Should Be $true
        Test-Path -LiteralPath $snapshotPath | Should Be $true
        $snapshot.channel | Should Be 'ExampleWorkflow-legacy'
        $snapshot.messages[0].id | Should Be 'msg-10'
        $snapshot.tasks[0].id | Should Be 'task-10'
    }
}
