# DunderIA

### Runtime local-first de escritorio multiagente.

O DunderIA te da um escritorio visivel para agentes de IA: canais compartilhados, broker local, runners novos por turno, MCP com escopo por agente, worktrees Git isoladas e uma interface onde o trabalho fica exposto em vez de escondido atras de um unico chat.

<p align="center">
  <img src="https://raw.githubusercontent.com/luiztrilha/dunderia/main/assets/hero.png" alt="DunderIA onboarding - Seu time de IA, visivel e trabalhando." width="720" />
</p>

[![npm](https://img.shields.io/npm/v/wuphf?color=A87B4F)](https://www.npmjs.com/package/wuphf)
[![Discord](https://img.shields.io/badge/Discord-Join%20Community-5865F2?logo=discord&logoColor=white)](https://discord.gg/gjSySC3PzV)
[![License: MIT](https://img.shields.io/badge/License-MIT-A87B4F)](https://github.com/luiztrilha/dunderia/blob/main/LICENSE)

[▶ Teaser e walkthrough completos no GitHub](https://github.com/luiztrilha/dunderia#readme)

## Comecando

Escolha um provider suportado:

- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) e o padrao
- [Codex CLI](https://github.com/openai/codex) com `--provider codex`
- [Gemini](https://ai.google.dev/) com `--provider gemini`
- [Gemini on Vertex AI](https://cloud.google.com/vertex-ai/generative-ai/docs/start/quickstarts/quickstart-multimodal) com `--provider gemini-vertex`
- [Ollama](https://ollama.com/) com `--provider ollama` e um modelo local ja baixado

`tmux` so e necessario para `--tui`.

```bash
npx wuphf
```

Isso sobe o escritorio local e abre a interface web automaticamente.

Prefere instalacao global?

```bash
npm install -g wuphf
wuphf
```

O wrapper de `npm` baixa o binario nativo sob demanda em macOS e Linux (`x64` / `arm64`). No Windows, faca build por fonte a partir do repositório:

Pacotes de desenvolvimento com versao `0.0.0` nao tentam baixar uma release inexistente. Para testar contra uma release publicada, defina `WUPHF_RELEASE_VERSION` com a tag sem o `v`, por exemplo `WUPHF_RELEASE_VERSION=1.2.3`.

```bash
git clone https://github.com/luiztrilha/dunderia.git
cd dunderia
go build -o wuphf ./cmd/wuphf
./wuphf
```

No Windows PowerShell:

```powershell
go build -o wuphf.exe ./cmd/wuphf
.\wuphf.exe
```

Se quiser salvar os padroes antes do primeiro uso:

```bash
wuphf init
```

Se o cloud backup ja estiver configurado e acessivel, `wuphf init` tambem reidrata e reespelha o estado local leve nao sensivel da maquina, como `company.json`, `onboarded.json` e `cloud-backup-bootstrap.json`. Credenciais, tokens, arquivos de auth e configs locais com segredos devem ser recriados no novo ambiente, nao restaurados de backup generico. Repositorios locais pesados continuam fora desse escopo.

## Flags Mais Importantes

| Flag | O que faz |
|---|---|
| `--provider <name>` | Sobrescreve o provider de runtime: `claude-code`, `codex`, `gemini`, `gemini-vertex`, `ollama` |
| `--blueprint <id>` | Inicia a partir de um blueprint operacional |
| `--pack <id>` | Alias legado para selecao de blueprint |
| `--from-scratch` | Ignora a configuracao salva e sintetiza uma nova operacao |
| `--1o1` | Inicia uma sessao 1:1 com um agente especifico |
| `--tui` | Sobe a TUI em `tmux` em vez da interface web |
| `--no-open` | Nao abre o navegador automaticamente |
| `--broker-port <n>` | Define a porta local do broker |
| `--web-port <n>` | Define a porta da interface web |
| `--threads-collapsed` | Inicia a interface web com threads recolhidas |
| `--memory-backend none` | Usa o unico modo de memoria organizacional suportado hoje |
| `--opus-ceo` | Troca o CEO de Sonnet para Opus |
| `--collab` | Inicia em modo colaborativo |
| `--unsafe` | Ignora checagens de permissao para desenvolvimento local |
| `--cmd <cmd>` | Executa um comando sem interacao |

## Comandos

```bash
wuphf init
wuphf shred
wuphf import --from legacy
wuphf log
wuphf log <taskID>
wuphf repair-channel-memory
```

- `wuphf init`: setup inicial, salvamento de padroes e restore/sync do estado local leve nao sensivel quando o cloud backup estiver configurado
- `wuphf shred`: limpa o estado do escritorio e reabre o onboarding no proximo boot
- `wuphf import --from legacy`: importa estado de um orquestrador externo ou de um arquivo exportado
- `wuphf log`: mostra os recibos de tarefa
- `wuphf repair-channel-memory`: reconstrui a memoria de canais a partir do historico do broker

## Memoria e Recuperacao

O DunderIA atualmente opera com memoria organizacional apenas local:

```bash
wuphf --memory-backend none
```

O contexto duravel fica em:

- historico de canais
- recibos de tarefa via `wuphf log`
- historico salvo do broker para `wuphf repair-channel-memory`
- retomada de trabalho inacabado apos reinicio
- notas privadas por agente

## Integracoes

- Telegram via `/connect`
- `one` para acoes locais via CLI
- `composio` para conexoes hospedadas e OAuth

## Links

- GitHub: https://github.com/luiztrilha/dunderia
- Issues: https://github.com/luiztrilha/dunderia/issues
- Architecture: https://github.com/luiztrilha/dunderia/blob/main/ARCHITECTURE.md
- Development: https://github.com/luiztrilha/dunderia/blob/main/DEVELOPMENT.md
- Forking: https://github.com/luiztrilha/dunderia/blob/main/FORKING.md
- Security: https://github.com/luiztrilha/dunderia/blob/main/SECURITY.md
- Contributing: https://github.com/luiztrilha/dunderia/blob/main/CONTRIBUTING.md
- Code of Conduct: https://github.com/luiztrilha/dunderia/blob/main/CODE_OF_CONDUCT.md
- Support: https://github.com/luiztrilha/dunderia/blob/main/SUPPORT.md

O nome publico do produto e **DunderIA**. O codinome tecnico historico **`wuphf`** continua no binario e no pacote npm por compatibilidade.
