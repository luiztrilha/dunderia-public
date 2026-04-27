# TOOLS.md

Notas locais e atalhos operacionais do workspace `D:\Repos`.

## Descoberta rapida

- Mapa de navegacao das regras, memoria e contratos do workspace:
  - `D:\Repos\STANDARDS-INDEX.md`
- Regra pratica:
  - usar o indice para descobrir a fonte canonica certa antes de abrir pastas e docs por tentativa

## Execucao segura de comandos

### Checklist rapido

- preferir executaveis diretos antes de chamar PowerShell:
  - `git`, `rg`, `dotnet`, `go`, `npm`, `node`
- quando PowerShell for necessario, preferir:
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File <script.ps1>`
- evitar:
  - `pwsh -Command` grande
  - `Invoke-Expression`
  - `-EncodedCommand`
  - `pwsh` dentro de `pwsh`
  - script improvisado em `%TEMP%`
  - here-string gigante so para montar comando
- preferir scripts curtos e versionados em:
  - `<repo>\scripts\`
  - `D:\Repos\scripts\`

### Exemplos preferidos

- Git direto:
  - `git status --short --branch`
- Busca direta:
  - `rg -n "RetornaCidadesUF" D:\Repos\ConveniosWebBNB_Novo`
- Dotnet direto:
  - `dotnet test D:\Repos\ConveniosWebBNB_Novo\ConveniosWebApi.sln`
- Script local do repo:
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File D:\Repos\dunderia\scripts\watch_legado_channel.ps1`
- Wrapper-base do workspace para execucao mais previsivel:
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File D:\Repos\scripts\Invoke-SafeWorkspaceCommand.ps1 -FilePath D:\Repos\dunderia\scripts\watch_legado_channel.ps1 -WorkingDirectory D:\Repos\dunderia`

### Gotcha de autostart

- neste host, launchers de logon baseados em `pwsh` podem ser bloqueados por heuristica de antivirus/EDR antes mesmo da execucao do script
- neste host, `wscript/cscript` + VBScript com health-check HTTP via `MSXML2.XMLHTTP` tambem pode ser bloqueado pela heuristica do antivirus
- mecanismo canonico de autostart do workspace: task `DunderIA User Logon Bootstrap` chamando `D:\Repos\dunderia\scripts\windows\start_user_logon_bootstrap.ps1`
- evidencia verificada em `2026-04-22`: `Start-ScheduledTask -TaskName 'DunderIA User Logon Bootstrap'` retornou `LastTaskResult=0` e atualizou `D:\Repos\dunderia\.wuphf\user-logon-bootstrap\bootstrap.jsonl`

## Runtime ativo

### Codex CLI

- Config ativa:
  - `C:\Users\l.sousa\.codex\config.toml`
- Skills locais:
  - `C:\Users\l.sousa\.codex\skills`
- Prompts OpenSpec:
  - `C:\Users\l.sousa\.codex\prompts\opsx-propose.md`
  - `C:\Users\l.sousa\.codex\prompts\opsx-explore.md`
  - `C:\Users\l.sousa\.codex\prompts\opsx-apply.md`
  - `C:\Users\l.sousa\.codex\prompts\opsx-archive.md`

### DunderIA como infraestrutura local de MCP

- Catalogo MCP local:
  - `D:\Repos\dunderia\mcp\dunderia-mcp-settings.json`
- Launchers MCP ativos:
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File D:\Repos\dunderia\scripts\launch_github_mcp.ps1`
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File D:\Repos\dunderia\scripts\launch_playwright_mcp.ps1`
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File D:\Repos\dunderia\scripts\launch_brave_mcp.ps1`
- GitHub MCP neste host:
  - o launcher usa `npx.cmd` em vez do shim PowerShell `npx.ps1`, evitando falha de autoload de modulo PowerShell
  - fallback explicito via GitHub CLI habilitado com `WUPHF_GITHUB_MCP_USE_GH_TOKEN=1`; reiniciar o Codex apos mudar variaveis persistidas com `setx`
- Watcher local:
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File D:\Repos\dunderia\scripts\watch_legado_channel.ps1`

### Local AI

- Modulo base:
  - `D:\Repos\dunderia\scripts\DunderIA.LocalAi.psm1`
- Insight visual:
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File D:\Repos\dunderia\scripts\Invoke-LocalVisionInsight.ps1 -InputPath "D:\caminho\arquivo.pdf"`
- Insight de media:
  - `pwsh -NoProfile -ExecutionPolicy Bypass -File D:\Repos\dunderia\scripts\Invoke-LocalMediaInsight.ps1 -InputPath "D:\caminho\arquivo.mp4"`

## Legado arquivado

- Arquivo frio do OpenClaw:
  - `D:\Repos\_archive\openclaw-decom-2026-04-19`
- Config antiga do Codex antes da remocao:
  - `D:\Repos\_archive\openclaw-decom-2026-04-19\codex\config.toml.before-openclaw-removal.toml`
- Tasks agendadas exportadas:
  - `D:\Repos\_archive\openclaw-decom-2026-04-19\workspace\scheduled-tasks-openclaw.json`

Regra pratica:

- consultar esse arquivo apenas para referencia historica, migracao ou recuperacao de conhecimento
- nao usar caminhos dentro do archive como runtime ativo

## Referencias externas locais

- `agent-rules-books`:
  - copia local: `D:\Repos\_archive\external-references\agent-rules-books`
  - upstream: `https://github.com/ciembor/agent-rules-books`
  - regra pratica: consultar a copia local primeiro quando precisar das lentes de engenharia por livros; atualizar via `git -C D:\Repos\_archive\external-references\agent-rules-books pull --ff-only` apenas quando quiser sincronizar com o upstream.

## Skills

### Fonte canonica

- Runtime de skills genericas de agente: `C:\Users\l.sousa\.agents\skills`
- Runtime de skills Codex e `.system`: `C:\Users\l.sousa\.codex\skills`

### Templates e overrides

- Templates/fontes versionadas: `D:\Repos\_tmp\impeccable\source\skills`, `D:\Repos\_tmp\superpowers\skills` e equivalentes
- Overrides repo-locais: `<repo>\.codex\skills`, `<repo>\.agents\skills`, `<repo>\.claude\skills`

## Repositorios

- Backend legado: `D:\Repos\ConveniosWebBNB`
- Backend novo: `D:\Repos\ConveniosWebBNB_Novo`
- Frontend: `D:\Repos\ConveniosWebExterno`
- Azure: `D:\Repos\ConveniosWebVSAzure_Default`
- Dados: `D:\Repos\ConveniosWebData`
- APIs Tectrilha: `D:\Repos\TectrilhaAPI`
- WebForms compartilhado: `D:\Repos\SistemasCompartilhadosWebForms`
- Transparencia Vue: `D:\Repos\TransparenciaWeb`
- Integracao local: `D:\Repos\dunderia`

## Codex

- Binario principal: `codex`
- Fallbacks disponiveis: `codex2`, `codex3`, `codex4`
- Observacao:
  - `D:\Repos\codex-lb` e um repositorio proprio
  - `D:\Repos` e uma junction para `D:\Repositorios`

## Distill

- Instalado localmente: `distill.ps1`
- Uso permitido apenas quando houver pedido explicito do usuario
- Config atual:
  - `%APPDATA%\distill\config.json`
- Teste rapido:
  - `'a`nb`nc' | distill "Return only the last line."`

## Regras praticas

- Se o pedido for sobre configuracao ativa do ambiente, olhar primeiro `C:\Users\l.sousa\.codex\config.toml` e `D:\Repos\dunderia\mcp\dunderia-mcp-settings.json`
- Se o pedido for sobre um repo, consultar primeiro o `AGENTS.md` do repo
- Se o pedido for sobre legado OpenClaw, consultar primeiro `D:\Repos\_archive\openclaw-decom-2026-04-19`
- Se um comando ou caminho virar recorrente, registrar aqui
