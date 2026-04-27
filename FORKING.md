# Forking DunderIA

Guia direto para adaptar o DunderIA sem herdar branding, release path ou documentacao quebrada.

Antes de mexer, leia o [ARCHITECTURE.md](ARCHITECTURE.md). Ele mostra onde o runtime realmente vive.

## 0. Fork por tag, nao por `main`

`main` continua mudando rapido. Se a ideia e publicar seu fork para terceiros, parta de uma tag.

```bash
git clone https://github.com/luiztrilha/dunderia.git
cd dunderia
git checkout v0.1.0   # exemplo
git checkout -b meu-fork
```

## 1. Comece da baseline local-first atual

Hoje, o unico backend de memoria organizacional suportado e:

```bash
./wuphf --memory-backend none
```

Isso significa:

- persistencia local do broker
- historico de canais
- recibos de tarefa
- notas privadas por agente
- snapshots de cloud backup, se configurados

Nao existe memoria organizacional compartilhada externa ativa por padrao neste momento.

## 2. Integracoes opcionais

O nucleo do produto e local. Nex, Telegram, One e Composio devem ser tratados como integracoes opcionais.

Se quiser remover Nex do seu fork por completo, os pontos principais continuam aqui:

```bash
# Nex API client
rm internal/action/nex_client.go

# Nex launcher hooks / compatibility paths
# - internal/team/launcher_nex.go
# - buscas por: nex-mcp, ResolveNoNex, WUPHF_NO_NEX, --no-nex
```

Depois ajuste as referencias restantes em:

- `cmd/wuphf/main.go`
- `cmd/wuphf/channel.go`
- `internal/config/config.go`
- `internal/team/launcher.go`
- `internal/team/headless_codex.go`

Se a sua intencao nao e remover Nex, mantenha a integracao como opcional e documente isso com clareza.

## 3. Nome do produto vs codinome tecnico

O nome publico atual e **DunderIA**. O codinome historico **`wuphf`** ainda aparece em:

- binario e pasta `cmd/wuphf/`
- modulo Go em `go.mod`
- pacote npm `wuphf`
- nomes de release e scripts de instalacao

Se voce quiser renomear tudo no seu fork, os arquivos centrais sao:

- `go.mod`
- `cmd/wuphf/`
- `.goreleaser.yml`
- `scripts/install.sh`
- `npm/package.json`
- `npm/scripts/download-binary.js`

Se tambem for renomear o modulo Go, reescreva os imports em lote:

```bash
find . -name '*.go' | xargs sed -i 's|github.com/nex-crm/wuphf|github.com/seu-org/seu-repo|g'
```

## 4. Branding e copy

Se voce for vender ou redistribuir o projeto com outra identidade, os pontos mais visiveis para limpar primeiro sao:

- `README.md`
- `npm/README.md`
- `web/index.html`
- `web/src/i18n/locales/`
- `cmd/wuphf/channel.go`
- `cmd/wuphf/channel_render.go`
- `internal/team/template.go`

Uma busca util:

```bash
rg -n "DunderIA|WUPHF|Nex|The Office|Scranton|Michael|Ryan" ./cmd ./internal ./web ./npm
```

## 5. Blueprints e presets

Para adaptar o comportamento do escritorio:

- blueprints operacionais: `templates/operations/`
- blueprints de especialistas e colaboradores: `templates/employees/`
- presets legados de compatibilidade: `internal/agent/packs.go`

Se o seu fork for produto e nao so experimento, prefira blueprints e templates antes de mexer no broker.

## 6. Release do seu fork

Antes de publicar um release do seu fork, ajuste estes caminhos:

- `.goreleaser.yml`
- `scripts/install.sh`
- `npm/package.json`
- `npm/scripts/download-binary.js`

Depois:

```bash
git tag v0.1.0
goreleaser release --clean
```

## 7. O que e caro de mudar

- Broker push-driven em `internal/team/broker.go`
- sessoes novas por turno nos headless runners
- isolamento por git worktree

Se voce remove qualquer um desses tres, esta construindo outro produto.

## Se travar

- Issues: https://github.com/luiztrilha/dunderia/issues
- Discord: veja o badge no [README.md](README.md)
- `CHANGELOG.md`: historico do que realmente foi entregue
