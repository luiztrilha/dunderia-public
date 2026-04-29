$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$scriptPath = Join-Path $here 'watch_legado_channel.ps1'

Describe 'watch_legado_channel' {
    It 'requires explicit flags for restart and mutation-capable behavior' {
        $scriptText = Get-Content -LiteralPath $scriptPath -Raw

        $scriptText | Should Match 'AllowBrokerRestart'
        $scriptText | Should Match 'AllowMutations'
        $scriptText | Should Not Match 'WindowStyle Hidden'
    }

    It 'flags out-of-scope drift and proposes a correction' {
        $statePath = Join-Path $TestDrive 'watch-state.json'
        $messagesPath = Join-Path $TestDrive 'messages.json'

        @'
{
  "channel": "legado-para-novo",
  "messages": [
    {
      "id": "msg-1",
      "from": "ceo",
      "channel": "legado-para-novo",
      "content": "Vamos refatorar o StudioApp do DunderIA em D:\\Repos\\dunderia e ajustar web/src/App.tsx",
      "timestamp": "2026-04-19T01:00:00Z"
    }
  ]
}
'@ | Set-Content -LiteralPath $messagesPath -Encoding UTF8

        $raw = & $scriptPath `
            -StatePath $statePath `
            -MessagesPath $messagesPath `
            -DryRun

        $result = $raw | ConvertFrom-Json

        $result.ok | Should Be $true
        $result.drift_detected | Should Be $true
        $result.latest_message_id | Should Be 'msg-1'
        $result.correction_message | Should Match 'ConveniosWebBNB_Antigo\\BNB'
        $result.correction_message | Should Match 'ConveniosWebExterno'
    }

    It 'ignores messages scoped to the four migration repos' {
        $statePath = Join-Path $TestDrive 'watch-state.json'
        $messagesPath = Join-Path $TestDrive 'messages.json'

        @'
{
  "channel": "legado-para-novo",
  "messages": [
    {
      "id": "msg-2",
      "from": "repo-auditor",
      "channel": "legado-para-novo",
      "content": "Mapeei BNB em D:\\Repos\\ConveniosWebBNB_Antigo\\BNB e equivalentes no frontend em D:\\Repos\\ConveniosWebExterno",
      "timestamp": "2026-04-19T01:10:00Z"
    }
  ]
}
'@ | Set-Content -LiteralPath $messagesPath -Encoding UTF8

        $raw = & $scriptPath `
            -StatePath $statePath `
            -MessagesPath $messagesPath `
            -DryRun

        $result = $raw | ConvertFrom-Json

        $result.ok | Should Be $true
        $result.drift_detected | Should Be $false
        $result.correction_message | Should BeNullOrEmpty
    }

    It 'does not flag its own scope correction as drift' {
        $statePath = Join-Path $TestDrive 'watch-state.json'
        $messagesPath = Join-Path $TestDrive 'messages.json'

        @'
{
  "channel": "legado-para-novo",
  "messages": [
    {
      "id": "msg-3",
      "from": "nex",
      "channel": "legado-para-novo",
      "content": "Correção automática de escopo: este canal deve tratar somente a conversão entre estes caminhos:\n- D:\\Repos\\ConveniosWebBNB_Antigo\\BNB\n- D:\\Repos\\ConveniosWebBNB_Antigo\\WSConvenio\n- D:\\Repos\\ConveniosWebBNB_Novo\n- D:\\Repos\\ConveniosWebExterno\n\nDesconsiderem referências a DunderIA, WUPHF, StudioApp, index.legacy.html, web/src ou qualquer outro repositório fora desses quatro diretórios.",
      "timestamp": "2026-04-19T01:20:00Z"
    }
  ]
}
'@ | Set-Content -LiteralPath $messagesPath -Encoding UTF8

        $raw = & $scriptPath `
            -StatePath $statePath `
            -MessagesPath $messagesPath `
            -DryRun

        $result = $raw | ConvertFrom-Json

        $result.ok | Should Be $true
        $result.drift_detected | Should Be $false
        $result.correction_message | Should BeNullOrEmpty
    }
}
