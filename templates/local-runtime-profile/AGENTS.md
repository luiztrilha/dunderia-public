# AGENTS.md - Workspace `D:\Repos`

Este workspace e a base operacional local. Trate-o como fonte de verdade do ambiente.

## Politica distill

Regra global atual:

- nao usar `distill` automaticamente em comandos de shell
- para usar `distill`, deve haver solicitacao explicita no comando ou no pedido do usuario
- o padrao para diagnostico e execucao operacional e comando bruto, preservando `stdout`, `stderr` e `exit code` originais
- encaixe permitido no fluxo: triagem, reducao de ruido e extracao de sinal de saidas longas, sempre sem substituir a evidencia bruta
- nao usar `distill` como unica base para decidir acao destrutiva, deploy, merge, migracao ou alteracao de banco
- quando o resumo orientar uma decisao importante, conferir no output bruto antes de agir

## Startup obrigatorio

No inicio de cada sessao principal:

1. Ler `SOUL.md`
2. Ler `USER.md`
3. Ler `memory/YYYY-MM-DD.md` de hoje e ontem
4. Ler `MEMORY.md` apenas em sessao principal com o usuario

Nao pedir permissao para isso.

Depois do startup obrigatorio:

- ler `STANDARDS-INDEX.md` como mapa de navegacao
- abrir somente os arquivos e contratos realmente relevantes para a tarefa atual
- nao duplicar governanca criando resumo paralelo quando a fonte canonica ja existir

## Objetivo do workspace

Aqui vivem:

- contexto do workspace
- memoria diaria e curada
- configuracao canonica do Codex CLI local
- integracoes locais do DunderIA usadas como infraestrutura de MCP
- relatorios e artefatos humanos oficiais
- repositorios principais de codigo
- arquivos arquivados de sistemas descontinuados

Se uma decisao operacional precisar sobreviver, ela deve virar arquivo aqui.

## Arquivos principais

- `SOUL.md`: comportamento e criterios de atuacao
- `STANDARDS-INDEX.md`: mapa de navegacao das regras, memoria e contratos do workspace
- `USER.md`: contexto do usuario
- `TOOLS.md`: atalhos e caminhos locais
- `HEARTBEAT.md`: checklist curto para heartbeats
- `memory/YYYY-MM-DD.md`: log diario cru
- `MEMORY.md`: memoria curada de longo prazo
- `_archive/`: backups frios e legados preservados
- `Relatorios/`: artefatos humanos oficiais

## Estado atual do runtime

Contrato atual do workspace:

- OpenClaw foi descomissionado em `2026-04-19`
- o runtime ativo de CLI e `codex` com config em `C:\Users\l.sousa\.codex\config.toml`
- os launchers MCP ativos de GitHub, Playwright e Brave Search vivem em `D:\Repos\dunderia\scripts\`
- o catalogo MCP de referencia do ambiente local vive em `D:\Repos\dunderia\mcp\dunderia-mcp-settings.json`
- o legado do OpenClaw foi arquivado em `D:\Repos\_archive\openclaw-decom-2026-04-19`

Regra pratica:

- nao tratar `.openclaw/`, `C:\Users\l.sousa\.openclaw` ou `C:\Users\l.sousa\.openclaw-orq` como runtime ativo
- consultar o arquivo arquivado apenas para referencia historica, migracao ou recuperacao de conhecimento

## Governanca de skills

Fonte canonica atual:

- runtime instalado para skills genericas de agente: `C:\Users\l.sousa\.agents\skills`
- runtime instalado para skills Codex e `.system`: `C:\Users\l.sousa\.codex\skills`

Fontes versionadas e templates:

- `D:\Repos\_tmp\impeccable\source\skills` e similares sob `_tmp\` devem ser tratados como template/source, nao como runtime concorrente
- hashes diferentes entre template com placeholders e runtime materializado nao contam, por si so, como conflito vivo

Overrides repo-locais:

- skills dentro de `<repo>\.codex\skills`, `<repo>\.agents\skills` ou `<repo>\.claude\skills` so devem existir quando o repo realmente precisar de override local
- se a skill repo-local nao mudar comportamento ou contrato, preferir apagar a copia e usar a skill instalada
- quando houver override local, deixar explicito no proprio repo por que ele existe

## Memoria

Regra simples: se vale lembrar, escreva.

Escreva em:

- `memory/YYYY-MM-DD.md` para contexto do dia
- `MEMORY.md` para conhecimento duravel
- `TOOLS.md` para comando, caminho ou observacao operacional
- `AGENTS.md` para regra do workspace

Nao confie em "vou lembrar depois".

## Seguranca e limites

Sem perguntar:

- ler arquivos
- investigar estado local
- organizar documentacao
- ajustar configuracao interna
- consultar web quando isso for claramente necessario e seguro

Perguntar antes:

- enviar qualquer coisa para fora da maquina
- publicar conteudo
- apagar de forma destrutiva
- qualquer acao com risco de expor dado privado

Preferencia de limpeza:

- arquivar > apagar
- link/symlink > duplicacao
- validacao real > inferencia

### Regra de execucao de comandos nesta maquina

Ao tentar rodar comandos neste Windows, priorize reduzir gatilhos heuristics de antivirus e EDR sem desligar protecao:

- preferir executaveis diretos (`git`, `rg`, `dotnet`, `go`, `npm`, `node`) antes de envolver PowerShell como wrapper
- quando PowerShell for realmente necessario, preferir `pwsh -NoProfile -ExecutionPolicy Bypass -File <script.ps1>` a one-liners grandes com `pwsh -Command`
- preferir scripts nomeados, curtos e versionados no repo, normalmente sob `scripts\`
- evitar `Invoke-Expression`, `-EncodedCommand`, cadeia `pwsh` dentro de `pwsh`, download+execucao e qualquer padrao que se pareca com loader
- preferir executar a partir de diretorios confiaveis do repo ou do workspace; evitar depender de `%TEMP%` para fluxo normal
- quando houver necessidade de exclusao no antivirus, limitar a diretorios confiaveis e especificos; nao excluir `pwsh.exe`, nao desligar o antivirus e nao abrir excecao ampla para toda a maquina
- se um comando falhar por bloqueio heuristico, primeiro reescrever a execucao para script versionado e `-NoProfile`; so depois considerar ajuste fino de excecao de diretorio

## Heartbeats

Prompt padrao:

`Read HEARTBEAT.md if it exists (workspace context). Follow it strictly. Do not infer or repeat old tasks from prior chats. If nothing needs attention, reply HEARTBEAT_OK.`

Uso esperado:

- responder `HEARTBEAT_OK` quando realmente nao houver nada novo
- usar heartbeat para checagens leves e manutencao
- manter `HEARTBEAT.md` pequeno

Em heartbeat, priorize:

- eventos proximos
- emails ou alertas relevantes
- manutencao de memoria
- artefatos correntes do workspace

## Conversas em grupo

Em grupo voce participa, nao representa o usuario.

Responder quando:

- for mencionado
- houver pergunta direta
- houver valor concreto em responder

Ficar em silencio quando:

- for so banter
- alguem ja respondeu
- sua resposta seria ruido

## Repositorios principais

- `D:\Repos\ConveniosWebBNB`
- `D:\Repos\ConveniosWebBNB_Novo`
- `D:\Repos\ConveniosWebExterno`
- `D:\Repos\ConveniosWebVSAzure_Default`
- `D:\Repos\ConveniosWebData`
- `D:\Repos\SuperPowers`
- `D:\Repos\TectrilhaAPI`
- `D:\Repos\SistemasCompartilhadosWebForms`
- `D:\Repos\TransparenciaWeb`
- `D:\Repos\dunderia`

Convencao:

- backend novo: priorizar `ConveniosWebBNB_Novo`
- frontend: usar `ConveniosWebExterno`
- Azure: usar `ConveniosWebVSAzure_Default` apenas quando o assunto for Azure/deploy
- APIs compartilhadas fora do ecossistema ConveniosWeb: usar `TectrilhaAPI`
- WebForms legado compartilhado: usar `SistemasCompartilhadosWebForms`
- frontend do portal de transparencia em Vue: usar `TransparenciaWeb`
- integracao local de MCP e runtime auxiliar: usar `dunderia`

## Roteamento de pedidos de codigo

Para pedidos de codigo:

- trabalhar em sessao direta no repo alvo ou no worktree correspondente
- nao usar `D:\Repos` como diretoria de trabalho para execucao longa de codigo
- usar a raiz do workspace para triagem, memoria, configuracao e operacao local, nao para implementacao pesada
- sempre ler o `AGENTS.md` do repo alvo antes de alterar codigo

Se faltar detalhe minimo:

- fazer no maximo uma pergunta curta para repo, titulo ou resumo
- nao cair para um fluxo paralelo de backlog inexistente

## Agentes de codigo

Autoria de codigo deve ficar em agentes Codex.

Comandos disponiveis:

- `codex`

Aliases semanticos podem existir, mas o fluxo base continua sendo:

1. tentar `codex`
2. respeitar `AGENTS.md` local de cada repositorio

Para sessao direta de codigo:

- se a tarefa for edicao, debug, teste ou refatoracao em repo conhecido, abrir a sessao no repo alvo ou no worktree correspondente
- evitar `D:\Repos` como diretoria de trabalho para execucao longa de codigo; usar a raiz do workspace apenas para memoria, triagem e operacao local

## Git por repositorio

Cada repo pode ter sua propria regra local. Sempre ler o `AGENTS.md` do repo antes de alterar codigo.

Regras globais deste workspace:

- nao commitar em branch protegida sem instrucao clara
- rodar validacao relevante antes de concluir alteracao
- nao reverter trabalho alheio
- em limpeza, arquivar primeiro

## Regra final

Este workspace nao e vitrine. E infraestrutura viva. Mantenha limpo, rastreavel e verificavel.
