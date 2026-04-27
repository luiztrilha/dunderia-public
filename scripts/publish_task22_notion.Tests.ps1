$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$scriptPath = Join-Path $here 'publish_task22_notion.ps1'

Describe 'publish_task22_notion' {
    It 'supports explicit paths and dry-run without workstation-specific defaults' {
        $configPath = Join-Path $TestDrive 'config.json'
        $statePath = Join-Path $TestDrive 'broker-state.json'

        @'
{"one_api_key":"one_test_key"}
'@ | Set-Content -LiteralPath $configPath -Encoding UTF8

        @'
{"messages":[{"id":"msg-315","content":"# Workspace\n## Immediate Next Actions\n- Publish update"}]}
'@ | Set-Content -LiteralPath $statePath -Encoding UTF8

        $raw = & $scriptPath `
            -PageId 'page-123' `
            -PageUrl 'https://example.invalid/page-123' `
            -ConfigPath $configPath `
            -StatePath $statePath `
            -MessageId 'msg-315' `
            -ConnectionKey 'conn-test' `
            -AppendActionId 'append-test' `
            -DeleteActionId 'delete-test' `
            -DryRun

        $result = $raw | ConvertFrom-Json

        $result.page_id | Should Be 'page-123'
        $result.connection_key | Should Be 'conn-test'
        $result.append_action_id | Should Be 'append-test'
        $result.delete_action_id | Should Be 'delete-test'
        $result.state_path | Should Be $statePath
        $result.message_id | Should Be 'msg-315'
        $result.block_count | Should BeGreaterThan 0
    }
}
