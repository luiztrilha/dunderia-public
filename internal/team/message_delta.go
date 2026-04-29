package team

import (
	"regexp"
	"sort"
	"strings"
)

var (
	blockedNoDeltaPathPattern = regexp.MustCompile(`(?i)[a-z]:\\[^\s"'` + "`" + `,;)\]]+`)
	blockedNoDeltaTaskPattern = regexp.MustCompile(`(?i)#?task-\d+`)
	blockedNoDeltaMentionPat  = regexp.MustCompile(`(?i)@[a-z0-9][a-z0-9_-]*`)
	coordinationTokenPattern  = regexp.MustCompile(`(?i)\b[a-z][a-z0-9_]{3,}\b`)
	taskLaneTokenPattern      = regexp.MustCompile(`(?i)\b[a-z][a-z0-9_]{5,}\b`)
	taskClaimTaskPattern      = regexp.MustCompile(`(?i)\btask-\d+\b`)
)

var blockedNoDeltaPhrases = []string{
	"sem delta",
	"no delta",
	"sigo bloqueado",
	"continua bloqueado",
	"continua bloqueada",
	"bloqueio continua",
	"still blocked",
	"continues blocked",
	"workspace incorreto",
	"blocked by infrastructure",
	"bloqueado por infraestrutura",
	"blocked by environment",
	"bloqueado por ambiente",
}

var awaitingHumanInputPhrases = []string{
	"awaiting human",
	"waiting for human",
	"human input",
	"aguardando o human",
	"aguardando o @human",
	"aguardando input do human",
	"aguardando input do @human",
	"aguardando o repositorio",
	"aguardando o repositório",
	"aguardando o repo",
	"preciso do caminho do repo",
	"preciso do caminho do repositorio",
	"preciso do caminho do repositório",
	"preciso do arquivo exato",
	"preciso do arquivo",
	"preciso do modulo",
	"preciso do módulo",
	"falta o alvo",
	"sem coordenada",
	"sem caminho",
	"mande o repositorio",
	"mande o repositório",
	"manda o repo",
	"manda o caminho",
}

var taskLaneStopWords = map[string]struct{}{
	"ajustar": {}, "api": {}, "backend": {}, "builder": {}, "canal": {}, "cidades": {},
	"com": {}, "compatibilidade": {}, "compatível": {}, "controller": {}, "controllers": {},
	"convenioswebbnb": {}, "criar": {}, "corte": {}, "coverage": {}, "cobertura": {},
	"detalhes": {}, "endpoint": {}, "externo": {}, "feature": {}, "filtro": {},
	"general": {}, "implementacao": {}, "implementar": {}, "lane": {},
	"legado": {}, "local_worktree": {}, "mapper": {}, "novo": {}, "office": {},
	"payload": {}, "pipeline": {}, "repo": {}, "review": {}, "rota": {}, "service": {},
	"slice": {}, "task": {}, "testes": {}, "teste": {}, "tipo": {}, "uf": {},
	"workspace": {}, "worktree": {},
}

var coordinationGuidanceKeywords = []string{
	"gate", "review", "revis", "criter", "diff", "workspace", "worktree",
	"blocked", "bloque", "destrav", "semant", "compat", "arquivo",
	"storagecompat", "datalength", "bnbinterno",
}

var coordinationGuidanceStopWords = map[string]struct{}{
	"agora": {}, "ainda": {}, "assim": {}, "arquivo": {}, "builder": {}, "ceo": {},
	"cirurgico": {}, "claro": {}, "continua": {}, "continuo": {}, "criterio": {},
	"criterios": {}, "corte": {}, "cortes": {}, "depois": {}, "diff": {}, "direto": {},
	"entao": {}, "essa": {}, "esse": {}, "estao": {}, "estar": {}, "feito": {},
	"fecho": {}, "gate": {}, "imediato": {}, "in_progress": {}, "mesmo": {},
	"minha": {}, "neste": {}, "nos": {}, "nosso": {}, "para": {}, "parte": {},
	"perfeito": {}, "pontos": {}, "porque": {}, "posto": {}, "pronto": {},
	"quando": {}, "quatro": {}, "review": {}, "revisao": {}, "segue": {},
	"segundo": {}, "sem": {}, "slice": {}, "task": {}, "thread": {}, "trio": {},
	"workspace": {},
}

func messageLooksLikeBlockedNoDeltaUpdate(content string) bool {
	content = strings.ToLower(strings.TrimSpace(content))
	if content == "" || !messageIsSubstantiveOfficeContent(content) {
		return false
	}
	for _, phrase := range blockedNoDeltaPhrases {
		if strings.Contains(content, phrase) {
			return true
		}
	}
	return strings.Contains(content, "bloque") && (strings.Contains(content, "worktree") || messageLooksLikeAwaitingHumanInput(content))
}

func messageLooksLikeAwaitingHumanInput(content string) bool {
	normalized := normalizeCoordinationText(content)
	if normalized == "" || !messageIsSubstantiveOfficeContent(content) {
		return false
	}
	for _, phrase := range awaitingHumanInputPhrases {
		if strings.Contains(normalized, normalizeCoordinationText(phrase)) {
			return true
		}
	}
	if strings.Contains(normalized, "preciso de") || strings.Contains(normalized, "need ") {
		if containsAnyNormalizedFragment(normalized,
			"repo", "repositorio", "repositório", "path", "caminho", "arquivo", "file", "classe", "class", "modulo", "módulo", "alvo", "target",
		) {
			return true
		}
	}
	return false
}

func normalizeBlockedNoDeltaSignature(content string) string {
	content = strings.ToLower(strings.TrimSpace(content))
	if content == "" {
		return ""
	}
	content = blockedNoDeltaPathPattern.ReplaceAllString(content, "<path>")
	content = blockedNoDeltaTaskPattern.ReplaceAllString(content, "<task>")
	content = blockedNoDeltaMentionPat.ReplaceAllString(content, "<mention>")
	content = strings.NewReplacer("`", " ", "\"", " ", "'", " ", ".", " ", ",", " ", ":", " ", ";", " ", "(", " ", ")", " ").Replace(content)
	return strings.Join(strings.Fields(content), " ")
}

func messageLooksLikeCoordinationGuidance(content string) bool {
	content = normalizeCoordinationText(content)
	if content == "" || !messageIsSubstantiveOfficeContent(content) {
		return false
	}
	for _, keyword := range coordinationGuidanceKeywords {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

func messageLooksLikeOperationalRollCall(content string) bool {
	normalized := normalizeCoordinationText(content)
	if normalized == "" || !messageIsSubstantiveOfficeContent(content) {
		return false
	}
	hasStatusTopic := containsAnyNormalizedFragment(normalized,
		"online", "status operacional", "status operacion", "status", "disponibilidade", "disponivel", "disponivel",
	)
	if !hasStatusTopic {
		return false
	}
	hasImperative := containsAnyNormalizedFragment(normalized,
		"confirme", "confirmem", "reporte", "reportem", "responda", "respondam", "reporte seu status", "confirmem o status",
	)
	return hasImperative
}

func normalizeCoordinationGuidanceSignature(content string) string {
	tokens := coordinationGuidanceTokens(content)
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, "|")
}

func coordinationGuidanceTokens(content string) []string {
	content = normalizeCoordinationText(content)
	if content == "" {
		return nil
	}
	content = blockedNoDeltaPathPattern.ReplaceAllString(content, "<path>")
	content = blockedNoDeltaTaskPattern.ReplaceAllString(content, "<task>")
	content = blockedNoDeltaMentionPat.ReplaceAllString(content, "<mention>")
	raw := coordinationTokenPattern.FindAllString(content, -1)
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	tokens := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, skip := coordinationGuidanceStopWords[token]; skip {
			continue
		}
		if len(token) < 5 && token != "false" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	if len(tokens) == 0 {
		return nil
	}
	sort.Slice(tokens, func(i, j int) bool {
		if len(tokens[i]) != len(tokens[j]) {
			return len(tokens[i]) > len(tokens[j])
		}
		return tokens[i] < tokens[j]
	})
	if len(tokens) > 8 {
		tokens = tokens[:8]
	}
	sort.Strings(tokens)
	return tokens
}

func coordinationGuidanceEquivalent(left, right string) bool {
	leftTokens := coordinationGuidanceTokens(left)
	rightTokens := coordinationGuidanceTokens(right)
	if len(leftTokens) < 4 || len(rightTokens) < 4 {
		return false
	}
	rightSet := make(map[string]struct{}, len(rightTokens))
	for _, token := range rightTokens {
		rightSet[token] = struct{}{}
	}
	shared := 0
	for _, token := range leftTokens {
		if _, ok := rightSet[token]; ok {
			shared++
		}
	}
	shorter := len(leftTokens)
	if len(rightTokens) < shorter {
		shorter = len(rightTokens)
	}
	return shared*10 >= shorter*7
}

func normalizeCoordinationText(content string) string {
	content = strings.ToLower(strings.TrimSpace(content))
	if content == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "ô", "o", "õ", "o",
		"ú", "u",
		"ç", "c",
	)
	return replacer.Replace(content)
}

type taskStateClaim struct {
	TaskIDs           []string
	ClaimsUnblocked   bool
	ClaimsInProgress  bool
	ClaimsReviewReady bool
	ClaimsDone        bool
}

func parseTaskStateClaim(content string) taskStateClaim {
	normalized := normalizeCoordinationText(content)
	claim := taskStateClaim{}
	for _, raw := range taskClaimTaskPattern.FindAllString(normalized, -1) {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" || containsString(claim.TaskIDs, id) {
			continue
		}
		claim.TaskIDs = append(claim.TaskIDs, id)
	}
	if len(claim.TaskIDs) == 0 {
		return claim
	}
	claim.ClaimsUnblocked = containsAnyNormalizedFragment(normalized,
		"destrav", "unblocked", "resumed", "revogado", "saiu de blocked", "out of blocked",
	)
	claim.ClaimsInProgress = containsAnyNormalizedFragment(normalized,
		"in_progress", "em progresso", "em execucao", "de volta para in_progress", "status para in_progress",
	)
	claim.ClaimsReviewReady = containsAnyNormalizedFragment(normalized,
		"review-ready", "review ready", "ready_for_review", "pronta para revisao", "pronto para revisao", "em review",
	)
	claim.ClaimsDone = containsAnyNormalizedFragment(normalized,
		"concluid", "done", "finalizad", "complet", "encerrad",
	)
	return claim
}

func containsAnyNormalizedFragment(text string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(text, normalizeCoordinationText(fragment)) {
			return true
		}
	}
	return false
}

func extractTaskLaneSignalTokens(text string) []string {
	raw := taskLaneTokenPattern.FindAllString(strings.ToLower(strings.TrimSpace(text)), -1)
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, skip := taskLaneStopWords[token]; skip {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	preferred := make([]string, 0, len(out))
	for _, token := range out {
		if strings.Contains(token, "_") || len(token) >= 12 {
			preferred = append(preferred, token)
		}
	}
	if len(preferred) > 0 {
		out = preferred
	}
	if len(out) > 4 {
		out = out[:4]
	}
	sort.Strings(out)
	return out
}
