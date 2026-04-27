# STANDARDS-INDEX.md

Mapa de navegacao do workspace `D:\Repos`.

Objetivo:

- facilitar descoberta do contrato certo sem criar uma segunda fonte de verdade
- apontar para a regra, memoria ou runtime ja existentes
- reduzir a tentacao de espalhar governanca duplicada em novos arquivos

Regra principal:

- este arquivo e um indice
- a fonte de verdade continua nos arquivos e diretorios apontados abaixo

## Leitura base do workspace

1. `AGENTS.md`
2. `SOUL.md`
3. `USER.md`
4. `MEMORY.md`
5. `TOOLS.md`
6. `HEARTBEAT.md`

Observacao:

- a memoria diaria continua em `memory/YYYY-MM-DD.md`
- o runtime ativo do ambiente e `codex` + integracoes do `dunderia`
- o legado do OpenClaw fica somente em `_archive/openclaw-decom-2026-04-19/`

## Mapa por assunto

### Comportamento e limites

- `SOUL.md`
- `AGENTS.md`

Use quando:

- houver duvida sobre autonomia, tom, seguranca, honestidade operacional ou permissao de acao

### Contexto humano e memoria

- `USER.md`
- `MEMORY.md`
- `memory/`

Use quando:

- precisar recuperar contexto recorrente, decisoes duraveis ou fatos recentes do workspace

### Atalhos, caminhos e comandos

- `TOOLS.md`

Use quando:

- a pergunta for operacional
- houver necessidade de localizar script, artefato, relatorio, repo ou comando recorrente

### Heartbeat e checagens leves

- `HEARTBEAT.md`
- `heartbeat-state.json`

Use quando:

- a sessao for de manutencao leve
- o pedido for um heartbeat
- for preciso decidir qual check minimo rodar

### Runtime ativo

- `C:\Users\l.sousa\.codex\config.toml`
- `D:\Repos\dunderia\mcp\dunderia-mcp-settings.json`
- `D:\Repos\dunderia\scripts\`

Use quando:

- o assunto for MCP, Playwright, GitHub MCP, Brave Search, Local AI ou configuracao ativa do Codex CLI

### Regras por repositorio

- `<repo>/AGENTS.md`
- `<repo>/openspec/` quando houver piloto OpenSpec

Repositorios principais:

- `ConveniosWebBNB`
- `ConveniosWebBNB_Novo`
- `ConveniosWebExterno`
- `ConveniosWebVSAzure_Default`
- `ConveniosWebData`
- `SuperPowers`
- `TectrilhaAPI`
- `SistemasCompartilhadosWebForms`
- `TransparenciaWeb`
- `dunderia`

Use quando:

- houver alteracao de codigo, validacao, branch, teste ou contrato especifico do repositorio

### Legado arquivado

- `_archive/openclaw-decom-2026-04-19/`

Use quando:

- precisar recuperar evidencia historica do OpenClaw
- precisar consultar scripts, contratos, relatorios ou o `knowledge.db` arquivado
- precisar portar algum conhecimento antigo para o fluxo atual

## Roteamento pratico minimo

### Pedido de codigo em repo conhecido

Fonte principal:

- `AGENTS.md` do workspace
- `AGENTS.md` do repo alvo

Acao esperada:

- abrir sessao direta no repo alvo ou worktree
- nao usar `D:\Repos` como diretoria de implementacao pesada

### Debug, regressao, teste quebrado ou causa raiz incerta

Fonte principal:

- skill `systematic-debugging-lite`

Complementos:

- `TOOLS.md`
- artefatos do repo alvo

### Review, auditoria ou busca de achados

Fonte principal:

- skill `code-review-findings`

### Planejamento curto ou tarefa ambigua

Fonte principal:

- skill `implementation-planning-lite`

### Encerramento com evidencia

Fonte principal:

- skill `verification-before-close`

### OpenSpec

Fonte principal:

- `<repo>/openspec/`
- prompts globais `C:\Users\l.sousa\.codex\prompts\opsx-*.md`

## Sinal de custo no workspace

Se `D:\Repos` estiver concentrando sessoes de codigo pesado:

- mover a proxima sessao para o repo alvo ou para worktree
- manter `D:\Repos` para triagem, memoria, configuracao e operacao local

## O que este indice nao faz

- nao substitui `AGENTS.md`
- nao substitui `SOUL.md`
- nao substitui `TOOLS.md`
- nao cria novos procedimentos se os arquivos existentes ja cobrem o caso

## Criterio para crescer

Adicionar nova entrada aqui apenas quando:

- um padrao recorrente ja existir em arquivo canonico e estiver dificil de localizar
- um novo runtime, skill ou contrato operacional virar capacidade estavel do workspace

Nao adicionar:

- duplicata de regra
- tutorial longo
- checklist que ja exista em outro lugar
