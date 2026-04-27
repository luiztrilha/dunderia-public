param(
    [Parameter(Mandatory = $true)]
    [string]$Mode,
    [Parameter(Mandatory = $true)]
    [string]$Text,
    [string]$SourceLabel = 'texto'
)

$ErrorActionPreference = 'Stop'

switch ($Mode) {
    'literal' {
        @"
Voce recebera $SourceLabel.
Resuma em portugues do Brasil, de forma objetiva e literal.
Use somente o que estiver explicitamente presente no texto.
Nao interprete, nao complete lacunas, nao explique o contexto, nao corrija nomes, nao infira significado.
Se o texto estiver curto, ruidoso ou ambiguo, diga isso explicitamente no resumo.
Responda em JSON valido com exatamente as chaves:
- summary
- action_items
- key_points

Regras:
- `summary`: 1 ou 2 frases curtas, estritamente ancoradas no texto.
- `action_items`: apenas itens explicitamente pedidos no texto; se nao houver, use `[]`.
- `key_points`: de 1 a 5 pontos literais curtos extraidos do texto; se o texto for muito fraco, use `[]`.

Texto:
$Text
"@
        break
    }
    'action-items' {
        @"
Voce recebera $SourceLabel.
Extraia somente pedidos, decisoes operacionais, tarefas ou proximos passos explicitamente mencionados.
Nao invente acao implicita. Nao deduza prioridade. Nao reescreva com contexto externo.
Responda em JSON valido com exatamente as chaves:
- summary
- action_items
- key_points

Regras:
- `summary`: uma frase curta dizendo se ha ou nao acoes explicitas.
- `action_items`: lista de strings com as acoes explicitamente presentes; se nao houver, use `[]`.
- `key_points`: lista curta com decisoes ou fatos operacionais explicitamente citados; se nao houver, use `[]`.

Texto:
$Text
"@
        break
    }
    'classification' {
        @"
Voce recebera $SourceLabel.
Classifique o conteudo apenas com base no texto fornecido.
Nao use conhecimento externo. Nao complete lacunas.
Responda em JSON valido com exatamente as chaves:
- summary
- action_items
- key_points

Regras:
- `summary`: uma frase curta descrevendo o tipo aparente de conteudo.
- `action_items`: use `[]`, a menos que exista pedido explicito.
- `key_points`: ate 5 rotulos ou fatos literais curtos observaveis no texto.

Texto:
$Text
"@
        break
    }
    default {
        throw "Modo de resumo invalido: $Mode"
    }
}
