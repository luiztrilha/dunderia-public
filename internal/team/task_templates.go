package team

import (
	"encoding/json"
	"net/http"
)

type taskTemplate struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Description       string   `json:"description,omitempty"`
	TaskType          string   `json:"task_type,omitempty"`
	ExecutionMode     string   `json:"execution_mode,omitempty"`
	Outcome           string   `json:"outcome,omitempty"`
	PlanContent       string   `json:"plan_content,omitempty"`
	ArtifactKinds     []string `json:"artifact_kinds,omitempty"`
	MaxAttempts       int      `json:"max_attempts,omitempty"`
	MaxRuntimeMinutes int      `json:"max_runtime_minutes,omitempty"`
	MaxCostCents      int      `json:"max_cost_cents,omitempty"`
}

var builtInTaskTemplates = []taskTemplate{
	{
		ID:            "code-change",
		Title:         "Mudanca de codigo com validacao",
		Description:   "Implementar uma mudanca em workspace local com plano, build/teste e artefato verificavel.",
		TaskType:      "feature",
		ExecutionMode: "local_worktree",
		Outcome:       "Patch implementado, validado por comando local e registrado com artefato de evidencia.",
		PlanContent:   "1. Mapear arquivos afetados\n2. Implementar menor mudanca segura\n3. Rodar build/teste relevante\n4. Registrar evidencia e riscos restantes",
		ArtifactKinds: []string{"build", "pull_request", "document"},
		MaxAttempts:   3,
	},
	{
		ID:                "external-action",
		Title:             "Acao externa governada",
		Description:       "Executar uma acao em sistema conectado com trilha de decisao e evidencia.",
		TaskType:          "follow_up",
		ExecutionMode:     "external_workspace",
		Outcome:           "Acao externa executada uma vez, com destino, resultado e evidencia registrados.",
		PlanContent:       "1. Confirmar sistema e escopo\n2. Executar a menor acao necessaria\n3. Registrar link, recibo ou retorno do sistema\n4. Pedir revisao humana se houver mutacao sensivel",
		ArtifactKinds:     []string{"link", "decision", "document"},
		MaxAttempts:       2,
		MaxRuntimeMinutes: 30,
	},
	{
		ID:            "research-brief",
		Title:         "Pesquisa com recomendacao",
		Description:   "Investigar uma decisao e devolver recomendacao objetiva com fontes/evidencias.",
		TaskType:      "research",
		ExecutionMode: "office",
		Outcome:       "Resumo com recomendacao, alternativas rejeitadas e evidencias suficientes para decidir.",
		PlanContent:   "1. Definir pergunta\n2. Coletar evidencias relevantes\n3. Comparar opcoes\n4. Registrar recomendacao e incertezas",
		ArtifactKinds: []string{"document", "decision"},
		MaxAttempts:   2,
	},
	{
		ID:            "incident-followup",
		Title:         "Follow-up de bloqueio/incidente",
		Description:   "Destravar um bloqueio existente com owner claro, causa e proximo passo.",
		TaskType:      "incident",
		ExecutionMode: "office",
		Outcome:       "Bloqueio triado, responsavel definido e proximo passo publicado no canal correto.",
		PlanContent:   "1. Identificar causa provavel\n2. Separar decisao humana de acao tecnica\n3. Reatribuir ou pedir resposta\n4. Registrar resultado no board",
		ArtifactKinds: []string{"decision", "document"},
		MaxAttempts:   2,
	},
	{
		ID:                "release-pr-checklist",
		Title:             "Release/PR com contrato forte",
		Description:       "Preparar uma entrega revisavel com diff limpo, testes declarados, risco explicito e plano de rollback.",
		TaskType:          "release",
		ExecutionMode:     "local_worktree",
		Outcome:           "PR ou pacote de release pronto para revisao, com checks executados, evidencias anexadas, riscos e rollback registrados.",
		PlanContent:       "1. Confirmar branch, escopo e arquivos tocados\n2. Rodar build/testes relevantes e registrar saida\n3. Revisar diff para lixo temporario, segredos, logs e artefatos gerados\n4. Preparar resumo de PR/release com risco, rollback e validacao\n5. Publicar ou entregar somente com evidencia anexada",
		ArtifactKinds:     []string{"build", "pull_request", "release_note", "document"},
		MaxAttempts:       2,
		MaxRuntimeMinutes: 45,
	},
}

func (b *Broker) handleTaskTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	templates := append([]taskTemplate(nil), builtInTaskTemplates...)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"templates": templates})
}
