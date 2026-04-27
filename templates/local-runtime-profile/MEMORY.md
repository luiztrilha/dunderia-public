# MEMORY

Memoria curada de longo prazo do workspace `D:\Repos`.

Regra operacional:

- registrar aqui apenas decisoes, contratos e fatos duraveis
- preferir fatos verificaveis e caminhos concretos
- evitar duplicatas; promover novidade real

## Nucleo duravel

- Este arquivo guarda memoria curada e verificavel do workspace; o log cru do dia continua em `memory/YYYY-MM-DD.md`.
- O workspace continua como fonte de verdade: regras em `AGENTS.md`, comportamento em `SOUL.md`, contexto humano em `USER.md`, atalhos em `TOOLS.md`.
- Em `2026-04-19`, o OpenClaw foi descomissionado como runtime ativo do workspace; o estado preservado foi arquivado em `D:\Repos\_archive\openclaw-decom-2026-04-19`, e o runtime local ativo passou a ser `codex` com MCPs puxados de `D:\Repos\dunderia\scripts\` e `D:\Repos\dunderia\mcp\dunderia-mcp-settings.json`.
- As referencias historicas ao OpenClaw e ao swarm abaixo permanecem como memoria de arquitetura e operacao passada, nao como contrato ativo do workspace.
- No OpenClaw/swarm, os contratos mais duraveis hoje sao: selecao de contexto por task antes do worker, supressoes explicitas para nao reexecutar certos tipos de task, governanca por lanes/perfis, e validacao observavel antes de reportar conclusao.
- No ChamadoWeb, os contratos mais duraveis hoje sao: bridge canonico no workspace, casos assistidos podem virar apoio/manual-intake sem task de codigo, produto `ConveniosWeb` deve rotear para `ConveniosWebBNB_Novo`, chamado nunca deve ser marcado como `atendido` automaticamente, e a captura automatica de chamados abertos via API ficou fora do fluxo padrao.
- No runtime local, o dashboard/bridge, o intake de email restaurado, o worker Codex no Windows e o handoff portatil derivado viraram capacidades estaveis do ambiente e nao apenas experimentos pontuais.
- No runtime global do Codex, um pack local de workflow skills leves passou a cobrir debug sistematico, verificacao antes de encerrar, review findings-first e planejamento conciso, sem importar frameworks externos inteiros nem ativar uma skill-mestra dominando o fluxo.
- O runtime do Codex/OpenClaw agora tambem trata MegaMemory como recurso por repositorio: cada repo Git de primeiro nivel em `D:\Repos` tem `knowledge.db` proprio e MCP dedicado, evitando mistura entre OpenClaw, produtos e utilitarios.
- No OpenClaw, a linha atual de evolucao inspirada no Superset privilegia copiar UX de workspace/presets/review sem trocar o nucleo local do swarm nem depender de cloud como fonte de verdade.
- O workspace agora usa `STANDARDS-INDEX.md` como mapa leve de navegacao entre regras, memoria e runtime, sem criar segunda fonte de verdade; `AGENTS.md` aponta para esse indice apos o startup obrigatorio, `TOOLS.md` o expoe como descoberta rapida e os repositorios principais espelham uma secao curta de `Navegacao minima`.
- O workspace passou a tratar OpenSpec como capacidade repo-local padronizada nos repositorios principais: `codex-lb` continua OpenSpec-first, `ConveniosWebBNB_Novo` e `ConveniosWebVSAzure_Default` mantem pilotos existentes, e os demais repos principais ganharam scaffold minimo (`openspec/config.yaml`, `openspec/README.md`, `openspec/changes/README.md`) mais contrato explicito em `AGENTS.md`.
- No setup local observado com OpenSpec `1.2.0`, a integracao com Codex fica efetivamente global em `C:\Users\l.sousa\.codex\prompts\opsx-*.md` apos `openspec init --tools codex`; os repositorios mantem o scaffold `openspec/` local, mas `openspec update <repo>` nao se mostrou um verificador confiavel dessa integracao.
- O recovery atual do DunderIA usa bundle imutável como artefato canônico offsite: cada execução gera `dunderia-recovery-bundle-YYYYMMDD-HHMMSS.zip` com `dunderia-state.zip`, `dunderia-secrets.vault`, manifests, `restore.md`, `AI-RESTORE-PROMPT.md` e scripts mínimos; o drill semanal valida o bundle mais recente no Google Drive em vez do par solto `state+vault`.
- O repositorio externo `ciembor/agent-rules-books` foi absorvido como referencia instrucional local em `D:\Repos\_archive\external-references\agent-rules-books` e apontado no `C:\Users\l.sousa\.codex\AGENTS.md` global, nao como dependencia instalada: usar `unified-software-engineering` como lente ampla quando faltar uma regra local mais especifica, e adicionar no maximo uma lente especializada por tarefa (`refactoring`, `working-effectively-with-legacy-code`, `release-it`, `designing-data-intensive-applications`, DDD ou arquitetura) quando ela mudar a decisao tecnica.

## Mapa tematico

- Contrato e governanca: `2026-03-22 | Contrato local de contexto e memoria`, `2026-03-26 | Baseline canonico de hardening para autonomia`, `2026-03-26 | Honestidade operacional baseada em verificacao`, `2026-04-02 | Supressao explicita para nao reexecutar tasks arquivadas pelo usuario`.
- Swarm e workers: `2026-03-25 | Watchdog de tasks ativas do swarm`, `2026-03-28 | Governanca de execucao por lanes fast medium high`, `2026-03-29 | Executor Codex do swarm precisa resolver config pelo runtime pai`, `2026-04-01 | Worker Codex do swarm deve preferir shim de aplicacao no Windows...`.
- Dashboard e runtime: `2026-03-27 | Dashboard operacional com centro de comando para tasks e chamados`, `2026-03-28 | Bridge do dashboard endurecido...`, `2026-04-02 | Dashboard local ganhou slice de runtime OpenClaw...`, `2026-04-02 | Aba Runtime > OpenClaw ganhou acoes operacionais inline via bridge`, `2026-04-03 | Dashboard ganhou presets configuraveis e catalogo visual de workspaces inspirado no Superset`.
- Skills e documentos Office: `2026-04-14 | Heuristicas office-docs foram absorvidas como skills locais e bridge do OpenClaw`.
- OpenSpec e governanca de mudanca: `2026-04-08 | Rollout repo-local de OpenSpec nos repositorios principais do workspace`, `2026-04-08 | Integracao OpenSpec com Codex fica global em ~/.codex/prompts no setup local`.
- Referencias externas de engenharia: `2026-04-25 | agent-rules-books absorvido como lente opcional de engenharia`.
- Contexto, memoria e handoff: `2026-04-03 | Piloto de selecao de contexto por task antes do worker`, `2026-04-03 | Heartbeat rotativo, limpeza de sessao no /new e busca SQLite auxiliar de memoria`, `2026-04-03 | Handoff portatil derivado do contexto local`, `2026-04-03 | Workflow skills locais leves no runtime global do Codex`, `2026-04-03 | MegaMemory dedicado por repositorio no workspace`, `2026-04-11 | STANDARDS-INDEX oficializa navegacao leve entre workspace e repositorios`.
- ChamadoWeb e intake: `2026-03-25 | Bridge inicial entre chamados e swarm`, `2026-03-25 | Chamados assistidos nao devem virar task de codigo`, `2026-03-25 | Auxilio schema-aware para chamados ConveniosWeb`, `2026-04-02 | Sync ChamadoWeb passou a ser idempotente...`, `2026-04-03 | Nunca marcar chamado como atendido automaticamente`, `2026-04-16 | Captura automatica de chamados abertos via API saiu do fluxo padrao`.
- Swarm e repos: `2026-04-13 | SuperPowers, ChamadoWebAPI, ChamadoWebExterno e ConveniosWebData saíram da auditoria automatica; email intake (openclaw@tectrilha.com.br) foi desativado e componente removido do fluxo`, `2026-04-16 | Auditoria automatica dos repositorios ativos saiu do fluxo padrao do workspace`.
- Plano de controle conversacional: `2026-04-16 | Telegram saiu do fluxo padrao do workspace; intake, scheduler e healthcheck deixaram de depender dele`.

## Registro detalhado

### 2026-04-25 | workspace/codex | agent-rules-books absorvido como lente opcional de engenharia
- O repositorio publico `https://github.com/ciembor/agent-rules-books` foi verificado como uma colecao MIT de regras operacionais para Codex, Cursor e Claude Code inspiradas em livros de engenharia de software, e clonado localmente em `D:\Repos\_archive\external-references\agent-rules-books`.
- Contrato local/global: nao instalar nem copiar automaticamente seus `AGENTS.md` sobre os repositorios do workspace; tratar o material como referencia instrucional global do Codex via `C:\Users\l.sousa\.codex\AGENTS.md` e aplicar seletivamente durante planejamento, implementacao, review e refatoracao.
- Uso recomendado: partir de `unified-software-engineering` como lente ampla quando a tarefa pedir julgamento geral de engenharia; combinar temporariamente com apenas uma lente especializada quando houver necessidade clara, por exemplo `working-effectively-with-legacy-code` para legado sem testes, `refactoring` para mudancas estruturais, `release-it` para caminhos produtivos e dependencias externas, `designing-data-intensive-applications` para consistencia/eventos/dados, ou DDD/arquitetura para modelagem e limites.
- Criterio pratico: as regras locais do workspace, `AGENTS.md` do repo alvo, skills locais e verificacao real continuam tendo precedencia; a referencia externa serve para melhorar heuristicas, nao para criar governanca paralela.
- Fontes verificadas em 2026-04-25: `README.md` do repo e `unified-software-engineering/codex/AGENTS.md`; copia local no commit `2851b85`.

### 2026-04-16 | workspace/openclaw | Captura automatica de chamados abertos via API saiu do fluxo padrao
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` passou a manter `chamadoWebSync.enabled = false`, impedindo que o bootstrap do ambiente ligue novamente o runner de sincronizacao de chamados abertos.
- `D:\Repos\.openclaw\runtime\swarm\workspace-presets.json`, `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` e `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1` deixaram de expor o preset `chamados-sync` e trocaram o destaque do `ChamadoWebAPI` para `openclaw-runtime`.
- O bridge/dashboard tambem deixou de aceitar a acao `syncChamados`; o suporte manual a `createChamado` e `updateChamado` foi preservado.
- Evidencia verificada em 2026-04-16: os contratos JSON continuaram parseando, o dashboard foi regenerado sem o preset `chamados-sync` e o runtime passou a refletir `chamadoWebSync.enabled = false`.
- Contrato duravel: o workspace continua podendo operar chamados manualmente, mas nao deve mais capturar automaticamente chamados abertos direto da API como parte do fluxo padrao.

### 2026-04-19 | workspace/dunderia | Recovery offsite passou a usar bundle imutavel como artefato canonico
- `D:\Repos\_tmp\dunderia-runtime-fix\scripts\Invoke-DunderIARecoveryBackup.ps1` passou a gerar, a cada execucao, `dunderia-recovery-bundle-YYYYMMDD-HHMMSS.zip` em `C:\Users\l.sousa\.wuphf\recovery-backups\archives\`, mais o alias local `latest\dunderia-recovery-bundle-latest.zip`.
- O bundle encapsula `dunderia-state.zip`, `dunderia-secrets.vault`, `backup-manifest.json`, manifests de state/secret, `restore.md`, `AI-RESTORE-PROMPT.md` e os scripts `Restore-DunderIARecoveryBackup.ps1`, `Restore-DunderIARecoveryBundle.ps1` e `DunderIA.Recovery.psm1`.
- `D:\Repos\_tmp\dunderia-runtime-fix\scripts\Test-DunderIARecoveryBundle.ps1` virou o drill canônico do artefato offsite: ele resolve o bundle mais recente em `RecoveryBackups\archives`, executa restore em pasta temporária e exige `.wuphf\team\broker-state.json`, `.codex\config.toml` e `.codex\auth.json`.
- `D:\Repos\_tmp\dunderia-runtime-fix\scripts\Register-DunderIAWeeklyRecoveryDrill.ps1` passou a registrar a task `DunderIA-Weekly-Recovery-Drill` contra `Test-DunderIARecoveryBundle.ps1`, com `SummaryLabel = google-drive-bundle-latest`.
- Evidência verificada em `2026-04-19`: `Invoke-Pester` passou em `Invoke-DunderIARecoveryBackup.Tests.ps1` e `Test-DunderIARecoveryBackup.Tests.ps1`; `Invoke-DunderIARecoveryBackup.ps1 -EmitEventLog -Json` gerou bundle real `dunderia-recovery-bundle-20260419-160739.zip` com `googleDrive.verified = true`; `Test-DunderIARecoveryBundle.ps1` restaurou com sucesso o bundle mais recente em `C:\Users\l.sousa\Meu Drive\05_TECNOLOGIA_E_SOFTWARE\DunderIA\RecoveryBackups`.

### 2026-04-16 | workspace/openclaw | Auditoria automatica dos repositorios ativos saiu do fluxo padrao
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` passou a manter `repoAudit.enabled = false`, e o planner/supervisor/doctor passaram a respeitar esse flag para nao bloquear pipeline nem frescor por falta de `autonomous-code-audit.md`.
- `D:\Repos\.openclaw\scripts\Set-OpenClawScheduledTasksSilent.ps1` deixou de recriar as tasks `OpenClaw Audit * Direct` e `OpenClaw Swarm Daily Audit Summary Direct` quando a auditoria automatica estiver desligada.
- `D:\Repos\.openclaw\OPENCLAW-OPERATING-CONTRACT.md` e `D:\Repos\.openclaw\README.md` foram alinhados ao novo contrato: auditoria agora so entra por execucao manual/explicita.
- Contrato duravel: o swarm continua operando backlog manual/OpenSpec e tasks preservadas sem depender de auditoria recorrente dos repositorios.

### 2026-04-16 | workspace/openclaw | Telegram saiu do fluxo padrao do workspace
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` passou a manter `telegram.enabled = false`, `telegram.intakeEnabled = false` e `telegram.notificationsEnabled = false`.
- `D:\Repos\.openclaw\scripts\Start-OpenClawEnvironment.ps1`, `D:\Repos\.openclaw\scripts\Invoke-OpenClawDoctor.ps1` e `D:\Repos\.openclaw\scripts\Set-OpenClawScheduledTasksSilent.ps1` passaram a tratar Telegram como componente desativado, em vez de exigir runner/task saudavel.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Add-SwarmTask.ps1` deixou de usar `manual://telegram-orquestrador` como origem default e passou a usar `manual://orquestrador-manual`.
- `D:\Repos\AGENTS.md`, `D:\Repos\TOOLS.md`, `D:\Repos\.openclaw\README.md` e `D:\Repos\.openclaw\OPENCLAW-OPERATING-CONTRACT.md` foram alinhados para remover Telegram do fluxo padrao.
- Contrato duravel: Telegram fica fora do caminho critico do workspace ate reativacao explicita.

### 2026-04-14 | workspace/openclaw | Heuristicas office-docs foram absorvidas como skills locais e bridge do OpenClaw
- As paginas externas `clawhub.ai/ivangdavila/word-docx`, `excel-xlsx` e `powerpoint-pptx` foram tratadas como referencia instrucional de baixo risco; o workspace nao passou a depender delas nem a instala-las como skill externa obrigatoria.
- O runtime local do Codex absorveu o que era util em skills proprias: `C:\Users\l.sousa\.codex\skills\doc` passou a cobrir round-trip fragil de Word, estilos, numeracao, secoes, campos, comentarios e tracked changes; `C:\Users\l.sousa\.codex\skills\xlsx` passou a cobrir preservacao de formulas, tipos, datas e estrutura de workbook; `C:\Users\l.sousa\.codex\skills\pptx` passou a cobrir placeholders, layouts, masters, notas, charts e QA visual de deck.
- `C:\Users\l.sousa\.codex\AGENTS.md` passou a expor `doc`, `xlsx` e `pptx` na lista de local workflow skills, com prompts curtos em `agents/openai.yaml` para descoberta deterministica.
- O pack embutido `D:\Repos\.openclaw\runtime\skills\openclaw-workspace` passou a republicar esses guias em `docs/local-workflow-skills.md`; `D:\Repos\.openclaw\scripts\Build-OpenClawEmbeddedSkills.ps1` foi ajustado para incluir os `SKILL.md` locais como `sourcePaths` no bridge.
- Evidencia verificada em 2026-04-14: `python C:\Users\l.sousa\.codex\skills\.system\skill-creator\scripts\quick_validate.py` retornou `Skill is valid!` para `doc`, `xlsx` e `pptx`; `D:\Repos\Relatorios\OpenClaw\embedded-skills.json` registrou `generatedAt = 2026-04-14T17:02:08.3306458Z` e os `sourcePaths` para os tres skills; `D:\Repos\Relatorios\OpenClaw\openclaw-runtime.json` permaneceu com `runtimePackCount = 5`.
- Contrato duravel: para formatos Office no workspace, preferir skills locais curadas e bridge embutido do OpenClaw; usar skills externos apenas como fonte de ideias, nunca como memoria paralela ou dependencia automatica do runtime.

### 2026-04-14 | workspace/openclaw | self-improvement-lite absorveu criterios de sinal do skill externo sem herdar memoria paralela
- A pagina externa `clawhub.ai/ivangdavila/self-improving` foi tratada como referencia instrucional de baixo risco, mas o workspace nao adotou `~/self-improving/`, `projects/`, `domains/`, `archive/`, `corrections.md` nem heartbeat-state paralelo.
- O que foi incorporado em `C:\Users\l.sousa\.codex\skills\self-improvement-lite` foram apenas os criterios uteis de triagem: distinguir `explicit_correction`, `explicit_preference`, `self_reflection` e `repeated_operational_evidence` de sinais fracos; proibir aprendizado por silencio; e tratar recorrencia como gatilho de avaliacao, nao como promocao automatica.
- O prompt curto da skill em `C:\Users\l.sousa\.codex\skills\self-improvement-lite\agents\openai.yaml` passou a orientar explicitamente a ignorar silencio, instrucoes pontuais e sinais especulativos.
- O bridge `D:\Repos\.openclaw\runtime\skills\openclaw-workspace\docs/local-workflow-skills.md` passou a republicar a secao `Signal Quality`, mantendo a descoberta do runtime alinhada ao contrato local.
- Evidencia verificada em 2026-04-14: `python C:\Users\l.sousa\.codex\skills\.system\skill-creator\scripts\quick_validate.py C:\Users\l.sousa\.codex\skills\self-improvement-lite` retornou `Skill is valid!`; `Build-OpenClawEmbeddedSkills.ps1 -Json` atualizou `D:\Repos\Relatorios\OpenClaw\embedded-skills.json` em `2026-04-14T17:13:11.0544339Z`; `local-workflow-skills.md` passou a conter `## Signal Quality`.
- Contrato duravel: no workspace `D:\Repos`, aprendizado local continua obrigado a usar arquivos canonicos existentes e filtros de evidencia forte; skills externos de self-improvement servem como fonte de heuristicas, nunca como segunda camada de memoria.

### 2026-04-11 | workspace | STANDARDS-INDEX oficializa navegacao leve entre workspace e repositorios
- O workspace ganhou `D:\Repos\STANDARDS-INDEX.md` como mapa leve de navegacao para descobrir a fonte canonica certa sem introduzir um novo centro de governanca.
- `D:\Repos\AGENTS.md` passou a orientar, logo apos o startup obrigatorio, a leitura do indice apenas como mapa seletivo, com abertura sob demanda dos contratos relevantes para cada tarefa.
- `D:\Repos\TOOLS.md` ganhou uma secao `Descoberta rapida` apontando para o indice, reduzindo busca manual por docs e pastas operacionais.
- Os repositorios principais `ConveniosWebBNB`, `ConveniosWebBNB_Novo`, `ConveniosWebExterno`, `ConveniosWebVSAzure_Default`, `ConveniosWebData` e `SuperPowers` passaram a espelhar uma secao curta `Navegacao minima` em seus `AGENTS.md`, apontando para `D:\Repos\AGENTS.md`, `D:\Repos\STANDARDS-INDEX.md`, o contrato local do repo e, quando aplicavel, `docs/agent-workspace/PROJECT.md` e `openspec/`.
- O contrato adotado e deliberadamente minimo: o indice navega, mas a fonte de verdade continua sendo os arquivos canonicos e o runtime existente (`AGENTS.md`, `SOUL.md`, `TOOLS.md`, `HEARTBEAT.md`, `MEMORY.md` e `.openclaw/`).
- Validacao observada em 2026-04-11: os arquivos `D:\Repos\AGENTS.md`, `D:\Repos\STANDARDS-INDEX.md` e `D:\Repos\TOOLS.md` ficaram conectados entre si, e os seis `AGENTS.md` dos repositorios principais passaram a conter a secao `Navegacao minima`.

### 2026-04-08 | workspace | Rollout repo-local de OpenSpec nos repositorios principais do workspace
- O workspace passou a tratar OpenSpec como capacidade padronizada por repositorio, sem transformar todo o ambiente em `OpenSpec-first`.
- O contrato atual ficou assim:
  - `codex-lb` permanece como excecao `OpenSpec-first` e SSOT principal.
  - `ConveniosWebBNB_Novo` preserva o piloto repo-local existente.
  - `ConveniosWebVSAzure_Default` preserva o piloto repo-local existente e ganhou alinhamento explicito no `AGENTS.md`.
  - `.openclaw`, `ChamadoWebAPI`, `ChamadoWebExterno`, `ConveniosWebBNB`, `ConveniosWebData`, `ConveniosWebExterno` e `SuperPowers` ganharam scaffold minimo repo-local em `openspec/` com `config.yaml`, `README.md` e `changes/README.md`.
- Todos os repositorios principais do workspace passaram a ter orientacao explicita no `AGENTS.md` sobre quando usar OpenSpec e quando nao usar, preservando `AGENTS.md` e a validacao real do repo como contrato principal.
- O baseline adotado para os pilotos foi deliberadamente leve:
  - `spec.md` permanece normativo e testavel
  - contexto livre fica em artefatos complementares do proprio OpenSpec
  - OpenSpec deve ser usado para comportamento, contrato, integracao, compatibilidade, migracao ou risco de rollout, e nao para bug pequeno ou tweak operacional curto
  - o scaffold e repo-local e nao cria dependencia do runtime global do Codex/OpenClaw
- Validacao observada em 2026-04-08: todos os repositorios-alvo responderam com `openspec/config.yaml` presente, e todos os `AGENTS.md` do conjunto principal passaram a conter `Piloto OpenSpec` ou `Workflow (OpenSpec-first)`.

### 2026-04-08 | workspace | Integracao OpenSpec com Codex fica global em ~/.codex/prompts no setup local
- O CLI oficial `openspec` foi instalado com `npm install -g @fission-ai/openspec@latest`, ficando disponivel no PATH como `C:\Users\l.sousa\AppData\Roaming\npm\openspec.ps1` com versao `1.2.0`.
- A tentativa de usar `openspec update <repo>` como rollout repo-a-repo nao funcionou neste setup: mesmo apos o scaffold local, o comando continuou respondendo `No configured tools found`.
- A integracao util observada para Codex veio de `openspec init <repo> --tools codex`, que materializou prompts globais em `C:\Users\l.sousa\.codex\prompts\`:
  - `opsx-propose.md`
  - `opsx-explore.md`
  - `opsx-apply.md`
  - `opsx-archive.md`
- O contrato operacional duravel passou a ser:
  - repositorios mantem `openspec/` repo-local como SSOT de specs e changes
  - o runtime do Codex consome comandos OpenSpec via prompts globais `opsx-*`
  - `openspec update` nao deve ser tratado como evidência de configuracao do Codex neste ambiente sem nova verificacao

### 2026-04-08 | openclaw | Primeiro change OpenSpec real do workspace foi validado e arquivado em .openclaw
- Foi criado em `.openclaw` o change `codify-repo-local-openspec-rollout` para formalizar o contrato do rollout repo-local de OpenSpec e a interpretacao correta da integracao com Codex no setup local.
- O change recebeu `proposal.md`, `design.md`, `tasks.md` e `specs/workspace-openspec-governance/spec.md`, foi validado com `openspec change validate codify-repo-local-openspec-rollout` e depois arquivado com `openspec archive codify-repo-local-openspec-rollout -y`.
- A capability promovida para o estado estavel ficou em `D:\Repos\.openclaw\openspec\specs\workspace-openspec-governance\spec.md`.
- Validacao final observada: `openspec validate --specs --no-interactive` retornou `1 passed, 0 failed`.
- Esse artefato virou a referencia concreta do workspace para futuros changes OpenSpec nos demais repositorios.

### 2026-04-08 | openclaw | Patch local no OpenSpec corrigiu deteccao de Codex em `openspec update`
- A causa raiz do falso negativo em `openspec update <repo>` foi provada no proprio pacote instalado do OpenSpec `1.2.0`.
- Em `C:\Users\l.sousa\AppData\Roaming\npm\node_modules\@fission-ai\openspec\dist\core\profile-sync-drift.js`, `getCommandConfiguredTools()` exigia a existencia de `project/<skillsDir>` para considerar um tool com comandos configurados.
- Isso conflita com o adapter oficial do Codex em `dist/core/command-generation/adapters/codex.js`, que grava prompts em caminho global absoluto `C:\Users\l.sousa\.codex\prompts\opsx-*.md`, e nao em `project/.codex/...`.
- A correcao local aplicada removeu a exigencia de diretorio repo-local e passou a delegar a deteccao para `toolHasAnyConfiguredCommand()`.
- Evidencia verificada apos o patch:
  - `openspec update D:\Repos\ChamadoWebAPI` passou a retornar `All 1 tool(s) up to date (v1.2.0)` com `Tools: codex`
  - `openspec update D:\Repos\.openclaw` passou a retornar `All 1 tool(s) up to date (v1.2.0)` com `Tools: codex`
  - `openspec update D:\Repos\ConveniosWebBNB_Novo` passou a retornar `All 1 tool(s) up to date (v1.2.0)` com `Tools: codex`
  - `openspec update --force D:\Repos\ChamadoWebAPI` conseguiu regravar os prompts globais do Codex sem erro
- Limitacao duravel: o fix vive no `node_modules` global do OpenSpec; uma reinstalacao ou upgrade do pacote pode sobrescreve-lo, entao essa divergencia precisa ser reaplicada ou upstreamed caso volte a ocorrer.

### 2026-04-03 | openclaw | Piloto de selecao de contexto por task antes do worker
- O OpenClaw passou a ter um piloto local de selecao de contexto por task, inspirado conceitualmente em retrieval/compilacao de contexto, mas implementado de forma nativa e simples em `D:\Repositórios\.openclaw\scripts\OpenClaw.TaskContext.psm1` e `D:\Repositórios\.openclaw\scripts\Resolve-OpenClawTaskContext.ps1`.
- O modulo indexa blocos de `AGENTS.md`, `SOUL.md`, `USER.md`, `MEMORY.md`, memorias diarias recentes, `AGENTS.md` do repo alvo e artefatos locais da task, grava o snapshot em `D:\Repos\.openclaw\runtime\swarm\state\context-index.json` e produz pacotes por task em `D:\Repos\.openclaw\runtime\swarm\state\context-packages\`.
- `D:\Repositórios\.openclaw\runtime\swarm\Scripts\Run-SwarmWorker.ps1` agora resolve esse pacote antes de montar o prompt do worker e persiste no resultado da task os campos `selectedContextPackage`, `selectedContextPackageJsonPath` e `selectedContextPackageMarkdownPath`.
- O budget atual e fixo por perfil: `fast=4 blocos/7000 chars`, `medium=7/12000`, `high=10/18000`, com prioridade para um bloco representativo de cada fonte obrigatoria (`AGENTS` do workspace, `SOUL`, `AGENTS` do repo) e uso opportunistico de artefatos locais como `manual-intake`.
- Validacao observada em 2026-04-03: o smoke test real com `cwbnn-009` gerou pacote `medium` com contexto distribuido entre `AGENTS.md`, `SOUL.md`, `ConveniosWebBNB_Novo\AGENTS.md` e `Relatorios\Swarm\manual-intake\cwbnn-009.md`, evitando concentrar todo o budget num unico arquivo.
- Na evolucao do mesmo dia, o contrato passou a viver em `D:\Repositórios\.openclaw\runtime\swarm\swarm-config.json` no bloco `contextSelection`, com budgets configuraveis por perfil (`fast`, `medium`, `high`) e leitura nativa pelo modulo de contexto.
- O runtime passou a expor esse estado em `D:\Repositórios\Relatorios\OpenClaw\openclaw-runtime.json` no bloco `contextSelection`, incluindo indice, quantidade de pacotes e metadados do ultimo pacote observado.
- O dashboard local passou a mostrar `Contexto por task` em `Runtime > OpenClaw` e `Contexto` em `Config`, usando `D:\Repositórios\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` e `D:\Repositórios\.openclaw\scripts\openclaw-dashboard.template.html`.
- O supervisor do swarm passou a agregar essa telemetria em `D:\Repositórios\Swarm\supervisor-status.md`, com contadores de tasks `com_pacote`, `sem_pacote`, `acima_budget` e `artefatos` de contexto em disco.
- O `D:\Repositórios\.openclaw\scripts\Refresh-OpenClawDashboard.ps1` foi endurecido para aceitar tanto a raiz do workspace quanto a pasta `.openclaw` como entrada, corrigindo o drift que gerava caminho `\.openclaw\.openclaw\...` quando chamado pelo supervisor.
- O `D:\Repositórios\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1` passou a calcular tambem recomendacoes de tuning de `contextSelection` por perfil de execucao, com heuristica simples baseada em incidencia de `sem_pacote` e `acima_budget`; por ora, essas recomendacoes sao apenas publicadas no status e nao alteram o config automaticamente.
- O fluxo atual de tuning e deliberadamente manual: quando houver recomendacao suficiente no supervisor, o workspace pode materializar um artefato de proposta em `D:\Repositórios\Relatorios\OpenClaw\context-selection-budget-recommendation-YYYY-MM-DD.md` e um patch companheiro `.patch`, sem aplicar mudanca automatica no `swarm-config.json`.
- Em 2026-04-03, a primeira calibracao manual aplicada no contrato `contextSelection` elevou os budgets para `fast=5 blocos/9000 chars`, `medium=8/14000` e `high=11/20000`, com regeneracao subsequente do `openclaw-runtime.json` e do dashboard local.

### 2026-04-02 | swarm | Supressao explicita para nao reexecutar tasks arquivadas pelo usuario
- O runtime do swarm passou a respeitar supressoes explicitas definidas em `D:\Repos\.openclaw\runtime\swarm\state\task-suppressions.json` via `Get-SwarmTaskSuppressionRules` e `Find-SwarmTaskSuppressionMatches` em `D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1`, consumidas por `D:\Repos\.openclaw\runtime\swarm\Scripts\Generate-SwarmTaskBacklog.ps1` antes de preservar ou recriar tasks.
- A decisao operacional atual bloqueia nova execucao das tasks `cwbnb-004` e `cwbnb-013` ligadas a `BD.aspx`, da task `cwbnb-012` ligada a alteracoes em `Web.config` / `WSConvenio\Web.Producao.config`, da task `cwazu-023` ligada ao segredo exposto em `Externo\Web.PMVV.config` e da task `supow-002` ligada ao segredo exposto em `App.config`.
- Em 2026-04-02 essas tasks foram removidas do backlog vivo e reanexadas a `D:\Repos\.openclaw\runtime\swarm\state\removed-tasks.json` com razao `manual_archive_user_no_rerun_2026-04-02`.
- Tags: swarm, backlog, suppressions, governance
- Fontes: D:\Repos\.openclaw\runtime\swarm\state\task-suppressions.json; D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Generate-SwarmTaskBacklog.ps1; D:\Repos\.openclaw\runtime\swarm\state\removed-tasks.json

### 2026-03-22 | openclaw | Contrato local de contexto e memoria
<!-- curated-memory:9a54ed439211b9630844932bb5f9e33874d53ec22f67e449d48abe47c235cdb1 -->
- O stack local do workspace usa provider registry, plugins versionados e persistencia por novidade para contexto e memoria curada.
- Providers e perfis ficam em D:\Repos\.openclaw\scripts\config\local-ai.providers.json.
- Plugins de captura local ficam em D:\Repos\.openclaw\plugins\context-capture\ e o task-context deduplica por conteudo extraido.
- Memoria curada duravel agora entra por Write-WorkspaceCuratedMemory.ps1 e reaproveita fingerprint identico.
- Tags: local-ai, memory, openclaw
- Fontes: D:\Repos\.openclaw\scripts\OpenClaw.ContextPlugins.psm1; D:\Repos\.openclaw\scripts\OpenClaw.LocalAi.psm1; D:\Repos\.openclaw\scripts\OpenClaw.Memory.psm1; D:\Repos\.openclaw\scripts\OpenClaw.Novelty.psm1

### 2026-03-25 | openclaw | Observabilidade de sessoes Codex
- O OpenClaw ganhou observabilidade nativa de sessoes Codex via D:\Repos\.openclaw\scripts\Invoke-CodexSessionObservability.ps1.
- A configuracao canonica vive em D:\Repos\.openclaw\runtime\swarm\swarm-config.json no bloco codexSessionObservability.
- Os artefatos oficiais ficam em D:\Repos\Relatorios\Swarm\codex-session-observability.md e D:\Repos\Relatorios\Swarm\codex-session-observability.json.
- O supervisor regenera esse resumo a cada rodada e o doctor valida frescor mais probe live read-only das sessoes.
- O artefato resume sessoes, repositorios, ferramentas, falhas de tool e acessos a caminhos sensiveis detectados por padrao.
- Tags: codex, observability, swarm, openclaw
- Fontes: D:\Repos\.openclaw\scripts\Invoke-CodexSessionObservability.ps1; D:\Repos\.openclaw\runtime\swarm\swarm-config.json; D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDoctor.ps1
- SensitiveAlerts agora considera apenas segredos de maior sinal (.env, secrets.local.json, auth.json, chaves SSH), enquanto web.config, appsettings e config.toml entram como acesso de configuracao e nao disparam warning sozinhos.
- Hits do proprio workspace operacional (D:\Repos, .openclaw, Relatorios, Swarm, memory) sao contabilizados em SuppressedSensitiveHits e nao entram no alerta principal.

### 2026-03-25 | openclaw | Starter pack sanitizado para novos times
- Existe um pack reutilizavel em D:\Repos\Relatorios\OpenClaw-Starter-Pack para bootstrap de um fluxo semelhante ao OpenClaw em outros repositorios e equipes.
- O pack inclui manual do zero, checklist, templates de workspace, templates de .openclaw, template de AGENTS.md por repo e script de scaffolding inicial.
- O artefato distribuivel local fica em D:\Repos\Relatorios\OpenClaw-Starter-Pack.zip.
- O pack foi mantido sem segredos, contas ou caminhos pessoais do ambiente original; tudo que depende de ambiente entra por placeholder.
- O starter pack sanitizado tambem possui uma versao v2 com roteiro por maturidade, templates de scheduler e geracao de runtime por placeholder, sem levar binds ou segredos do ambiente original.
- 2026-03-25: o starter pack do OpenClaw passou a incluir `examples/AcmeOrders-Workspace` como referencia ponta a ponta para implantacao em times iniciantes.
- Tags: openclaw, starter-pack, bootstrap, templates
- Fontes: D:\Repos\Relatorios\OpenClaw-Starter-Pack; D:\Repos\Relatorios\OpenClaw-Starter-Pack.zip

### 2026-03-25 | chamadoweb | Bridge inicial entre chamados e swarm
- O fluxo de integracao ChamadoWeb -> swarm passou a ter base canonica em `D:\Repos\.openclaw\scripts\OpenClaw.ChamadoWeb.ps1`, `New-ChamadoWebCommand.ps1` e `Sync-ChamadoWebSwarm.ps1`.
- `D:\Repos\.openclaw\runtime\swarm\chamadoweb-automation.json` define API/base de sistema, roteamento para repo e statuses do fluxo automatico; credenciais devem vir de `D:\Repos\.openclaw\runtime\secrets.local.json` em `main.chamadoWeb` ou por parametro.
- `D:\Repos\ChamadoWebAPI\ChamadoWebAPI\Controllers\CHTarefaChamadoController.cs` expoe o vinculo `CHTarefaChamado` para registrar task externa por chamado e marcar conclusao de retorno.
- Tags: chamadoweb, swarm, automacao, openclaw
- Fontes: D:\Repos\ChamadoWebAPI\ChamadoWebAPI\Controllers\CHTarefaChamadoController.cs; D:\Repos\ChamadoWebAPI\ChamadoWebAPI\Filtros\FCHTarefaChamado.cs; D:\Repos\ChamadoWebAPI\ChamadoWebAPI\ModelViews\VCHTarefaChamado.cs; D:\Repos\.openclaw\scripts\OpenClaw.ChamadoWeb.ps1; D:\Repos\.openclaw\scripts\New-ChamadoWebCommand.ps1; D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1; D:\Repos\.openclaw\runtime\swarm\chamadoweb-automation.json
- 2026-03-25: a base de producao canonica do ChamadoWebAPI para o bridge local ficou definida como `https://chamadowebapi.azurewebsites.net` em `D:\Repos\.openclaw\runtime\swarm\chamadoweb-automation.json`.
- ChamadoWebAPI producao usa https://chamadowebapi.azurewebsites.net com identificador de sistema ChamadoWebExterno para o frontend externo e para o bridge OpenClaw.
- O helper D:\Repos\.openclaw\scripts\OpenClaw.ChamadoWeb.ps1 precisa tratar payloads da API em camelCase e achatar colecoes dos endpoints Sistema, CHCliente/cliente-completo, CHProduto/produto-completo, CHAssunto/assunto-completo e CHPessoa; sem isso a resolucao automatica por nome falha.
- O sync D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1 precisa achatar as leituras de CHTarefaChamado/por-chamado/{id} e CHTarefaChamado?ids=... para funcionar com o payload real da API em producao.
- Com a configuracao atual, o sync em producao sobe com ChamadoWebExterno, mas chamados de produtoNome=ConveniosWeb ainda exigem regra explicita em `repoRouting` antes de virarem tasks automaticamente.
- Chamados produtoNome=ConveniosWeb devem ser roteados pelo bridge ChamadoWeb para ConveniosWebBNB_Novo, alinhado com a preferencia de backend novo do workspace.
- Em 2026-03-25 o sync real converteu o chamado 377/2026 (id 37696) na task swarm cwbnn-005; isso valida o fluxo ChamadoWeb -> CHTarefaChamado -> backlog swarm em producao.

- 2026-03-25: o sync `D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1` ganhou modo de auxilio para casos nao auto-resoluveis; ele consolida descricao, acompanhamentos e anexos do chamado, classifica sinais como producao/relatorio/banco/sem-roteamento, grava artefato de contexto em `D:\Repos\Relatorios\Swarm\manual-intake\chamadoweb-<numero>-<repo>.md` e pode adicionar acompanhamento `[OPENCLAW-AUXILIO]` com orientacoes e script base sem encerrar automaticamente o chamado.
- `D:\Repos\.openclaw\scripts\OpenClaw.ChamadoWeb.ps1` passou a encaminhar `ContextArtifactPath` e `ContextSummary` para o `Add-SwarmTask.ps1`, permitindo que a task do swarm nasca com o contexto consolidado do chamado.
- `D:\Repos\.openclaw\runtime\swarm\chamadoweb-automation.json` agora tem o bloco `assistMode` como contrato configuravel para marker, previews textuais e palavras-chave do modo de auxilio.

### 2026-03-25 | swarm | Watchdog de tasks ativas do swarm
- Em `25/03/2026`, o supervisor do swarm passou a tratar `processo vivo porem travado` como falha operacional valida.
- A logica fica em `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1` e usa `activeTaskWatchdog` de `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` com defaults `maxRuntimeMinutes=120`, `maxIdleMinutes=20` e `missingLogGraceMinutes=10`.
- Quando excede o limiar sem log/resultado, o supervisor mata o PID, move a tentativa para historico e devolve a task para `ready` sem apagar `branch` ou `worktreePath`, permitindo retomar trabalho parcial no mesmo worktree.
- Tags: swarm, watchdog, supervisor, recovery
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1; D:\Repos\.openclaw\runtime\swarm\swarm-config.json

### 2026-03-25 | swarm | Preservacao de tasks manuais ready no backlog
- Em `25/03/2026`, `D:\Repos\.openclaw\runtime\swarm\Scripts\Generate-SwarmTaskBacklog.ps1` passou a preservar tambem tasks manuais com status `ready` (alem de `in_progress`, `review_pending`, `awaiting_review`, `blocked` e `failed`).
- A deteccao usa `sourceKind=manual` ou `sourceRef/sourceReport` com prefixo `manual://`.
- Isso evita que intake manual do ChamadoWeb desapareca em regeneracoes do backlog antes da execucao.
- Tags: swarm, backlog, manual-intake, preservation
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Generate-SwarmTaskBacklog.ps1

### 2026-03-25 | chamadoweb | Chamados assistidos nao devem virar task de codigo
- Em `25/03/2026`, `D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1` passou a tratar `assistProfile.requiresAssistance = true` como fluxo `assist-only`: gerar artefato/contexto, publicar acompanhamento com proximos passos e script base, e evitar criacao de task de alteracao de codigo no swarm.
- Se existir vinculo aberto de task anterior em caso assistido, o sync pode concluir o link no ChamadoWeb para reclassificar o caso como apoio manual.
- Tags: chamadoweb, assist-mode, intake, governance
- Fontes: D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1; D:\Repos\.openclaw\runtime\swarm\chamadoweb-automation.json

### 2026-03-25 | chamadoweb | Auxilio schema-aware para chamados ConveniosWeb
- Em `25/03/2026`, `D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1` passou a enriquecer chamados assistidos do ConveniosWeb com referencias locais de schema/codigo antes de montar scripts de apoio.
- O fluxo passou a reconhecer tabelas-base do banco produtivo em `D:\Repos\ConveniosWebData\dbo\Tables` como `cvConvenio`, `cvPrestacaoContas`, `cvExtrato`, `cvConciliacaoExtrato` e `cvConciliacaoBancaria`, alem do comportamento de exclusao logica em `D:\Repos\ConveniosWebVSAzure_Default\Classes\ConveniosWeb.Classes.cs` e da tela `D:\Repos\ConveniosWebVSAzure_Default\ConciliacaoBancaria.Cadastro.aspx.cs`.
- Para o chamado `381/2026` (`id 37700`), isso resultou em um artefato operacional com scripts revisaveis de diagnostico e de desvinculo controlado usando `Ativo = 0` em vez de delete fisico, em `D:\Repos\Relatorios\Swarm\manual-intake\chamadoweb-37700-convenioswebvsazure_default.md`.
- 2026-03-25: o bridge do ChamadoWeb passou a tratar o intake em duas fases operacionais explicitas. Ao iniciar a triagem, `D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1` move o chamado para `Em Analise` (`situacao 90`); ao decidir entre automacao, apoio tecnico ou atendimento manual, move para `Em Atendimento` (`situacao 65`) com `PrevisaoSolucao` padrao configuravel em `defaultForecastHours` no `D:\Repos\.openclaw\runtime\swarm\chamadoweb-automation.json`. Os detalhes tecnicos deixaram de ir no texto principal do acompanhamento e passaram para `DescricaoTecnica`, preservando `Descricao` curta para notificacao operacional.
- 2026-03-28: a abertura de chamados por email passou a aplicar o mesmo principio tambem no registro inicial. `D:\Repos\.openclaw\scripts\Invoke-OutlookFolderChamadoIntake.ps1` agora monta `Problema` em linguagem publica mais formal e sem expor automacao, enquanto o detalhe tecnico do intake entra em `DescricaoTecnica` do acompanhamento inicial via `D:\Repos\.openclaw\scripts\New-ChamadoWebCommand.ps1`.
- 2026-03-26: os acompanhamentos automaticos publicados pelo bridge do ChamadoWeb deixaram de disparar e-mail. O helper `New-ChamadoAcompanhamentoUpdate` em `D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1` agora envia `CHAcompanhamentos[].EnviaEmail = false`, aproveitando o campo nativo `VCHAcompanhamento.EnviaEmail` da API (`D:\Repos\ChamadoWebAPI\ChamadoWebAPI\ModelViews\VCHAcompanhamento.cs`).
- 2026-03-26: o OpenClaw passou a consumir email por mailbox dedicada do Outlook, implementado em `D:\Repos\.openclaw\scripts\Invoke-OutlookFolderEmailIntake.ps1` e `D:\Repos\.openclaw\scripts\Run-EmailDropIntake.ps1`. O contrato atual observa apenas a pasta `OpenClaw/Novo` da store `lfasousa@outlook.com.br`, trata toda mensagem recebida nessa pasta como chamado, infere repo/prioridade pelo contexto, move o item no Outlook para `OpenClaw/Processed` ou `OpenClaw/Failed`, gera artefato de contexto em `D:\Repos\Relatorios\Swarm\manual-intake\email-source` e cria task manual no backlog via `Add-SwarmTask.ps1`, com fallback de repo para `ConveniosWebBNB_Novo`.
- 2026-03-26: neste host, a persistencia do intake de email ficou por launcher `D:\Repos\.openclaw\scripts\Start-OpenClawEmailIntake.vbs` instalado no `Startup` do usuario, porque a criacao/registro da task `OpenClaw Email Intake` no Windows Scheduler continuou retornando `Acesso negado`.
- Tags: chamadoweb, schema-aware, conveniosweb, assist-mode
- Fontes: D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1; D:\Repos\ConveniosWebData\dbo\Tables; D:\Repos\ConveniosWebVSAzure_Default\Classes\ConveniosWeb.Classes.cs; D:\Repos\ConveniosWebVSAzure_Default\ConciliacaoBancaria.Cadastro.aspx.cs; D:\Repos\Relatorios\Swarm\manual-intake\chamadoweb-37700-convenioswebvsazure_default.md
### 2026-03-26 | openclaw | Auditorias diretas guiadas para estabilidade em repos grandes
- As auditorias diretas do swarm passaram a usar um prompt canonicamente guiado e limitado por amostragem objetiva em `D:\Repos\.openclaw\prompts\autonomous-code-audit.md`, em vez de incentivar mapeamento amplo do repositorio.
- O runner `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-RepoAuditDirect.ps1` agora resolve o modelo a partir do `codexModelRouting.defaultExecutionProfile` de `D:\Repos\.openclaw\runtime\swarm\swarm-config.json`, usando o profile `balanced` quando presente.
- Essa combinacao estabilizou as auditorias recorrentes de `ConveniosWebBNB` e `ConveniosWebBNB_Novo`, que voltaram a atualizar `D:\Repos\Relatorios\ConveniosWebBNB\autonomous-code-audit.md` e `D:\Repos\Relatorios\ConveniosWebBNB_Novo\autonomous-code-audit.md` dentro da janela operacional e com `LastTaskResult = 0` no Scheduler.
- Observacao operacional: disparos manuais simultaneos dessas duas auditorias podem induzir `lastResult = 1` em teste, mas o agendamento real e escalonado (`BNB` e `BNB_Novo` com 10 minutos de distancia) fechou limpo.
- Tags: openclaw, swarm, auditoria, scheduler, codex
- Fontes: D:\Repos\.openclaw\prompts\autonomous-code-audit.md; D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-RepoAuditDirect.ps1; D:\Repos\.openclaw\runtime\swarm\swarm-config.json
### 2026-03-26 | openclaw | Baseline canonico de hardening para autonomia
- O workspace passou a explicitar em `D:\Repos\SOUL.md` e `D:\Repos\.openclaw\OPENCLAW-OPERATING-CONTRACT.md` um baseline de hardening para reduzir autonomia perigosa em integracoes externas.
- O baseline fixa: modelos pesados por excecao, gateways em loopback por padrao, rollout minimo/read-only para integracoes novas, proibicao de criar contas ou conectar servicos novos sem aprovacao explicita, e bloqueio operacional a skills/plugins de origem nao verificada.
- O baseline tambem proibe enviar `.env`, `secrets.local.json`, `auth.json`, tokens ou dados pessoais para servicos externos sem aprovacao explicita e trata instrucoes vagas como insuficientes para acoes irreversiveis.
- Decisao de implementacao: nao gravar chaves de configuracao nao verificadas no `openclaw.json` local quando o schema nao estiver presente no stack real; nesses casos, endurecer contrato e operacao antes de mexer no runtime.
- Tags: openclaw, hardening, autonomia, seguranca
- Fontes: D:\Repos\SOUL.md; D:\Repos\.openclaw\OPENCLAW-OPERATING-CONTRACT.md
### 2026-03-26 | openclaw | Honestidade operacional baseada em verificacao
- O workspace passou a explicitar que tentativa nao equivale a conclusao: agentes so devem reportar sucesso quando houver verificacao observavel do efeito produzido.
- O `SOUL.md` agora proibe responder "feito" sem evidencia verificavel e exige que falhas de ferramenta, permissao, site, arquivo ou integracao sejam admitidas imediatamente.
- O `OPENCLAW-OPERATING-CONTRACT.md` passou a exigir evidencia minima verificavel para acoes externas, com relato de o que foi checado, como foi checado e identificadores concretos quando existirem.
- Para tarefas longas com efeitos externos, o baseline operacional passou a preferir checkpoints verificaveis e etapas menores em vez de um unico bloco opaco de execucao.
- Tags: openclaw, verificacao, honestidade, confiabilidade
- Fontes: D:\Repos\SOUL.md; D:\Repos\.openclaw\OPENCLAW-OPERATING-CONTRACT.md

### 2026-03-27 | openclaw | Fluxo de PR sem comentario automatico `@codex review`
- O supervisor do swarm deixou de solicitar review por comentario automatico em PR. `D:\Repos\.openclaw\runtime\swarm\Scripts\Run-SwarmPrReview.ps1` agora consulta apenas o estado do PR e o `reviewDecision` retornado pelo GitHub.
- A task continua transitando entre `review_pending`, `awaiting_review`, `completed` ou `blocked` conforme estado do PR, mas sem gravar novo `reviewRequestedAt` nem artefato epistemico de `gh_pr_comment`.
- O texto operacional do supervisor tambem deixou de usar a expressao `Review Codex`, e o gatilho local `codex-review-loop` foi removido de `D:\Repos\codex-lb\.agents\skills\skill-rules.json`.
- Tags: openclaw, swarm, pr, review
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Run-SwarmPrReview.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1; D:\Repos\codex-lb\.agents\skills\skill-rules.json

### 2026-03-27 | openclaw | Semantica de review do swarm separa entrega sem PR de trilha explicita
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1` passou a marcar sucesso de worker sem `prUrl` como `review_pending`, em vez de `awaiting_review`.
- O mesmo supervisor agora rebaixa automaticamente para `review_pending` qualquer task legada em `awaiting_review` que nao tenha `prUrl` nem PR resolvivel; isso evita fila "aguardando review" sem trilha concreta de revisao.
- `awaiting_review` fica reservado para casos com PR/trilha de revisao mais explicita, enquanto `review_pending` cobre entrega pronta para revisar, com ou sem PR aberto.
- Tags: openclaw, swarm, review, status
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1; D:\Repos\Relatorios\OpenClaw\guia-fluxo-openclaw-equipe.md

### 2026-03-27 | openclaw | Health do worker pool nao deve marcar erro de task como worker quebrado
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-CodexFallback.ps1` passou a distinguir `task_failure` de falha real do worker. Casos como `apply_patch verification failed`, `Failed to find expected lines` e `error=Exit code: 1` ficam sem cooldown e sem rotular o agente como indisponivel.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Get-SwarmStatus.ps1` deixou de tratar `error` e `semantic_error` como "falha de comando" do pool; esses estados agora aparecem como worker disponivel, preservando o destaque apenas para `semantic_failure`, `bootstrap_failure` e falhas de probe.
- Essa separacao evita falso positivo no status do swarm quando a ultima task falhou por contexto, diff ou aplicacao de patch, mas o `codex-lb` continua respondendo normalmente.
- Tags: openclaw, swarm, worker-pool, status, observability
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-CodexFallback.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Get-SwarmStatus.ps1

### 2026-03-27 | openclaw | Dashboard operacional com centro de comando para tasks e chamados
- `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html` ganhou o tab `Comandos`, com cards visuais para copiar atalhos do swarm e montar comandos parametrizados de task e ChamadoWeb direto no painel.
- O dashboard agora monta comandos reais sobre scripts dedicados: `D:\Repos\.openclaw\runtime\swarm\Scripts\Set-SwarmTaskState.ps1`, `D:\Repos\.openclaw\runtime\swarm\Scripts\Remove-SwarmTask.ps1` e `D:\Repos\.openclaw\scripts\Update-ChamadoWeb.ps1`, alem dos scripts ja existentes de add/retry/sync.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` passou a embutir metadados de `commandCenter` no JSON do painel e a reutilizar a semantica atual de health do worker pool, evitando marcar `error`/`semantic_error` como atencao de agente.
- O dashboard passou a executar essas acoes por um bridge local whitelisted em `http://127.0.0.1:18795/dashboard-bridge`, implementado por `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1` e `D:\Repos\.openclaw\scripts\Run-OpenClawDashboardBridge.ps1`. `D:\Repos\.openclaw\scripts\Start-OpenClawDashboard.ps1` agora sobe o bridge, valida `health` e abre o painel pela rota `.../dashboard`, evitando o bloqueio de browser que aparece ao depender de `file:///` para chamar `localhost`.
- O tab `Comandos` ganhou estado do bridge, console da ultima execucao e botoes `Executar agora` para quick actions, operacoes de task e formularios de ChamadoWeb. Falhas de negocio continuam aparecendo no painel, mas nao rebaixam mais o bridge para `offline`.
- As duplicatas tardias de `Resolve-ChamadoWebCliente`, `Resolve-ChamadoWebProduto` e `Resolve-ChamadoWebAssunto` em `D:\Repos\.openclaw\scripts\OpenClaw.ChamadoWeb.ps1` foram alinhadas para aceitar `ClienteNomes`, `ProdutoNomes` e `AssuntoNomes`; no formulario `Novo chamado`, o campo `Cliente` passou a avisar quando a sessao do ChamadoWeb nao consegue inferir um cliente padrao.
- O detalhe da task ganhou atalhos `Editar status`, `Retry` e `Arquivar`, que levam a task focada para o tab de comandos.
- Tags: openclaw, dashboard, ux, swarm, chamadoweb, bridge
- Fontes: D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1; D:\Repos\.openclaw\scripts\Start-OpenClawDashboard.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1; D:\Repos\.openclaw\scripts\Run-OpenClawDashboardBridge.ps1; D:\Repos\.openclaw\scripts\OpenClaw.ChamadoWeb.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Set-SwarmTaskState.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Remove-SwarmTask.ps1; D:\Repos\.openclaw\scripts\Update-ChamadoWeb.ps1

### 2026-03-26 | openclaw | SuperPowers integrado ao swarm local
- O repo `SuperPowers` passou a fazer parte do mapa canonico do swarm local, com clone em `D:\Repos\SuperPowers`, `repoCode = supow`, `agentId = repo-superpowers` e relatorio previsto em `D:\Repos\Relatorios\SuperPowers\autonomous-code-audit.md`.
- O allowlist operacional (`arkPolicy.allowedRepoPaths`) e a documentacao base do workspace foram atualizados para tratar `SuperPowers` como repositorio conhecido do fluxo.
- Foi reservado o job recorrente `\OpenClaw Audit SuperPowers Direct` com a mesma cadencia das auditorias diretas existentes.
- Tags: openclaw, swarm, repositorio, auditoria
- Fontes: D:\Repos\.openclaw\runtime\swarm\swarm-config.json; D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1; D:\Repos\AGENTS.md

### 2026-03-27 | openclaw | Prompt local de frontend para evitar UI generica de IA
- O workspace ganhou o prompt local `D:\Repos\.openclaw\prompts\frontend-no-ai-slop.md` para geracao e revisao de frontend.
- O prompt adapta a ideia do repo externo `Uncodixfy` sem instalar skill ou plugin de origem externa no fluxo principal.
- O contrato local preserva design system existente, rejeita glassmorphism, KPI fake, copy decorativa, dark SaaS generico, cantos exagerados e filler visual, mas evita o dogmatismo de reduzir tudo a uma UI "normal" sem personalidade.
- O inventario de `.openclaw` foi atualizado para expor esse prompt como artefato reaproveitavel do fluxo.
- Tags: openclaw, frontend, prompt, design
- Fontes: D:\Repos\.openclaw\prompts\frontend-no-ai-slop.md; D:\Repos\.openclaw\INVENTORY.md

### 2026-03-27 | openclaw | Reboot do bootstrap ficou observavel e menos suscetivel a falso negativo
- `D:\Repos\.openclaw\scripts\Start-OpenClawEnvironment.ps1` passou a subir e validar o `dashboard bridge` local no bootstrap, verificando `127.0.0.1:18795/dashboard-bridge/health` e incluindo esse estado no health gravado.
- O bootstrap tambem passou a distinguir componentes opcionais: o bloco `teamsOutgoingWebhook` foi desabilitado em `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` porque o segredo `main.teams.empresaOutgoingWebhookSecret` nao existe no host; assim o reboot deixa de acusar falha estrutural por um listener que nao pode autenticar.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Generate-SwarmTaskBacklog.ps1` passou a tolerar tasks preservadas com `summary` ou `implementationPrompt` vazio, evitando quebra do planner e efeito cascata no `daily-audit-summary` e no validador de artefatos.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDoctor.ps1` foi calibrado para refletir saude operacional atual: usa snapshot do pool de workers em vez de re-probe pesado, checa `dashboard bridge`/Teams explicitamente e nao rebaixa task agendada so por `lastResult = 1` historico quando o artefato canonicamente esperado esta fresco.
- `D:\Repos\.openclaw\scripts\Test-LocalAutomationCommands.ps1` passou a tratar o runtime local do Ollama como opcional para o doctor, evitando warning quando a stack principal do OpenClaw esta saudavel sem LLM local ativo.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Test-SwarmWorkerAvailability.ps1` ganhou timeout configuravel (`workerProbeTimeoutSeconds`, `workerProbeRetryCount`) para o probe do pool nao ficar travado no retorno pos-reboot.
- Resultado observado em 27/03/2026 apos os ajustes: `orq` (`18790`), `codex-lb` (`2455`) e `dashboard bridge` (`18795`) sobem no bootstrap; `Invoke-OpenClawDoctor.ps1 -Json` voltou a `overallStatus = ok`.


### 2026-03-28 | openclaw | Dashboard prioriza e arquiva falhas do intake de email
- D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html passou a dar destaque visual real para falhas do intake de email no overview, nas metricas e no radar de prioridades, alem de expor botoes Excluir do painel junto do fluxo de correcao.
- A exclusao no painel nao apaga o email do Outlook: o novo script D:\Repos\.openclaw\scripts\Archive-OutlookFailedChamado.ps1 move apenas o registro JSON de failed para failed\\archived, preservando trilha (archivedAt, archiveReason) e deixando o item do Outlook em OpenClaw/Failed.
- O bridge local D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1 ganhou a acao archiveEmailFailure, e D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1 passou a expor esse script no commandCenter e o archivedPath no payload do painel.
- Tags: openclaw, dashboard, email-intake, archive
- Fontes: D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html; D:\Repos\.openclaw\scripts\Archive-OutlookFailedChamado.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1

### 2026-03-28 | openclaw | Deduplicacao estrutural e retry direto para tasks originadas por email
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Add-SwarmTask.ps1` passou a aceitar metadados de origem (`SourceFingerprint`, `SourceMessageId`, `SourceSubject`, `SourceSender`) e a reutilizar a task existente quando o mesmo fingerprint de origem tentar entrar de novo no backlog.
- `D:\Repos\.openclaw\scripts\Invoke-EmailDropIntake.ps1` deixou de depender apenas de estado local/body text para dedupe; agora usa o fingerprint do email como chave de task, grava os metadados na task e respeita o retorno `created=false` do `Add-SwarmTask` para nao inserir duplicata tardia.
- O dashboard (`D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` + `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html`) passou a identificar `isEmailOriginTask`, destacar `Tasks email com problema` nas metricas/gargalos e oferecer `Retry agora` direto no detalhe e no card operacional dessas tasks.
- Validacao manual em config temporario confirmou o contrato: mesma `SourceFingerprint` cria uma unica task e a segunda chamada retorna `reused=true` com o mesmo `id`.
- Tags: openclaw, email-intake, swarm, dedupe, dashboard
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Add-SwarmTask.ps1; D:\Repos\.openclaw\scripts\Invoke-EmailDropIntake.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1; D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html

### 2026-03-28 | openclaw | Bridge do dashboard endurecido e wrappers de auditoria corrigidos
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1` deixou de aceitar CORS aberto e passou a exigir `application/json`, host local valido e o header `X-OpenClaw-Bridge-Request: 1` para `POST /dashboard-bridge/execute`.
- `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html` foi alinhado para enviar esse header nas acoes do painel, reforcando o contrato same-origin do dashboard servido pelo proprio bridge.
- `D:\Repos\.openclaw\scripts\Archive-OutlookFailedChamado.ps1` e `Retry-OutlookFailedChamado.ps1` passaram a validar `ResultPath` por caminho canonico dentro de `emailIntake.failedPath` e extensao `.json`, evitando operar em arquivos arbitrarios por prefixo ou parametro malformado.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Run-Audit-ConveniosWebBNB.cmd`, `Run-Audit-ConveniosWebBNB_Novo.cmd`, `Run-Audit-ConveniosWebData.cmd`, `Run-Audit-ConveniosWebExterno.cmd` e `Run-Audit-ConveniosWebVSAzure_Default.cmd` foram corrigidos para batch real, sem o literal quebrado `` `r`n ``.
- `D:\Repos\.openclaw\runtime\codex\config.template.toml` deixou de confiar o `C:\Users\l.sousa` inteiro e passou a chamar `D:\Repos\.openclaw\scripts\launch_mssql_mcp.ps1`, um wrapper local que resolve o launcher MSSQL MCP pelo ambiente (`USERPROFILE`/`CODEX_HOME`) em vez de depender de caminho pessoal fixo.
- `D:\Repos\.openclaw\.gitignore` passou a ignorar `runtime/tmp/`, e `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` passou a explicitar `knowledgeRoot` em vez de depender de default implicito.
- Tags: openclaw, dashboard, bridge, hardening, wrappers, codex-config
- Fontes: D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1; D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html; D:\Repos\.openclaw\scripts\Archive-OutlookFailedChamado.ps1; D:\Repos\.openclaw\scripts\Retry-OutlookFailedChamado.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Run-Audit-ConveniosWebBNB.cmd; D:\Repos\.openclaw\runtime\swarm\Scripts\Run-Audit-ConveniosWebBNB_Novo.cmd; D:\Repos\.openclaw\runtime\swarm\Scripts\Run-Audit-ConveniosWebData.cmd; D:\Repos\.openclaw\runtime\swarm\Scripts\Run-Audit-ConveniosWebExterno.cmd; D:\Repos\.openclaw\runtime\swarm\Scripts\Run-Audit-ConveniosWebVSAzure_Default.cmd; D:\Repos\.openclaw\runtime\codex\config.template.toml; D:\Repos\.openclaw\scripts\launch_mssql_mcp.ps1; D:\Repos\.openclaw\runtime\swarm\swarm-config.json; D:\Repos\.openclaw\.gitignore

### 2026-03-28 | openclaw | Governanca de execucao por lanes fast medium high
- O swarm passou a resolver perfis de execucao por lanes canonicas `fast`, `medium` e `high`, com compatibilidade retroativa para aliases legados `economy`, `balanced` e `premium` em `D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1`.
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` agora usa `defaultExecutionProfile = medium`; os perfis atuais sao `fast -> gpt-5.3-codex-spark`, `medium -> gpt-5.3-codex` e `high -> gpt-5.3-codex`, enquanto planejamento e review permanecem em `gpt-5.4`.
- Tasks do swarm passaram a persistir e exibir `executionProfileLabel`, `reasoningEffort` e `serviceTier`; o worker respeita a lane resolvida por padrao e so preserva modelo/parametro antigo quando houver override explicito.
- Dashboard e backlog markdown passaram a mostrar lane, modelo, reasoning e service tier de cada task.
- Fontes: D:\Repos\.openclaw\runtime\swarm\swarm-config.json; D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Add-SwarmTask.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Generate-SwarmTaskBacklog.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Run-SwarmWorker.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-CodexFallback.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-RepoAuditDirect.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1; D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html

### 2026-03-28 | openclaw | Cleanup estrutural de helpers legados e caminhos do swarm
- `D:\Repos\.openclaw\scripts\Archive-OutlookFailedChamado.ps1` e `Retry-OutlookFailedChamado.ps1` pararam de raspar helpers por substring + `Invoke-Expression`; ambos agora dot-sourceiam `Invoke-OutlookFolderChamadoIntake.ps1 -LoadHelpersOnly`, e esse script ganhou o switch para expor helpers sem disparar o intake principal.
- `D:\Repos\.openclaw\scripts\Invoke-EmailDropIntake.ps1` foi reduzido a shim de compatibilidade para `Invoke-OutlookFolderEmailIntake.ps1`, removendo uma copia grande e divergente do fluxo ativo.
- Scripts centrais do swarm passaram a resolver o `swarm-config.json` local do runtime quando chamados sem `ConfigPath` explicito ou com o caminho legado do home, reduzindo dependencia da ponte `C:\Users\l.sousa\.openclaw\swarm\swarm-config.json`.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Review-SwarmBestPractices.ps1` e `Send-SwarmStatusNotification.ps1` passaram a preferir artefatos canonicos do runtime local ao consultar `openclaw.json`/estado, mantendo fallback apenas como compatibilidade.
- O teste temporario `runtime\tmp\email-dedup-test-20260328110702` e os backups `scripts\Sync-ChamadoWebSwarm.ps1.bak*` foram movidos para `D:\Repos\.openclaw\archive\2026-03-28`, limpando a area operacional sem perda de historico.
- Fontes: D:\Repos\.openclaw\scripts\Archive-OutlookFailedChamado.ps1; D:\Repos\.openclaw\scripts\Retry-OutlookFailedChamado.ps1; D:\Repos\.openclaw\scripts\Invoke-OutlookFolderChamadoIntake.ps1; D:\Repos\.openclaw\scripts\Invoke-EmailDropIntake.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Get-SwarmStatus.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Review-SwarmBestPractices.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Send-SwarmStatusNotification.ps1; D:\Repos\.openclaw\archive\2026-03-28

### 2026-03-28 | openclaw | Swarm canonizado dentro do repo com pontes legadas para compatibilidade
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` passou a apontar `stateRoot` e `logsRoot` para dentro de `D:\Repos\.openclaw\runtime\swarm`, e `knowledgeRoot` para `D:\Repos`.
- O estado vivo do swarm foi migrado para `D:\Repos\.openclaw\runtime\swarm\state`, preservando `active-tasks.json`, `task-history.json`, `codex-worker-pool.json`, `openclaw-agent-pool.json`, `learning-review-state.json`, `epistemic-checks`, `queue` e `results`.
- Os logs vivos passaram a nascer em `D:\Repos\.openclaw\runtime\swarm\logs`; o restart local reabriu `codex-lb.pid`, `stdout.log` e `stderr.log` no destino novo.
- Para nao quebrar consumidores antigos, `C:\Users\l.sousa\.openclaw\swarm\state` e `...\logs` viraram symlinks para os destinos canonicos; `C:\Users\l.sousa\.openclaw\workspace\.learnings` e `...\workspace\memory\ontology` passaram a apontar para `D:\Repos\.learnings` e `D:\Repos\memory\ontology`.
- `Restore-OpenClawLinks.ps1`, `Swarm-Common.ps1`, `Invoke-OpenClawDoctor.ps1` e `Start-OpenClawEnvironment.ps1` foram alinhados com esse contrato para evitar recaida em paths antigos.
- Validacao operacional apos migracao: `Get-SwarmStatus.ps1` leu `poolStatePath = D:\Repos\.openclaw\runtime\swarm\state\codex-worker-pool.json`, e os listeners do `codex-lb` (`2455`), gateway `orq` (`18790`) e dashboard bridge (`18795`) estavam ativos. O doctor continuou acusando erro apenas por auditorias desatualizadas, nao por falha estrutural da migracao.
- Fontes: D:\Repos\.openclaw\runtime\swarm\swarm-config.json; D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1; D:\Repos\.openclaw\scripts\Restore-OpenClawLinks.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDoctor.ps1; D:\Repos\.openclaw\scripts\Start-OpenClawEnvironment.ps1; D:\Repos\.openclaw\runtime\swarm\state; D:\Repos\.openclaw\runtime\swarm\logs; D:\Repos\.openclaw\archive\2026-03-28\legacy-home-migration

### 2026-03-29 | openclaw | Esqueleto local para orquestracao estruturada via Vertex AI
- O workspace passou a ter um esqueleto opt-in para endurecer o intake do orquestrador com saida estruturada, sem mover a execucao de codigo para fora do Codex.
- O contrato atual vive em `D:\Repos\.openclaw\prompts\orchestrator-intake-v1.md`, `D:\Repos\.openclaw\schemas\orchestrator-decision.schema.json`, `D:\Repos\.openclaw\scripts\Invoke-VertexOrchestrator.ps1` e `D:\Repos\.openclaw\scripts\Test-VertexOrchestratorDecision.ps1`.
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` ganhou o bloco `vertexOrchestrator` com `enabled = false`, `decisionLogPath = D:\Repos\.openclaw\runtime\swarm\state\orchestrator-decisions.jsonl` e `evaluationDatasetPath = D:\Repos\Relatorios\OpenClaw\vertex-orchestrator-eval-dataset.jsonl`.
- O `-DryRun` do wrapper valida schema, escreve trilha de auditoria e serve como base para ligacao futura da chamada real ao Vertex; a politica atual continua sendo Vertex decide, OpenClaw valida e script local executa.
- Fontes: D:\Repos\.openclaw\prompts\orchestrator-intake-v1.md; D:\Repos\.openclaw\schemas\orchestrator-decision.schema.json; D:\Repos\.openclaw\scripts\Invoke-VertexOrchestrator.ps1; D:\Repos\.openclaw\scripts\Test-VertexOrchestratorDecision.ps1; D:\Repos\.openclaw\runtime\swarm\swarm-config.json; D:\Repos\Relatorios\OpenClaw\vertex-orchestrator-eval-dataset.jsonl
- 2026-03-29: o wrapper `D:\Repos\.openclaw\scripts\Invoke-VertexOrchestrator.ps1` deixou de ser apenas dry run e passou a conter caminho real de chamada REST ao Vertex AI, resolvendo `projectId`, `location` e token pelo ambiente/gcloud; a validacao local por schema continua obrigatoria, mas a confirmacao end-to-end da chamada live ainda depende de teste externo concluido.

### 2026-03-29 | openclaw | Executor Codex do swarm precisa resolver config pelo runtime pai
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-CodexWorker.ps1` nao pode resolver `swarm-config.json` nem `stateRoot` relativos ao proprio diretorio `Scripts`; o caminho correto eh o runtime pai `D:\Repos\.openclaw\runtime\swarm`.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Run-SwarmWorker.ps1` passou a encaminhar `-ConfigPath` explicitamente para o executor, evitando cair em defaults errados quando o worker e chamado pelo supervisor.
- Evidencia verificada em 2026-03-29: smoke local do executor retornou `PING`, atualizou apenas `D:\Repos\.openclaw\runtime\swarm\state\codex-worker-pool.json` e nao atualizou `runtime\swarm\Scripts\state\codex-worker-pool.json`.
- O estado espurio criado pela versao anterior foi arquivado em `D:\Repos\.openclaw\archive\2026-03-29\swarm-scripts-state`.
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-CodexWorker.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Run-SwarmWorker.ps1; D:\Repos\.openclaw\runtime\swarm\state\codex-worker-pool.json; D:\Repos\.openclaw\archive\2026-03-29\swarm-scripts-state

### 2026-03-29 | openclaw | Vertex orquestrador validado em chamada real com schema solto + validacao local
- D:\Repos\.openclaw\scripts\Invoke-VertexOrchestrator.ps1 foi validado em chamada real ao Vertex AI no host local.
- O contrato live final nao usa systemInstruction: o prompt canonico e a entrada do intake seguem juntos em uma unica mensagem user, porque a combinacao anterior com systemInstruction ficou lenta/pendurada neste ambiente.
- O provider recebe apenas um `responseSchema` solto para forcar JSON com os campos esperados; as restricoes fortes continuam no validador local `D:\Repos\.openclaw\scripts\Test-VertexOrchestratorDecision.ps1`, que decide se a saida pode ser aceita.
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` passou a explicitar `vertexOrchestrator.maxOutputTokens = 2000` e o bloco `notes.status = live_validated`.
- Evidencia verificada em 2026-03-29: Invoke-VertexOrchestrator.ps1 -InputText "erro SQL em producao ao abrir prestacao de contas" retornou decisao valida e gravou nova linha em D:\Repos\.openclaw\runtime\swarm\state\orchestrator-decisions.jsonl.
- Fontes: D:\Repos\.openclaw\scripts\Invoke-VertexOrchestrator.ps1; D:\Repos\.openclaw\scripts\Test-VertexOrchestratorDecision.ps1; D:\Repos\.openclaw\runtime\swarm\swarm-config.json; D:\Repos\.openclaw\runtime\swarm\state\orchestrator-decisions.jsonl

### 2026-03-29 | openclaw | Bootstrap precisa carregar swarm-config antes do snapshot de health
- `D:\Repos\.openclaw\scripts\Start-OpenClawEnvironment.ps1` nao pode acessar `$swarmConfig.logsRoot` no bloco de health sem antes carregar `D:\Repos\.openclaw\runtime\swarm\swarm-config.json`; com `Set-StrictMode`, isso aborta o bootstrap e deixa o pos-reboot inconsistente.
- O bootstrap agora inicializa `$swarmConfig` explicitamente antes de montar o snapshot final, o que preserva a leitura do `codex-lb.pid` pelo `logsRoot` canonico sem depender de variavel implicita.
- Evidencia verificada em 2026-03-29: apos a correcao, o bootstrap roda ate o fim, `D:\Repos\Relatorios\OpenClaw\doctor-latest.json` volta a marcar `dashboard-bridge` como `ok` e o runner `D:\Repos\.openclaw\scripts\Run-OpenClawDashboardBridge.ps1` consegue manter o endpoint `http://127.0.0.1:18795/dashboard-bridge/health` saudavel.
- Fontes: D:\Repos\.openclaw\scripts\Start-OpenClawEnvironment.ps1; D:\Repos\.openclaw\scripts\Run-OpenClawDashboardBridge.ps1; D:\Repos\Relatorios\OpenClaw\doctor-latest.json; D:\Repos\.openclaw\runtime\orq\state\dashboard-bridge-state.json

### 2026-03-31 | openclaw | Regra pratica para plugin comunitario com servico externo
- O plugin comunitario `google-stitch` (repo `Electric-Coding-LLC/plugins`) foi classificado como aderencia parcial ao baseline: manifesto declarativo simples, sem executavel local, mas com dependencia de endpoint externo e API key (`https://stitch.googleapis.com/mcp` via `X-Goog-Api-Key`).
- Para o fluxo principal, o contrato fica: plugin externo/comunitario entra primeiro em sandbox, com aprovacao explicita para conexao externa e para uso de credencial, sem promocao automatica para baseline canonico.
- Minimo de hardening obrigatorio antes de uso regular: segredo apenas local (`.mcp.json` nao versionado), validacao de endpoint unico esperado, smoke test controlado e rollback claro (desabilitar marketplace entry + remover chave local).
- Tags: openclaw, plugin, hardening, external-service, governance
- Fontes: D:\Repos\SOUL.md; D:\Repos\AGENTS.md; D:\Repos\.openclaw\OPENCLAW-OPERATING-CONTRACT.md; https://github.com/Electric-Coding-LLC/plugins/tree/main/google-stitch; https://raw.githubusercontent.com/Electric-Coding-LLC/plugins/main/google-stitch/.codex-plugin/plugin.json; https://raw.githubusercontent.com/Electric-Coding-LLC/plugins/main/google-stitch/.mcp.json.example
### 2026-04-01 | openclaw | Worker Codex do swarm deve preferir shim de aplicacao no Windows e probe precisa espelhar execucao real
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-CodexWorker.ps1` passou a ordenar `Get-Command codex` para preferir `Application` (`.cmd`, `.exe`) antes de `ExternalScript` (`.ps1`) no Windows, evitando fragilidade do wrapper PowerShell `codex.ps1` no runtime do swarm.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Test-SwarmWorkerAvailability.ps1` passou a resolver o mesmo executavel preferido e enviar o prompt por stdin para o probe reproduzir melhor o worker real.
- O pipeline nativo do Windows PowerShell para `codex.cmd` ainda pode travar em `Reading prompt from stdin...`; por isso `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-CodexWorker.ps1` e `Test-SwarmWorkerAvailability.ps1` passaram a preferir `pwsh.exe` como host auxiliar quando chamados fora do PowerShell Core, mantendo compatibilidade com o supervisor legado em `powershell.exe`.
- Evidencia verificada em 2026-04-01: smoke local de `Invoke-CodexWorker.ps1` no worktree `D:\WT\cwazu\cwazu-002-p3` respondeu `OK`; `Test-SwarmWorkerAvailability.ps1 -ForceAll` retornou `healthyWorkers = [codex-lb, codex-lb-2]`.
- Tags: openclaw, swarm, codex, worker-runtime
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-CodexWorker.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Test-SwarmWorkerAvailability.ps1; D:\Repos\.openclaw\runtime\swarm\state\diagnostics\cwazu-002-p3-codex-lb-attempt1-20260401T152953085Z.json

### 2026-04-01 | openclaw | Tasks do swarm agora sincronizam GitHub Issues com branch, PR e ChamadoWeb
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` ganhou o bloco `githubIssues` com o contrato canônico para criar issue na abertura da task, comentar início/resultado/bloqueios e fechar no término.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1` passou a concentrar os helpers de integração (`gh issue list/view/create/comment/close`), dedupe por `issueCommentKeys`, resolução do repositório GitHub pela `origin` e extração de link de ChamadoWeb a partir de `manual://chamadoweb-sync/<id>`.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Add-SwarmTask.ps1` passou a criar ou reusar issue no intake e persistir `issueNumber`, `issueUrl`, `issueState`, `issueSyncStatus` e `issueSyncError` dentro da task.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1` agora publica comentários de execução relevantes na issue (início, sucesso da tentativa, cooldown, watchdog, falha, PR fechado sem merge) e fecha a issue quando a task chega a `completed`.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Run-SwarmWorker.ps1` e `D:\Repos\.openclaw\runtime\swarm\Scripts\Run-SwarmPrReview.ps1` passaram a incluir `Closes #<issue>` no corpo do PR automático, garantindo vínculo explícito entre issue, branch e merge.
- Quando a task nasce do ChamadoWeb, a issue inclui no corpo o link `https://chamadowebapi.azurewebsites.net/CHChamado/<id>` para o chamado de origem.
- Evidência verificada em 2026-04-01: parse sintático OK nos cinco scripts alterados; a integração live com GitHub não foi disparada durante a implementação para evitar criação/comentário/fechamento real de issues em produção.

### 2026-04-02 | openclaw | codex-lb passa a ter autostart explícito no Windows via Startup + launcher endurecido
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Start-CodexLb.ps1` deixou de depender apenas de PID legado e passou a usar `logsRoot` canônico do `swarm-config.json`, validar `http://127.0.0.1:2455/health`, reaproveitar listener saudável já existente e regravar `codex-lb.pid` com o PID real quando o arquivo estiver stale.
- O launcher agora falha de forma observável: quando o `codex-lb` não fica saudável dentro do timeout, ele retorna JSON com `healthy=false`, `stderrTail` e exit code `1`, em vez de reportar start cego.
- `D:\Repos\.openclaw\scripts\Ensure-OpenClawCodexLbAutostart.ps1` passou a instalar `C:\Users\l.sousa\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup\OpenClaw Codex LB.vbs`, que chama `Run-HiddenCommand.vbs` para subir o `Start-CodexLb.ps1` automaticamente no logon do Windows sem janela.
- Evidência verificada em 2026-04-02: o launcher do `Startup` foi materializado e executado com `launcher_exit=0`; `Start-CodexLb.ps1` respondeu `alreadyRunning=true`, `healthy=true`, `pid=8576`; o endpoint `http://127.0.0.1:2455/health` permaneceu `{"status":"ok"}`.

### 2026-04-02 | openclaw | Dashboard local ganhou slice de runtime OpenClaw estilo ClawBox sem stack paralelo
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawRuntimeSnapshot.ps1` virou o coletor canônico do runtime OpenClaw para o dashboard, gerando `D:\Repos\Relatorios\OpenClaw\openclaw-runtime.json` com snapshot sanitizado de gateway, modelos, sessoes, canais configurados e skills.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` e `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html` passaram a expor esse artefato como subaba `OpenClaw` na secao Runtime, com grid dedicado e comando `Atualizar snapshot OpenClaw`.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1` ganhou a acao `refreshOpenClawRuntime`, que executa o snapshot e re-renderiza `D:\Repos\Relatorios\OpenClaw\dashboard.html` pelo mesmo caminho usado pelo painel web.
- O coletor precisou ser endurecido para o runtime legado do bridge: chamadas ao CLI passaram a usar `System.Diagnostics.Process` com preferencia por `openclaw.cmd`, evitando `NativeCommandError` do wrapper `openclaw.ps1`; a leitura de `generated/main/openclaw.json` ganhou fallback via `pwsh` porque o arquivo pode usar sintaxe tolerada pelo PowerShell 7 mas rejeitada pelo `ConvertFrom-Json` do Windows PowerShell; o parse de skills passou a buscar os labels textuais (`Eligible`, `Missing requirements`, etc.) em vez de depender de icones Unicode.
- Evidencia verificada em 2026-04-02: `Invoke-OpenClawRuntimeSnapshot.ps1` funcionou tanto em `pwsh` quanto em `powershell.exe`; `POST /dashboard-bridge/execute` com `action=refreshOpenClawRuntime` retornou `ok=true`, atualizou `openclaw-runtime.json`, marcou `dashboardRegenerated=true` e o health do bridge passou a registrar `lastAction=refreshOpenClawRuntime`.
- Regra operacional associada: alteracao de roteamento do bridge exige reiniciar o listener (`Run-OpenClawDashboardBridge.ps1` / `Invoke-OpenClawDashboardBridge.ps1`) se ele ja estiver residente, porque o processo em memoria nao recarrega novas acoes sozinho.

### 2026-04-02 | openclaw | Runtime do dashboard passou a expor auth/perfis e identidade do device sem vazar segredo
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawRuntimeSnapshot.ps1` ganhou `authProfiles` e `device` no payload do snapshot.
- `authProfiles` resume apenas metadados seguros do `auth.profiles`: total de perfis, agrupamento por provider, modos (`api_key`, `oauth`, `token`) e chaves lógicas de perfil; nao inclui API key, token ou segredo.
- `device` resume apenas presenca de identidade local e metadados seguros de `D:\Users\l.sousa\.openclaw\identity\device.json` / `device-auth.json`: presenca de `deviceId`, par de chaves, `createdAt`, providers de token, `role`, `scopeCount` e flags como `hasRefreshToken`; nao serializa PEM, token, refresh token ou access token.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` ganhou fallback padrao para essas duas secoes, e `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html` passou a renderizar os cards `Auth / Perfis` e `Device` dentro de `Runtime > OpenClaw`.
- Durante essa extensao, o template apresentou regressao de frontend por `)` extra no card `Gateway`; o painel em `http://127.0.0.1:18795/dashboard-bridge/dashboard` deixava de inicializar com `Unexpected token ')'`. O template foi corrigido e o HTML final voltou a passar no parser JS e no browser sem erros de console.
- Evidencia verificada em 2026-04-02: a rota do dashboard respondeu `200`; o browser mostrou a aba `Runtime > OpenClaw` com os cards `Auth / Perfis` e `Device`; o snapshot ativo passou a mostrar `authProfiles.totalProfiles = 4`, `device.devicePresent = true` e `device.tokenProviderCount = 1`.

### 2026-04-02 | swarm | Supressoes duraveis agora cobrem tasks de segredos em appsettings do BNB_Novo e ChamadoWebAPI
- `D:\Repos\.openclaw\runtime\swarm\state\task-suppressions.json` ganhou as regras `cwbnn-appsettings-secrets-no-rerun` e `chwapi-appsettings-secrets-no-rerun`.
- As regras bloqueiam reexecucao/recriacao das tasks `cwbnn-002`, `cwbnn-006`, `cwbnn-008`, `chwapi-009`, `chwapi-010` e `chwapi-011`, combinando `taskIds`, trechos de titulo e termos de `appsettings*`/segredos para manter a decisao duravel.
- As seis tasks foram removidas do backlog vivo em `D:\Repos\Relatorios\Swarm\task-backlog.json` e arquivadas em `D:\Repos\.openclaw\runtime\swarm\state\removed-tasks.json` com motivo `user_archived_no_rerun_2026-04-02`.

### 2026-04-02 | swarm | Auditoria e backlog agora ignoram rotacao de credenciais expostas e endurecimento de config so para proteger segredo
- `D:\Repos\.openclaw\prompts\autonomous-code-audit.md` passou a instruir explicitamente a auditoria a ignorar achados cuja remediacao principal seja rotacionar/revogar credenciais expostas, remover segredos do Git, mover segredos para vault/secret store/cofre ou alterar `*.config`/`appsettings*` apenas para proteger informacoes.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1` ganhou `Test-SwarmAuditTaskIgnoredByPolicy`, e `D:\Repos\.openclaw\runtime\swarm\Scripts\Generate-SwarmTaskBacklog.ps1` passou a aplicar esse filtro em tasks preservadas, candidatas e finais antes de consolidar o backlog.
- `D:\Repos\.openclaw\runtime\swarm\state\task-suppressions.json` ganhou a regra `global-audit-secret-rotation-no-rerun` para bloquear reentrada imediata das tasks `cwbnb-002`, `cwbnb-003`, `cwazu-001` e `chwext-005`.
- As quatro tasks foram removidas do backlog vivo por `D:\Repos\.openclaw\runtime\swarm\Scripts\Remove-SwarmTask.ps1` com motivo `user_ignore_audit_secret_rotation_config_protection_2026-04-02`, arquivadas em `D:\Repos\.openclaw\runtime\swarm\state\removed-tasks.json` e nao reapareceram apos `Generate-SwarmTaskBacklog.ps1`.
- Evidencia verificada em 2026-04-02: parse sintatico OK em `Swarm-Common.ps1` e `Generate-SwarmTaskBacklog.ps1`; `active-tasks.json` ficou vazio; a busca final no backlog por `rotacionar`, `credenciais expostas`, `vault`, `web.config`, `app.config` e `appsettings` retornou `NO_MATCHES`.

### 2026-04-02 | openclaw | Aba Runtime > OpenClaw ganhou acoes operacionais inline via bridge
- `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html` passou a renderizar no card `Gateway` os botoes `Atualizar snapshot`, `Rodar doctor` e `Iniciar ambiente`, todos chamando o bridge local.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1` ganhou as acoes `openclawDoctor` e `startOpenClawEnvironment`, alem de expor essas acoes em `allowedActions`.
- `openclawDoctor` roda `D:\Repos\.openclaw\scripts\Invoke-OpenClawDoctor.ps1 -Json` de forma sincrona e registra resumo legivel no health do bridge (`Doctor OpenClaw: status=..., ok=..., warning=..., error=...`).
- `startOpenClawEnvironment` nao deve rodar sincronicamente dentro do request do bridge; o fluxo correto e disparar `D:\Repos\.openclaw\scripts\Start-OpenClawEnvironment.ps1` em background via `Start-Process`, retornar ack imediato ao painel e deixar o usuario atualizar o snapshot depois.
- O mesmo slice passou a expor os cards `Doctor` e `Bootstrap` dentro de `Runtime > OpenClaw`, lendo `D:\Repos\Relatorios\OpenClaw\doctor-latest.json`, `D:\Repos\.openclaw\runtime\main\logs\startup-bootstrap-health.json` e `D:\Repos\.openclaw\runtime\main\logs\startup-bootstrap.log` direto no `Invoke-OpenClawDashboard.ps1`, sem vazar segredos ou despejar os artefatos brutos na UI.
- O card `Gateway` ganhou a acao `Recuperar gateway`, e o bridge ganhou `recoverOrqGateway`, que roteia para `D:\Repos\.openclaw\scripts\Start-OpenClawGatewayTask.ps1` com `GatewayCmdPath = C:\Users\l.sousa\.openclaw-orq\gateway.cmd`, `Port = 18790` e `HealthPath = /health`.
- `D:\Repos\.openclaw\scripts\Start-OpenClawEnvironment.ps1` foi ajustado para filtrar entradas nulas em `$results` e para interpolar `${taskName}: $status` sem parser error no PowerShell.
- Evidencia verificada em 2026-04-02: `POST /dashboard-bridge/execute` com `action=openclawDoctor` retornou `ok=true` e resumo `status=warning, ok=26, warning=18, error=0`; `action=startOpenClawEnvironment` retornou `ok=true` em modo background; `action=recoverOrqGateway` retornou `ok=true` e resumo `Gateway orq ja estava saudavel na porta 18790`; o browser passou a mostrar `Gateway`, `Doctor`, `Bootstrap`, `Modelos`, `Sessoes`, `Canais`, `Auth / Perfis`, `Device` e `Skills` na mesma aba.

### 2026-04-02 | openclaw | Intake IMAP de email foi restaurado, reativado e voltou a subir no logon do Windows
- O intake da mailbox `openclaw@tectrilha.com.br` ficou operacional de novo apos restaurar da pasta `D:\Repos\.openclaw\archive\2026-03-31\email-intake-deprecated` os scripts `Invoke-EmailChamadoHelpers.ps1`, `Invoke-EmailChamadoIntake.ps1`, `Invoke-ImapEmailIntake.ps1`, `Invoke-EmailDropIntake.ps1`, `Run-EmailDropIntake.ps1`, `imap_email_intake.py`, `Archive-EmailFailedChamado.ps1`, `Retry-EmailFailedChamado.ps1` e `Start-OpenClawEmailIntake.vbs` para `D:\Repos\.openclaw\scripts\`.
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` voltou a expor `emailIntake.enabled = true` e `emailIntake.imap.enabled = true`, e `D:\Repos\.openclaw\scripts\Start-OpenClawEnvironment.ps1` passou a inicializar `Run-EmailDropIntake.ps1` em vez de manter email intake permanentemente desabilitado.
- O runner continuo voltou a ficar residente (`Run-EmailDropIntake.ps1`) e o autostart do Windows foi restaurado em `C:\Users\l.sousa\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup\OpenClaw Email Intake.vbs`.
- Evidencia verificada em 2026-04-02: `D:\Repos\.openclaw\runtime\orq\state\email-drop-intake-state.json` avancou sem erro ate `lastSuccessfulPollAt = 2026-04-02T10:03:43.6115202-03:00`, `pollCount = 33140`, `lastError = null`, e registrou a entrega `imap://openclaw@tectrilha.com.br/INBOX/2 -> numero=418 | id=37737`.

### 2026-04-02 | openclaw | Sync ChamadoWeb passou a ser idempotente para acompanhamentos automaticos e houve saneamento dos chamados contaminados
- `D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1` ganhou dedupe por estado visivel do chamado. O sync agora consulta os acompanhamentos existentes e deixa de republicar `Analise iniciada`, `Analise concluida...` e `Automacao concluida...` quando ja houver uma ocorrencia equivalente para o mesmo estado.
- A causa raiz verificada em producao foi combinacao de dois fatores: `eligibleSituacoes` inclui `90` e `65` em `D:\Repos\.openclaw\runtime\swarm\chamadoweb-automation.json`, e o fluxo antigo republicava acompanhamentos mesmo quando o chamado ja estava vinculado a task/link existente.
- Evidencia verificada em 2026-04-02: o chamado `37762` acumulou dezenas de pares repetidos; a limpeza operacional removeu por soft delete `63` duplicatas desse chamado e mais `805` duplicatas espalhadas por `13` chamados relacionados (`36008`, `37602`, `37718`, `37719`, `37720`, `37734`, `37735`, `37736`, `37737`, `37757`, `37758`, `37760`, `37761`).
- Validacao final em producao: a varredura do sync em `DryRun` retornou `[]` para os chamados afetados, indicando que o fluxo nao tentava mais escrever novos acompanhamentos duplicados para esse conjunto.

### 2026-04-03 | chamadoweb | Nunca marcar chamado como atendido automaticamente
- Regra operacional duravel do fluxo: agente, automacao e swarm nao devem marcar chamado como `atendido` por conta propria.
- O encerramento/marcacao de atendimento do chamado deve permanecer estritamente manual, feito pelo usuario no sistema.
- Em fluxos de triagem, apoio tecnico, automacao, sync ou acompanhamento, o maximo permitido e mover entre estados operacionais intermediarios previstos no contrato; nao concluir atendimento automaticamente.
- Tags: chamadoweb, governance, atendimento, manual-only
- Fontes: D:\Repos\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1; D:\Repos\.openclaw\scripts\Update-ChamadoWeb.ps1; D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html

### 2026-04-03 | openclaw | Heartbeat rotativo, limpeza de sessao no /new e busca SQLite auxiliar de memoria
- `D:\Repositórios\HEARTBEAT.md` deixou de ser placeholder e passou a definir heartbeat rotativo guiado por `D:\Repositórios\heartbeat-state.json`, com checks `gateway`, `swarm`, `email-intake`, `tasks` e `memory`; o contrato agora manda executar apenas a verificacao mais atrasada dentro da janela horaria e responder `HEARTBEAT_OK` quando nada for acionavel.
- `D:\Repositórios\.openclaw\runtime\main\openclaw.template.json` ganhou `hooks.internal.entries.delete-session-on-new.enabled = true`.
- `D:\Repositórios\.openclaw\runtime\main\hooks\delete-session-on-new\handler.ts` implementa limpeza da sessao anterior quando `/new` roda: remove a entrada antiga de `sessions.json` e tenta arquivar o transcript em `sessions/archive/`.
- `D:\Repositórios\.openclaw\scripts\Sync-OpenClawRuntime.ps1` passou a copiar `runtime\main\hooks\` para `runtime\generated\main\hooks\` durante a regeneracao do runtime, e deixou de abortar quando `Set-Acl` falha por problema ambiental de trust/domain no Windows; nesses casos ele apenas emite warning.
- `D:\Repositórios\.openclaw\scripts\build-workspace-memory-index.mjs` e `D:\Repositórios\.openclaw\scripts\search-workspace-memory.mjs` criam um indice FTS5 local em `D:\Repositórios\.openclaw\runtime\memory-search\workspace-memory.db` sobre `AGENTS.md`, `SOUL.md`, `USER.md`, `MEMORY.md`, `HEARTBEAT.md`, `TOOLS.md`, `memory\` e `\.openclaw\reports\`, como busca auxiliar barata sem embeddings.
- O reforco tecnico atual fica em `D:\Repositórios\.openclaw\scripts\Sync-ChamadoWebSwarm.ps1`, que nao publica mais `CompletedSituacao` ao concluir task do swarm; em `D:\Repositórios\.openclaw\scripts\Update-ChamadoWeb.ps1`, que nao aceita mais preset `completed` nem `Situacao` customizada igual a `CompletedSituacao`; e em `D:\Repositórios\.openclaw\scripts\openclaw-dashboard.template.html`, que removeu a opcao `Concluido` e bloqueia esse valor via UI.
- Tags: openclaw, heartbeat, sessions, memory-search
- Fontes: D:\Repos\HEARTBEAT.md; D:\Repos\heartbeat-state.json; D:\Repos\.openclaw\runtime\main\openclaw.template.json; D:\Repos\.openclaw\runtime\main\hooks\delete-session-on-new\handler.ts; D:\Repos\.openclaw\scripts\Sync-OpenClawRuntime.ps1; D:\Repos\.openclaw\scripts\build-workspace-memory-index.mjs; D:\Repos\.openclaw\scripts\search-workspace-memory.mjs

### 2026-04-03 | openclaw | Handoff portatil derivado do contexto local
- `D:\Repos\.openclaw\scripts\New-OpenClawHandoff.ps1` passou a gerar artefatos `openclaw-handoff/v1` em `D:\Repos\Relatorios\OpenClaw\handoffs\`, sempre como snapshot derivado e explicitamente nao canonico.
- O script suporta tres modos: `identity` (contrato operacional + perfil do usuario), `workspace` (identidade + memoria curada + memoria diaria recente) e `task` (workspace + task do backlog + `selectedContextPackage` do swarm).
- O modo `task` reaproveita `D:\Repos\.openclaw\runtime\swarm\state\context-packages\<task>.json` quando existir e, se faltar, resolve um pacote novo a partir do backlog via `D:\Repos\.openclaw\scripts\Resolve-OpenClawTaskContext.ps1`.
- O handoff puxa identidade de `D:\Repos\SOUL.md`, `D:\Repos\AGENTS.md` e `D:\Repos\USER.md`, memoria de `D:\Repos\MEMORY.md` e `D:\Repos\memory\`, e inclui referencias de origem para auditoria reversa ao filesystem.
- Ha sanitizacao minima no payload gerado: linhas com `password`, `secret`, `api key`, `refresh/access token` e chaves privadas sao redigidas; emails tambem sao mascarados.
- `D:\Repos\TOOLS.md` ganhou a secao `Handoff portatil de contexto` com os comandos operacionais de geracao.
- Tags: openclaw, handoff, context-selection, portability
- Fontes: D:\Repos\.openclaw\scripts\New-OpenClawHandoff.ps1; D:\Repos\.openclaw\scripts\Resolve-OpenClawTaskContext.ps1; D:\Repos\.openclaw\runtime\swarm\state\context-packages; D:\Repos\Relatorios\OpenClaw\handoffs; D:\Repos\TOOLS.md

### 2026-04-03 | workspace | Markdown operacional passou a priorizar bridges e overlays curtos em vez de templates genéricos
- O workspace passou a tratar `CLAUDE.md` de raiz como arquivo de compatibilidade curto apontando para `AGENTS.md`, em vez de manter um segundo contrato longo em drift.
- Arquivos genericos de `SOUL`, `USER`, `TOOLS`, `IDENTITY` e `HEARTBEAT` que ainda existiam como template cru em overlays locais passaram a ser notes curtas explicando fonte canonica e uso opcional, reduzindo persona-placeholder e duplicacao de contrato.
- `MEMORY.md` passou a ter camadas explicitas de leitura rapida (`Nucleo duravel` e `Mapa tematico`) antes do registro detalhado, e as memorias diarias recentes passaram a distinguir resumo do dia de log bruto.
- A regra operacional implicita agora e: markdown operacional deve priorizar fonte canonica clara, bridge curto e overlay opcional; templates genericos so permanecem quando fizerem parte de scaffold deliberado ou documentacao de terceiros.
- Tags: workspace, docs, governance, markdown
- Fontes: D:\Repos\MEMORY.md; D:\Repos\memory\2026-04-03.md; D:\Repos\ConveniosWebBNB\CLAUDE.md; D:\Repos\ConveniosWebBNB_Novo\docs\agent-workspace\SOUL.md; D:\Repos\ConveniosWebData\docs\agent-workspace\TOOLS.md; D:\Repos\ConveniosWebVSAzure_Default\docs\agent-workspace\HEARTBEAT.md

### 2026-04-03 | workspace | Auditor de convencoes markdown passou a validar bridges, overlays e memorias
- Foi criado `D:\Repos\.openclaw\scripts\Test-WorkspaceMarkdownConventions.ps1` para auditar o conjunto de markdown operacional monitorado em `D:\Repos` e `C:\Users\l.sousa`, com foco em `CLAUDE.md`, overlays `SOUL/USER/TOOLS/IDENTITY/HEARTBEAT`, `MEMORY.md` e memorias diarias.
- O auditor verifica se `CLAUDE.md` com `AGENTS.md` irmao continua curto e compatível, detecta placeholders genericos remanescentes, exige `Nucleo duravel`/`Mapa tematico`/`Registro detalhado` em `MEMORY.md` e exige `Resumo rapido`/`Mapa do dia`/`Registro bruto` nos diarios.
- A curadoria passou a cobrir tambem `D:\Repos\memory\2026-03-19.md` e `D:\Repos\memory\2026-03-21.md` ate `D:\Repos\memory\2026-03-30.md`, alem de notes de compatibilidade em `C:\Users\l.sousa\.openclaw\workspace\`.
- Validacao real: o script retornou `IssueCount = 0`, `Status = ok` e `ScannedFileCount = 76` tanto em modo texto quanto em `-Json`.
- Tags: workspace, docs, validation, markdown
- Fontes: D:\Repos\.openclaw\scripts\Test-WorkspaceMarkdownConventions.ps1; D:\Repos\TOOLS.md; D:\Repos\USER.md; C:\Users\l.sousa\.openclaw\workspace\AGENTS.md; D:\Repos\memory\2026-03-19.md; D:\Repos\memory\2026-03-21.md

### 2026-04-03 | codex | Workflow skills locais leves no runtime global
- O runtime global do Codex em `C:\Users\l.sousa\.codex` passou a ter um conjunto local e enxuto de workflow skills, inspirado em `obra/superpowers`, mas deliberadamente sem instalar o pacote inteiro nem adotar a skill central `using-superpowers`.
- Foram criadas em `C:\Users\l.sousa\.codex\skills\` as skills `systematic-debugging-lite`, `verification-before-close`, `code-review-findings` e `implementation-planning-lite`.
- O pack local foi estendido depois com `self-improvement-lite`, mas em modo deliberadamente restrito: sem hooks, sem `.learnings/` paralelo e sem promocao automatica; a skill existe apenas para decidir se um aprendizado verificado merece persistencia e qual arquivo canonico e o destino correto.
- O runtime embutido do OpenClaw tambem passou a refletir essa capacidade no pack `openclaw-workspace`: `Build-OpenClawEmbeddedSkills.ps1` agora gera `runtime/skills/openclaw-workspace/docs/local-workflow-skills.md` a partir de `C:\Users\l.sousa\.codex\AGENTS.md` e `...\\self-improvement-lite\\SKILL.md`, e `OpenClaw.TaskContext.psm1` pode incluir esse bridge como contexto opcional de baixa prioridade.
- O objetivo do pack e reforcar comportamentos recorrentes de maior valor para o ambiente: provar causa raiz antes de corrigir, separar evidencia de suposicao no encerramento, responder reviews com findings primeiro e usar plano curto apenas quando a tarefa realmente pede decomposicao.
- `C:\Users\l.sousa\.codex\AGENTS.md` ganhou a secao `Local workflow skills`, apontando para essas skills como apoio contextual e deixando explicito que elas nao sao obrigatorias para pedidos triviais.
- Validacao real: as skills do pack local passaram em `quick_validate.py`, e `self-improvement-lite` ficou com escopo explicitamente narrow para nao competir com a memoria canonica do workspace.
- Validacao adicional observada: `Build-OpenClawEmbeddedSkills.ps1` regenerou `Relatorios/OpenClaw/embedded-skills.json` com `openclaw-workspace.docCount = 4` e o arquivo gerado `runtime/skills/openclaw-workspace/docs/local-workflow-skills.md`; `openclaw-runtime.json` passou a refletir `skills.runtimePackCount = 5`.
- Tags: codex, skills, workflow, local-runtime
- Fontes: C:\Users\l.sousa\.codex\AGENTS.md; C:\Users\l.sousa\.codex\skills\systematic-debugging-lite\SKILL.md; C:\Users\l.sousa\.codex\skills\verification-before-close\SKILL.md; C:\Users\l.sousa\.codex\skills\code-review-findings\SKILL.md; C:\Users\l.sousa\.codex\skills\implementation-planning-lite\SKILL.md; C:\Users\l.sousa\.codex\skills\self-improvement-lite\SKILL.md; C:\Users\l.sousa\.codex\skills\.system\skill-creator\scripts\quick_validate.py
- Fontes: C:\Users\l.sousa\.codex\AGENTS.md; C:\Users\l.sousa\.codex\skills\systematic-debugging-lite\SKILL.md; C:\Users\l.sousa\.codex\skills\verification-before-close\SKILL.md; C:\Users\l.sousa\.codex\skills\code-review-findings\SKILL.md; C:\Users\l.sousa\.codex\skills\implementation-planning-lite\SKILL.md; C:\Users\l.sousa\.codex\skills\self-improvement-lite\SKILL.md; C:\Users\l.sousa\.codex\skills\.system\skill-creator\scripts\quick_validate.py; D:\Repos\.openclaw\scripts\Build-OpenClawEmbeddedSkills.ps1; D:\Repos\.openclaw\scripts\OpenClaw.TaskContext.psm1; D:\Repos\Relatorios\OpenClaw\embedded-skills.json

### 2026-04-03 | codex/openclaw | MegaMemory do workspace com grafos por projeto e aliases compativeis
- O runtime do Codex/OpenClaw em `D:\Repos\.openclaw\runtime\codex\config.template.toml` expõe MCPs MegaMemory por projeto do workspace, com aliases compativeis quando dois repositorios representam o mesmo produto em camadas ou versoes diferentes.
- O mapeamento ativo ficou assim: `.openclaw -> megamemory-openclaw`, `ChamadoWebAPI + ChamadoWebExterno -> megamemory-chamadowebapi / megamemory-chamadowebexterno` apontando para o mesmo `knowledge.db`, `codex-lb -> megamemory-codex-lb`, `ConveniosWebBNB + ConveniosWebBNB_Novo -> megamemory-bnb / megamemory-convenioswebbnb-novo` apontando para o mesmo `knowledge.db`, `ConveniosWebData -> megamemory-convenioswebdata`, `ConveniosWebExterno -> megamemory-convenioswebexterno`, `ConveniosWebVSAzure_Default -> megamemory-azure`, `SuperPowers -> megamemory-superpowers`.
- O runtime gerado `D:\Repos\.openclaw\runtime\generated\codex\config.toml` foi regenerado e passou a materializar esses MCPs no bootstrap local do Codex.
- Foram materializados bancos `knowledge.db` em `\.megamemory\` para os projetos do conjunto; os bancos compartilhados de `ConveniosWebBNB/ConveniosWebBNB_Novo` e `ChamadoWebAPI/ChamadoWebExterno` concentram a memoria semantica comum desses produtos.
- Os `AGENTS.md` dos repos passaram a declarar explicitamente o MCP correto por repo ou alias compativel do mesmo projeto, e os repositorios passaram a ignorar `.megamemory/` no Git.
- Como parte da limpeza de drift anterior, os conceitos do `.openclaw` que tinham sido gravados no `megamemory-bnb` foram migrados para `D:\Repos\.openclaw\.megamemory\knowledge.db`; o grafo `bnb` ficou sem conceitos ativos do `.openclaw`.
- Evidencia operacional e auditoria ficaram documentadas em `D:\Repos\Relatorios\OpenClaw\megamemory-routing-audit-2026-04-03.md` e `D:\Repos\Relatorios\OpenClaw\megamemory-rollout-all-repos-2026-04-03.md`.
- Tags: codex, openclaw, megamemory, workspace, runtime
- Fontes: D:\Repos\.openclaw\runtime\codex\config.template.toml; D:\Repos\.openclaw\runtime\generated\codex\config.toml; D:\Repos\.openclaw\README.md; D:\Repos\.openclaw\AGENTS.md; D:\Repos\Relatorios\OpenClaw\megamemory-routing-audit-2026-04-03.md; D:\Repos\Relatorios\OpenClaw\megamemory-rollout-all-repos-2026-04-03.md

### 2026-04-03 | openclaw | Dashboard ganhou presets configuraveis e catalogo visual de workspaces inspirado no Superset
- O dashboard OpenClaw passou a carregar um contrato local de presets em `D:\Repos\.openclaw\runtime\swarm\workspace-presets.json`, com dois blocos: `presets[]` para operacoes rapidas do bridge e `workspaces[]` para metadados por repo.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` passou a expor `commandCenter.workspacePresets`, `commandCenter.workspaceCatalog`, `commandCenter.workspacePresetPath` e `commandCenter.workspacePresetSource`, alem de enriquecer `scripts` com paths para `Invoke-OpenClawDoctor.ps1`, `Start-OpenClawEnvironment.ps1` e `Start-OpenClawGatewayTask.ps1`.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1` ganhou a acao `runWorkspacePreset`, que resolve `presetId` no contrato local e delega para a acao real do bridge (`getStatus`, `supervisor`, `planner`, `refreshOpenClawRuntime`, `openclawDoctor`, `startOpenClawEnvironment`, `recoverOrqGateway`, `workerProbe`, `syncChamados`).
- `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html` deixou de hardcodar os quick commands principais e passou a renderizar os presets do contrato em `Comandos > Atalhos imediatos`; a mesma tela ganhou `Comandos > Catalogo de workspaces`, com cards por repo exibindo fila, links e presets preferidos.
- O backlog operacional mais amplo dessa linha de produto foi materializado em `D:\Repos\Relatorios\OpenClaw\superset-inspired-backlog-2026-04-03.md`, priorizando ainda `setup/run/teardown`, registry de portas, review queue forte e diff viewer.
- Evidencia verificada em 2026-04-03: `workspace-presets.json` parseou com `ConvertFrom-Json`; `Invoke-OpenClawDashboard.ps1` regenerou `D:\Repos\Relatorios\OpenClaw\dashboard.html`; Playwright abriu `http://127.0.0.1:18795/dashboard-bridge/dashboard?tab=commands` sem erros de console e a snapshot exibiu `9 presets / 8 workspaces / bridge online`.
- Tags: openclaw, dashboard, presets, workspaces, superset-inspired
- Fontes: D:\Repos\.openclaw\runtime\swarm\workspace-presets.json; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1; D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html; D:\Repos\Relatorios\OpenClaw\superset-inspired-backlog-2026-04-03.md

### 2026-04-03 | openclaw | Dashboard/bridge ganharam ciclo `setup/run/teardown` por workspace com estado e logs persistidos
- O OpenClaw passou a carregar `D:\Repos\.openclaw\runtime\swarm\workspace-ops.json` como contrato canonico de operacoes por workspace, com `setup`, `run`, `teardown` e `preview` por repo.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1` ganhou a acao `runWorkspaceOperation`, que resolve a operacao do repo no contrato local e executa `exec`, `spawn` ou `stop`, persistindo estado em `D:\Repos\.openclaw\runtime\swarm\state\workspace-ops\` e logs em `D:\Repos\.openclaw\runtime\swarm\logs\workspace-ops\`.
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` passou a ler esse estado e a derivar sinais de runtime no catalogo de workspaces; quando um `run` morre cedo e deixa stderr, o dashboard mostra o erro do proprio repo em vez de mascarar como simples idle.
- `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html` ganhou botoes `setup`, `run`, `teardown`, preview e links de log no tab `Comandos > Catalogo de workspaces`.
- Evidencia verificada em 2026-04-03:
  - `ConveniosWebBNB_Novo / setup` concluiu `dotnet restore` via `runWorkspaceOperation`.
  - `ConveniosWebExterno / run` persistiu `convenioswebexterno-run.json` e logs; o stderr mostrou falha real de Vite (`failed to load config ... vite.config.ts`, `defaultLoader is not a function`), o que confirmou a infraestrutura e isolou o defeito no repo.
  - `ConveniosWebExterno / teardown` atualizou o estado persistido para `stopped`.
  - Playwright confirmou no dashboard o card `Frontend externo` com `Run stopped` e o erro de `vite.config.ts` exposto no proprio catalogo.
- Tags: openclaw, dashboard, workspace-ops, preview, superset-inspired
- Fontes: D:\Repos\.openclaw\runtime\swarm\workspace-ops.json; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboardBridge.ps1; D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1; D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html; D:\Repos\.openclaw\runtime\swarm\state\workspace-ops; D:\Repos\.openclaw\runtime\swarm\logs\workspace-ops

### 2026-04-03 | openclaw | Dashboard ganhou registry de portas/previews e serializacao HTML foi endurecida
- `D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1` passou a derivar `workspacePortRegistry` a partir do `workspaceCatalog`, combinando `preview`, `run`, `endpoint`, `listener`, `health`, `runStatus`, `runPid` e logs por repo.
- `D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html` ganhou o painel `Comandos > Portas e previews`, com renderer dedicado para cards de endpoint local por repo/source.
- A geracao de `D:\Repos\Relatorios\OpenClaw\dashboard.html` parou de depender de um `ConvertTo-Json` monolitico sobre todo o payload do dashboard; o HTML agora usa payload reduzido/truncado por blocos para evitar `OutOfMemoryException` e manter a pagina geravel no Windows PowerShell.
- Evidencia verificada em 2026-04-03:
  - `dashboard.html` regenerado com sucesso apos a reducao do payload
  - parse do JSON embutido confirmou `workspaceCatalog = 8`, `workspacePortRegistry = 10`, `taskCount = 15`
  - exemplos reais no registry: `ChamadoWebAPI:5000`, `ChamadoWebExterno:4174`, `ConveniosWebBNB_Novo:7122`
  - o artefato final contem `Portas e previews`, `workspace-port-grid`, `workspace-port-summary` e `renderWorkspacePortRegistry`
- Limitacao conhecida: o smoke visual final pelo `dashboard-bridge` ficou pendente porque o endpoint local deixou de responder durante a ultima rodada de validacao.
- Tags: openclaw, dashboard, workspace-ports, preview, serialization
- Fontes: D:\Repos\.openclaw\scripts\Invoke-OpenClawDashboard.ps1; D:\Repos\.openclaw\scripts\openclaw-dashboard.template.html; D:\Repos\Relatorios\OpenClaw\dashboard.html; D:\Repos\Relatorios\OpenClaw\superset-inspired-backlog-2026-04-03.md

### 2026-04-08 | openclaw | OpenSpec passou a alimentar o backlog manual do swarm por tag explicita
- O swarm do `.openclaw` passou a ter uma ponte repo-local para OpenSpec em `D:\Repos\.openclaw\runtime\swarm\Scripts\Sync-SwarmOpenSpecTasks.ps1`.
- O intake varre `openspec/changes/*/tasks.md` dos repositorios mapeados em `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` e sincroniza apenas itens unchecked marcados com `[swarm]` ou `[swarm:fast|medium|high]`.
- Cada item marcado vira task manual deduplicada via `Add-SwarmTask.ps1`, com `sourceRef` sob `manual://openspec/<repo>/<change>`, artefato canônico em `D:\Repos\Relatorios\Swarm\manual-intake\` e `sourceFingerprint` estavel por repo + change + texto da task.
- `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmScheduledJob.ps1` passou a rodar esse intake antes do planner (`Generate-SwarmTaskBacklog.ps1`), e o bloco `openSpecIntake` em `swarm-config.json` controla `enabled`, `failClosed`, `tag`, `sourcePrefix`, `defaultPriority`, `defaultExecutionProfile` e `chainTaggedTasks`.
- O intake trata `mutex_timeout:Global\OpenClawSwarmBacklogLock` como janela ocupada (`lockBusy`) em vez de erro fatal, para nao quebrar a rodada do planner quando outro job estiver usando o backlog.
- Quando varios itens `[swarm]` aparecem no mesmo `tasks.md`, o intake agora os encadeia automaticamente pela ordem do checklist usando `dependsOnTaskId`, `dependsOnTaskIds` e `chainInputFromTaskId`; a proxima task so fica elegivel depois da anterior fechar em estado satisfatorio do swarm.
- Esse encadeamento reaproveita a politica nativa do supervisor: retries automaticos ate `maxAttempts`, com satisfacao de dependencia quando a task anterior chega a `completed`, `review_pending` ou `awaiting_review`.
- As tasks sincronizadas de OpenSpec agora carregam metadados de plano (`openSpecPlanId`, `openSpecChangeName`, `openSpecTaskText`, `openSpecTaskOrder`, `openSpecTaskCount`) e o planner passou a derivar `openSpecPlans[]` em `task-backlog.json`, para que o criterio de sucesso observado para OpenSpec seja o change como um todo, e nao apenas uma task isolada.
- Cada entrada em `openSpecPlans[]` resume `status`, `expectedResult`, `expectedTaskCount`, `satisfiedTaskCount`, `failedTaskCount`, `blockedTaskCount`, `nextTaskId` e `taskIds`; o plano so conta como concluido quando todos os itens `[swarm]` do change ficam em estado satisfatorio.
- A ergonomia do fluxo ficou documentada em `D:\Repos\.openclaw\openspec\README.md`, `D:\Repos\.openclaw\runtime\swarm\README.md` e `D:\Repos\.openclaw\OPENCLAW-OPERATING-CONTRACT.md`; os prompts globais `C:\Users\l.sousa\.codex\prompts\opsx-propose.md` e `opsx-apply.md` passaram a orientar a marcacao `[swarm]` quando o repo participa do swarm local.
- Validacao real: um item existente em `D:\Repos\ConveniosWebBNB_Novo\openspec\changes\raise-api-coverage-near-100\tasks.md` foi marcado com `[swarm]`; `Sync-SwarmOpenSpecTasks.ps1 -Json` encontrou `taggedTasksFound = 1` e reaproveitou a task `cwbnn-015`; `D:\Repos\Relatorios\Swarm\task-backlog.json` registrou `sourceRef = manual://openspec/convenioswebbnb_novo/raise-api-coverage-near-100` e o artefato `D:\Repos\Relatorios\Swarm\manual-intake\cwbnn-015.md` foi materializado com o prompt do change.
- Validacao adicional da cadeia: os itens `2.2` e `2.3` do mesmo change foram marcados com `[swarm]`; o intake sincronizou `cwbnn-016` e `cwbnn-017`, e `D:\Repos\Relatorios\Swarm\task-backlog.json` passou a registrar `cwbnn-017.dependsOnTaskId = cwbnn-016`, `dependsOnTaskIds = [\"cwbnn-016\"]`, `chainInputFromTaskId = cwbnn-016` e `blockedReason = dependency_wait: pending dependency state(s): cwbnn-016:ready`.
- Validacao adicional da semantica de plano: o planner passou a expor em `task-backlog.json` o plano `openspec://convenioswebbnb_novo/raise-api-coverage-near-100` com `expectedResult = Concluir o change OpenSpec como um todo; o plano so fecha quando todos os itens [swarm] ficam satisfeitos.` e status observado `failed (0/2 itens satisfeitos)` quando a primeira task da cadeia falhou.
- O gerador do backlog tambem passou a classificar visualmente cada task por linha de trabalho em `Generate-SwarmTaskBacklog.ps1`, preenchendo `workstreamKind`, `workstreamLabel`, `workstreamGroupLabel` e o agregado `repoWorkstreams[]` para separar `Plano OpenSpec`, `Achados de auditoria`, `Chamados do ChamadoWeb` e `Intake manual`.
- O markdown planejado de `task-backlog.md` ganhou a secao `Repo workstreams` para repositorios com mais de uma linha de trabalho e uma linha `Linha de trabalho` dentro de cada task, para evitar confusao entre tasks de testes OpenSpec e findings de auditoria no mesmo repo.
- Validacao somente leitura no backlog atual confirmou, para `ConveniosWebBNB_Novo`, a separacao `Plano OpenSpec = cwbnn-018/cwbnn-019`, `Achados de auditoria = cwbnn-010..014` e `Chamados do ChamadoWeb = cwbnn-003/cwbnn-009`.
- Limitacao remanescente: a deduplicacao de tasks antigas de OpenSpec pelo mesmo `sourceFingerprint` ja foi implementada no planner, mas a projeção completa desse cleanup no backlog depende de uma rodada do planner sem disputa do mutex `Global\OpenClawSwarmBacklogLock`.
- Intervencao operacional concluida: a scheduled task `OpenClaw Swarm Supervisor Direct` estava relancando o supervisor a cada minuto e monopolizando o mutex do backlog; o planner foi forcado com `Disable-ScheduledTask`, encerramento da instancia corrente, execucao manual de `Generate-SwarmTaskBacklog.ps1` e `Enable-ScheduledTask` para restaurar a agenda.
- Validacao final: `Generate-SwarmTaskBacklog.ps1` retornou `SWARM_TASK_BACKLOG_OK`, `task-backlog.json` passou a expor `repoWorkstreams[]` e `workstreamKind/workstreamLabel/workstreamGroupLabel`, e `task-backlog.md` passou a mostrar `Repo workstreams` e `Linha de trabalho` por task.
- A cadencia da scheduled task `OpenClaw Swarm Supervisor Direct` foi elevada de 1 minuto para 10 minutos; a validacao via `Export-ScheduledTask` confirmou o trigger `<Interval>PT10M</Interval>` e a task permaneceu habilitada.
- `D:\Repos\.openclaw\scripts\Set-OpenClawScheduledTasksSilent.ps1` passou a carregar um helper para reescrever o intervalo de repeticao por XML quando a task do supervisor for reaplicada, e a correcao imediata no Scheduler foi aplicada com `schtasks /Change /TN "OpenClaw Swarm Supervisor Direct" /RI 10 /ENABLE`.
- O roteamento do Google specialist no swarm foi corrigido para nao capturar tasks OpenSpec por falso positivo: `D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1` agora so promove uma rota Google quando `matchedSignals.Count > 0`, e o route `browser_validation` passou a ignorar `filesHint` sob `openspec/` e arquivos `.md`.
- `D:\Repos\.openclaw\runtime\swarm\swarm-config.json` tambem foi endurecido: `browser_validation.filePatterns` deixou de usar o token generico `spec` e passou a mirar apenas sinais realmente de browser/e2e (`playwright`, `cypress`, `puppeteer`, `webdriver`, `e2e`, `*.spec.ts(x)`, `*.cy.ts(x)`), reduzindo drift de roteamento sobre artefatos OpenSpec.
- Validacao real em 2026-04-08: `Resolve-SwarmGoogleSpecialist` passou a retornar `eligible = false` para `cwbnn-018` e `cwbnn-019`; `Generate-SwarmTaskBacklog.ps1` reprojetou `task-backlog.json` com `googleSpecialistEligible = false`; `cwbnn-018` foi rearmada para `ready` e `Start-SwarmTaskDirect.ps1` voltou a subir a task em `attempt = 3`, com `Run-SwarmWorker.ps1 -> invoke-runner -> codex.js exec -m gpt-5.3-codex` ativo e sem repetir o erro `YOLO mode is enabled` do Gemini.
- O runtime do swarm tambem passou a persistir backlog com derivacao canônica de agregados: `D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1` ganhou `Save-SwarmBacklog`, que recalcula `openSpecPlans[]`, `repoWorkstreams[]` e `workstreamKind/workstreamLabel/workstreamGroupLabel` antes de escrever `task-backlog.json`.
- Essa mudança corrige um bug de consistência observado em 2026-04-08: tasks OpenSpec podiam mudar para `in_progress` ou `review_pending` por `Start-SwarmTaskDirect`, `Set-SwarmTaskState` ou `Invoke-SwarmSupervisor`, mas o plano agregado em `openSpecPlans[]` permanecia `ready` porque os scripts faziam `Write-JsonFile` direto sem recomputar agregados.
- Os scripts `Add-SwarmTask.ps1`, `Remove-SwarmTask.ps1`, `Retry-SwarmTask.ps1`, `Set-SwarmTaskState.ps1`, `Start-SwarmTaskDirect.ps1`, `Invoke-SwarmSupervisor.ps1` e `Sync-SwarmRuntimeState` passaram a usar `Save-SwarmBacklog`; validacao real: com `cwbnn-019` em andamento, `task-backlog.json` passou a refletir `inProgressTaskCount = 1` e `status = in_progress` no plano `openspec://convenioswebbnb_novo/raise-api-coverage-near-100`, e `task-backlog.md` renderizou o mesmo estado.
- Ajuste de governanca para tasks OpenSpec em cadeia:
  - itens OpenSpec intermediarios bem-sucedidos agora devem fechar em `completed`; so o item terminal do bloco/change fica em `review_pending` ou `awaiting_review`
  - a regra ficou centralizada em `D:\Repos\.openclaw\runtime\swarm\Scripts\Swarm-Common.ps1` por `Get-SwarmSuccessStatusForTask`, com uso no supervisor e no planner
  - `D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmSupervisor.ps1` tambem passou a reconciliar tasks OpenSpec intermediarias antigas que ainda estivessem em review, promovendo-as para `completed`
  - `D:\Repos\.openclaw\runtime\swarm\Scripts\Generate-SwarmTaskBacklog.ps1` passou a preservar tasks OpenSpec em `completed`, para que um item intermediario concluido continue contando no plano agregado e nao suma do backlog em reprojecoes
  - validacao real no change `raise-api-coverage-near-100` do `ConveniosWebBNB_Novo`: `cwbnn-018` ficou `completed`, `cwbnn-019` ficou `review_pending`, e `openSpecPlans[]` registrou o plano como `completed` com `2/2 itens satisfeitos`
- Continuidade de branch/worktree por plano OpenSpec:
  - o swarm deixou de tratar OpenSpec incremental como branches totalmente independentes por `taskId`; agora `Start-SwarmTaskDirect.ps1` e `Invoke-SwarmSupervisor.ps1` usam `Get-SwarmTaskExecutionTarget` para reutilizar a branch/worktree da etapa anterior do mesmo `openSpecPlanId`
  - sem branch anterior, o runtime deriva uma branch do proprio plano; com branch anterior, a etapa seguinte herda a branch/worktree ja usada, preservando o codigo incremental entre itens `[swarm]`
  - isso evita erro de produto em que a etapa seguinte perderia o resultado da etapa anterior por ter sido aberta em outra branch isolada
  - validacao em seco no plano `openspec://convenioswebbnb_novo/raise-api-coverage-near-100`: uma task sintetica `cwbnn-020` passou a resolver `strategy = openspec-plan-reuse`, `branch = agent/cwbnn-019` e `worktreePath = D:\WT\cwbnn\cwbnn-019`
- Tags: openclaw, openspec, swarm, backlog, automation
- Fontes: D:\Repos\.openclaw\runtime\swarm\Scripts\Sync-SwarmOpenSpecTasks.ps1; D:\Repos\.openclaw\runtime\swarm\Scripts\Invoke-SwarmScheduledJob.ps1; D:\Repos\.openclaw\runtime\swarm\swarm-config.json; D:\Repos\.openclaw\openspec\README.md; D:\Repos\.openclaw\runtime\swarm\README.md; D:\Repos\.openclaw\OPENCLAW-OPERATING-CONTRACT.md; D:\Repos\ConveniosWebBNB_Novo\openspec\changes\raise-api-coverage-near-100\tasks.md; D:\Repos\Relatorios\Swarm\task-backlog.json; D:\Repos\Relatorios\Swarm\manual-intake\cwbnn-015.md

- `2026-04-08`: a branch canonica do trabalho incremental de testes do `ConveniosWebBNB_Novo` passou a ser `agent/cwbnn-001-saneamento-texto-corrompido`, agora fast-forwardada para conter os commits OpenSpec `cwbnn-018` e `cwbnn-019` mais um fix de compilacao (`e21e194`). A unificacao foi feita primeiro em `agent/cwbnn-001-saneamento-texto-corrompido-unify-test` e so depois promovida para a branch original, preservando alteracoes locais nao relacionadas em `Backend/.sonarqube/`.
- `2026-04-08`: depois da unificacao da linha OpenSpec de testes do `ConveniosWebBNB_Novo`, o estado canonico ficou consolidado em `origin/agent/cwbnn-001-saneamento-texto-corrompido` (`e21e194`). As branches/worktrees antigas `agent/cwbnn-018` e `agent/cwbnn-019` foram removidas localmente e no remoto, e o backlog do swarm para `cwbnn-018`/`cwbnn-019` foi reescrito para apontar para a branch/worktree canonicas em vez das worktrees descartadas.
- `2026-04-08`: a task agendada `OpenClaw Swarm Supervisor Direct` voltou para cadencia de 2 minutos (`PT2M`) e permaneceu habilitada. A origem canonica dessa definicao fica em `D:\Repos\.openclaw\scripts\Set-OpenClawScheduledTasksSilent.ps1`.
- `2026-04-08`: o change OpenSpec `ConveniosWebBNB_Novo/raise-api-coverage-near-100` foi expandido para execucao integral no swarm: todos os itens pendentes relevantes passaram a usar `[swarm]`, o intake materializou a cadeia `cwbnn-013` + `cwbnn-020..038`, e o planner passou a contar corretamente itens historicos ja satisfeitos do mesmo plano ao calcular `openSpecPlans[].expectedTaskCount`.
- `2026-04-08`: a task de auditoria `cwbnn-014` foi consolidada na branch canonica `agent/cwbnn-001-saneamento-texto-corrompido`. O remoto ja continha commits equivalentes (`33b1c5f`, `e9b2f66`), entao a branch local foi apenas alinhada via `git reset --keep` para preservar a alteracao local do `tasks.md` do OpenSpec.
- `2026-04-08`: apos a consolidacao da `cwbnn-014`, a branch antiga `agent/cwbnn-014` e sua worktree `D:\WT\cwbnn\cwbnn-014` foram removidas localmente, `origin/agent/cwbnn-014` foi apagada, e o backlog ativo do swarm deixou de apontar para a branch/worktree removidas, passando a referenciar `agent/cwbnn-001-saneamento-texto-corrompido` para o contexto vivo da task.
- `2026-04-08`: duas falhas artificiais do backlog foram reconciliadas sem retrabalho real: `cwbnn-013` caiu por `worktreePath` corrompido e precisou ser rearmada com `D:\Repos\ConveniosWebBNB_Novo`; `cwbnn-017` tinha resultado bem-sucedido gravado, mas ficou `failed` por `runtime_process_missing` e por um artefato inconsistente (`workerResult.success = true` com topo `success = false`). O backlog e o artefato de resultado foram corrigidos, eliminando `failedCount` residual.
- `2026-04-08`: o swarm ganhou auto-heal canônico para falhas artificiais do backlog. `D:\Repos\.openclaw\runtime\swarm\Scripts\Repair-SwarmTaskFailures.ps1` chama `Invoke-SwarmAutoHealBacklog` de `Swarm-Common.ps1` para reconciliar tasks cujo `state/results/<taskId>.json` já prova sucesso e para rearmar tasks com `worktreePath` inválido/mojibake. O supervisor também passou a acionar esse auto-heal no início de cada rodada.
- `2026-04-08`: a causa raiz de um no-op no auto-heal OpenSpec foi um wrapper de array em `Get-SwarmRepairWorktreePath` (`@(... | Select-Object -First 1)`), que fazia `Get-ObjectStringValue` devolver vazio ao ler peers do mesmo plano. Depois da correção, o caso real `cwbnn-013` foi curado automaticamente e o plano `openspec://convenioswebbnb_novo/raise-api-coverage-near-100` voltou a `ready`.
- `2026-04-16`: o dashboard operacional do OpenClaw passou a publicar e renderizar explicitamente apenas `activeFeatures`; cards e indicadores de `Telegram`, `Chamados API`, `auditoria automatica`, `notificacoes` e `email` deixam de aparecer quando essas funcionalidades estao desligadas no `swarm-config.json`, e a aba de repositorios mostra `auditoria desativada` em vez de cobrar `autonomous-code-audit.md`.
