package team

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type officeAdapter struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	Description   string   `json:"description,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Status        string   `json:"status,omitempty"`
	HealthStatus  string   `json:"health_status,omitempty"`
	HealthSummary string   `json:"health_summary,omitempty"`
	ConfigRef     string   `json:"config_ref,omitempty"`
	Source        string   `json:"source,omitempty"`
	CreatedBy     string   `json:"created_by,omitempty"`
	CreatedAt     string   `json:"created_at,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
}

type orgProposal struct {
	ID                            string `json:"id"`
	Kind                          string `json:"kind,omitempty"`
	Title                         string `json:"title"`
	Summary                       string `json:"summary,omitempty"`
	Rationale                     string `json:"rationale,omitempty"`
	ProposedBy                    string `json:"proposed_by,omitempty"`
	Channel                       string `json:"channel,omitempty"`
	TargetType                    string `json:"target_type,omitempty"`
	TargetID                      string `json:"target_id,omitempty"`
	ProposedChange                string `json:"proposed_change,omitempty"`
	Status                        string `json:"status,omitempty"`
	RequiresTopologyAuthorization bool   `json:"requires_topology_authorization,omitempty"`
	SourceTaskID                  string `json:"source_task_id,omitempty"`
	SourceMessageID               string `json:"source_message_id,omitempty"`
	DecidedBy                     string `json:"decided_by,omitempty"`
	DecidedAt                     string `json:"decided_at,omitempty"`
	DecisionReason                string `json:"decision_reason,omitempty"`
	CreatedAt                     string `json:"created_at"`
	UpdatedAt                     string `json:"updated_at,omitempty"`
}

func defaultOfficeAdapters() []officeAdapter {
	now := "builtin"
	return []officeAdapter{
		{
			ID:            "local-broker",
			Name:          "Local broker",
			Kind:          "runtime",
			Provider:      "dunderia",
			Description:   "Local-first message, task, decision and audit bus.",
			Capabilities:  []string{"message.route", "task.create", "decision.record", "action.audit"},
			Status:        "active",
			HealthStatus:  "ready",
			HealthSummary: "Available in the local runtime.",
			Source:        "builtin",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "fresh-runner",
			Name:          "Fresh task runner",
			Kind:          "runtime",
			Provider:      "dunderia",
			Description:   "Per-turn runner contract with isolated task execution.",
			Capabilities:  []string{"task.execute", "worktree.isolate", "artifact.record"},
			Status:        "active",
			HealthStatus:  "ready",
			HealthSummary: "Backed by the local runner and task log model.",
			Source:        "builtin",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "scoped-mcp",
			Name:          "Scoped MCP",
			Kind:          "mcp",
			Provider:      "dunderia",
			Description:   "Scoped tool and context access for agents.",
			Capabilities:  []string{"tool.invoke", "context.scope", "permission.audit"},
			Status:        "active",
			HealthStatus:  "ready",
			HealthSummary: "Registered as a governed local capability.",
			Source:        "builtin",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "github-publication",
			Name:          "GitHub publication",
			Kind:          "integration",
			Provider:      "github",
			Description:   "Issue, PR and review publication contract.",
			Capabilities:  []string{"issue.open", "pr.open", "review.sync", "release.audit"},
			Status:        "active",
			HealthStatus:  "unknown",
			HealthSummary: "Depends on repository credentials and task publication policy.",
			Source:        "builtin",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "learning-registry",
			Name:          "Learning registry",
			Kind:          "knowledge",
			Provider:      "dunderia",
			Description:   "Promotes completed task evidence into reusable skills.",
			Capabilities:  []string{"knowledge.reuse", "skill.invoke", "learning.promote"},
			Status:        "active",
			HealthStatus:  "ready",
			HealthSummary: "Uses task evidence and promoted skills.",
			Source:        "builtin",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
}

func normalizeAdapterKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	switch value {
	case "runtime", "integration", "mcp", "knowledge", "workflow", "ui", "storage", "adapter":
		return value
	default:
		if value == "" {
			return "adapter"
		}
		return "adapter"
	}
}

func normalizeAdapterStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "active", "proposed", "disabled", "degraded", "archived":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "active"
	}
}

func normalizeAdapterRecord(adapter officeAdapter, now string) officeAdapter {
	adapter.ID = normalizeExecutionKey(adapter.ID)
	if adapter.ID == "" {
		adapter.ID = normalizeExecutionKey(adapter.Name)
	}
	adapter.Name = strings.TrimSpace(adapter.Name)
	if adapter.Name == "" {
		adapter.Name = humanizeSlug(adapter.ID)
	}
	adapter.Kind = normalizeAdapterKind(adapter.Kind)
	adapter.Provider = strings.TrimSpace(adapter.Provider)
	adapter.Description = strings.TrimSpace(adapter.Description)
	adapter.Capabilities = normalizeSkillCapabilities(adapter.Capabilities)
	adapter.Status = normalizeAdapterStatus(adapter.Status)
	adapter.HealthStatus = normalizeSkillHealthStatus(adapter.HealthStatus)
	if adapter.HealthStatus == "" {
		adapter.HealthStatus = "unknown"
	}
	adapter.HealthSummary = strings.TrimSpace(adapter.HealthSummary)
	adapter.ConfigRef = strings.TrimSpace(adapter.ConfigRef)
	adapter.Source = strings.TrimSpace(adapter.Source)
	adapter.CreatedBy = strings.TrimSpace(adapter.CreatedBy)
	if adapter.CreatedAt == "" {
		adapter.CreatedAt = now
	}
	if adapter.UpdatedAt == "" {
		adapter.UpdatedAt = adapter.CreatedAt
	}
	return adapter
}

func (b *Broker) normalizeAdaptersLocked() {
	now := time.Now().UTC().Format(time.RFC3339)
	seen := make(map[string]struct{}, len(b.adapters))
	out := make([]officeAdapter, 0, len(b.adapters))
	for _, adapter := range b.adapters {
		adapter = normalizeAdapterRecord(adapter, now)
		if adapter.ID == "" {
			continue
		}
		if _, ok := seen[adapter.ID]; ok {
			continue
		}
		seen[adapter.ID] = struct{}{}
		out = append(out, adapter)
	}
	b.adapters = out
}

func mergedOfficeAdapters(overrides []officeAdapter) []officeAdapter {
	now := time.Now().UTC().Format(time.RFC3339)
	ordered := defaultOfficeAdapters()
	index := make(map[string]int, len(ordered)+len(overrides))
	for i := range ordered {
		ordered[i] = normalizeAdapterRecord(ordered[i], now)
		index[ordered[i].ID] = i
	}
	for _, adapter := range overrides {
		adapter = normalizeAdapterRecord(adapter, now)
		if adapter.ID == "" || adapter.Status == "archived" {
			continue
		}
		if i, ok := index[adapter.ID]; ok {
			ordered[i] = adapter
			continue
		}
		index[adapter.ID] = len(ordered)
		ordered = append(ordered, adapter)
	}
	return ordered
}

func adapterMatchesFilters(adapter officeAdapter, kindFilter, providerFilter, capabilityFilter, statusFilter string) bool {
	if kindFilter != "" && adapter.Kind != kindFilter {
		return false
	}
	if providerFilter != "" && !strings.EqualFold(adapter.Provider, providerFilter) {
		return false
	}
	if capabilityFilter != "" && !stringSliceContains(adapter.Capabilities, capabilityFilter) {
		return false
	}
	if statusFilter != "" && adapter.Status != statusFilter {
		return false
	}
	return true
}

func (b *Broker) handleAdapters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		kindFilter := normalizeAdapterKind(r.URL.Query().Get("kind"))
		if strings.TrimSpace(r.URL.Query().Get("kind")) == "" {
			kindFilter = ""
		}
		providerFilter := strings.TrimSpace(r.URL.Query().Get("provider"))
		capabilityFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("capability")))
		statusFilter := normalizeAdapterStatus(r.URL.Query().Get("status"))
		if strings.TrimSpace(r.URL.Query().Get("status")) == "" {
			statusFilter = ""
		}
		b.mu.Lock()
		adapters := mergedOfficeAdapters(b.adapters)
		b.mu.Unlock()
		filtered := make([]officeAdapter, 0, len(adapters))
		for _, adapter := range adapters {
			if adapterMatchesFilters(adapter, kindFilter, providerFilter, capabilityFilter, statusFilter) {
				filtered = append(filtered, adapter)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"adapters": filtered})
	case http.MethodPost, http.MethodPut:
		b.handleUpsertAdapter(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleUpsertAdapter(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Adapter       officeAdapter `json:"adapter"`
		ID            string        `json:"id"`
		Name          string        `json:"name"`
		Kind          string        `json:"kind"`
		Provider      string        `json:"provider"`
		Description   string        `json:"description"`
		Capabilities  []string      `json:"capabilities"`
		Status        string        `json:"status"`
		HealthStatus  string        `json:"health_status"`
		HealthSummary string        `json:"health_summary"`
		ConfigRef     string        `json:"config_ref"`
		Source        string        `json:"source"`
		CreatedBy     string        `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	adapter := body.Adapter
	if adapter.ID == "" {
		adapter.ID = body.ID
	}
	if adapter.Name == "" {
		adapter.Name = body.Name
	}
	if adapter.Kind == "" {
		adapter.Kind = body.Kind
	}
	if adapter.Provider == "" {
		adapter.Provider = body.Provider
	}
	if adapter.Description == "" {
		adapter.Description = body.Description
	}
	if adapter.Capabilities == nil {
		adapter.Capabilities = body.Capabilities
	}
	if adapter.Status == "" {
		adapter.Status = body.Status
	}
	if adapter.HealthStatus == "" {
		adapter.HealthStatus = body.HealthStatus
	}
	if adapter.HealthSummary == "" {
		adapter.HealthSummary = body.HealthSummary
	}
	if adapter.ConfigRef == "" {
		adapter.ConfigRef = body.ConfigRef
	}
	if adapter.Source == "" {
		adapter.Source = body.Source
	}
	if adapter.CreatedBy == "" {
		adapter.CreatedBy = body.CreatedBy
	}
	now := time.Now().UTC().Format(time.RFC3339)
	adapter = normalizeAdapterRecord(adapter, now)
	if adapter.ID == "" || adapter.Name == "" {
		http.Error(w, "id and name required", http.StatusBadRequest)
		return
	}
	if adapter.CreatedBy == "" {
		http.Error(w, "created_by required", http.StatusBadRequest)
		return
	}
	if adapter.Source == "" {
		adapter.Source = "manual"
	}

	b.mu.Lock()
	updated := false
	for i := range b.adapters {
		if normalizeExecutionKey(b.adapters[i].ID) != adapter.ID {
			continue
		}
		if b.adapters[i].CreatedAt != "" {
			adapter.CreatedAt = b.adapters[i].CreatedAt
		}
		adapter.UpdatedAt = now
		b.adapters[i] = adapter
		updated = true
		break
	}
	if !updated {
		adapter.CreatedAt = now
		adapter.UpdatedAt = now
		b.adapters = append(b.adapters, adapter)
	}
	kind := "adapter_registered"
	if updated {
		kind = "adapter_updated"
	}
	b.appendActionLocked(kind, "office", "general", adapter.CreatedBy, truncateSummary(adapter.Name+" ["+adapter.Kind+"]", 140), adapter.ID)
	if err := b.saveLocked(); err != nil {
		b.mu.Unlock()
		http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
		return
	}
	b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"adapter": adapter, "persisted": true, "updated": updated})
}

func normalizeOrgProposalKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	switch value {
	case "agent", "channel", "routing", "workflow", "skill", "policy", "process", "topology", "team", "member":
		return value
	default:
		if value == "" {
			return "process"
		}
		return "process"
	}
}

func normalizeOrgProposalStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "proposed", "approved", "rejected", "archived":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "proposed"
	}
}

func orgProposalRequiresTopologyAuthorization(kind, targetType, change string) bool {
	kind = normalizeOrgProposalKind(kind)
	targetType = normalizeOrgProposalKind(targetType)
	switch kind {
	case "agent", "channel", "routing", "topology", "team", "member":
		return true
	}
	switch targetType {
	case "agent", "channel", "routing", "topology", "team", "member":
		return true
	}
	lower := strings.ToLower(change)
	return strings.Contains(lower, "create agent") ||
		strings.Contains(lower, "delete agent") ||
		strings.Contains(lower, "create channel") ||
		strings.Contains(lower, "delete channel") ||
		strings.Contains(lower, "rename channel") ||
		strings.Contains(lower, "reassign")
}

func normalizeOrgProposalRecord(proposal orgProposal, now string) orgProposal {
	proposal.ID = strings.TrimSpace(proposal.ID)
	proposal.Kind = normalizeOrgProposalKind(proposal.Kind)
	proposal.Title = strings.TrimSpace(proposal.Title)
	proposal.Summary = strings.TrimSpace(proposal.Summary)
	proposal.Rationale = strings.TrimSpace(proposal.Rationale)
	proposal.ProposedBy = strings.TrimSpace(proposal.ProposedBy)
	proposal.Channel = normalizeChannelSlug(proposal.Channel)
	if proposal.Channel == "" {
		proposal.Channel = "general"
	}
	proposal.TargetType = normalizeOrgProposalKind(proposal.TargetType)
	proposal.TargetID = strings.TrimSpace(proposal.TargetID)
	proposal.ProposedChange = strings.TrimSpace(proposal.ProposedChange)
	proposal.Status = normalizeOrgProposalStatus(proposal.Status)
	proposal.SourceTaskID = strings.TrimSpace(proposal.SourceTaskID)
	proposal.SourceMessageID = strings.TrimSpace(proposal.SourceMessageID)
	proposal.DecidedBy = strings.TrimSpace(proposal.DecidedBy)
	proposal.DecidedAt = strings.TrimSpace(proposal.DecidedAt)
	proposal.DecisionReason = strings.TrimSpace(proposal.DecisionReason)
	proposal.RequiresTopologyAuthorization = proposal.RequiresTopologyAuthorization ||
		orgProposalRequiresTopologyAuthorization(proposal.Kind, proposal.TargetType, proposal.ProposedChange)
	if proposal.CreatedAt == "" {
		proposal.CreatedAt = now
	}
	if proposal.UpdatedAt == "" {
		proposal.UpdatedAt = proposal.CreatedAt
	}
	return proposal
}

func (b *Broker) normalizeOrgProposalsLocked() {
	now := time.Now().UTC().Format(time.RFC3339)
	seen := make(map[string]struct{}, len(b.orgProposals))
	out := make([]orgProposal, 0, len(b.orgProposals))
	for _, proposal := range b.orgProposals {
		proposal = normalizeOrgProposalRecord(proposal, now)
		if proposal.ID == "" {
			continue
		}
		if _, ok := seen[proposal.ID]; ok {
			continue
		}
		seen[proposal.ID] = struct{}{}
		out = append(out, proposal)
	}
	b.orgProposals = out
}

func (b *Broker) handleOrgProposals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		statusFilter := normalizeOrgProposalStatus(r.URL.Query().Get("status"))
		if strings.TrimSpace(r.URL.Query().Get("status")) == "" {
			statusFilter = ""
		}
		kindFilter := normalizeOrgProposalKind(r.URL.Query().Get("kind"))
		if strings.TrimSpace(r.URL.Query().Get("kind")) == "" {
			kindFilter = ""
		}
		b.mu.Lock()
		proposals := make([]orgProposal, 0, len(b.orgProposals))
		for _, proposal := range b.orgProposals {
			if statusFilter != "" && proposal.Status != statusFilter {
				continue
			}
			if kindFilter != "" && proposal.Kind != kindFilter {
				continue
			}
			proposals = append(proposals, proposal)
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"proposals": proposals})
	case http.MethodPost:
		b.handleMutateOrgProposal(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *Broker) handleMutateOrgProposal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action                        string `json:"action"`
		ID                            string `json:"id"`
		Kind                          string `json:"kind"`
		Title                         string `json:"title"`
		Summary                       string `json:"summary"`
		Rationale                     string `json:"rationale"`
		ProposedBy                    string `json:"proposed_by"`
		Channel                       string `json:"channel"`
		TargetType                    string `json:"target_type"`
		TargetID                      string `json:"target_id"`
		ProposedChange                string `json:"proposed_change"`
		RequiresTopologyAuthorization bool   `json:"requires_topology_authorization"`
		SourceTaskID                  string `json:"source_task_id"`
		SourceMessageID               string `json:"source_message_id"`
		Actor                         string `json:"actor"`
		DecisionReason                string `json:"decision_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(body.Action)
	if action == "" {
		action = "propose"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	b.mu.Lock()
	defer b.mu.Unlock()
	switch action {
	case "propose":
		if strings.TrimSpace(body.Title) == "" || strings.TrimSpace(body.ProposedBy) == "" {
			http.Error(w, "title and proposed_by required", http.StatusBadRequest)
			return
		}
		channel := normalizeChannelSlug(body.Channel)
		if channel == "" {
			channel = "general"
		}
		if b.findChannelLocked(channel) == nil {
			http.Error(w, "channel not found", http.StatusNotFound)
			return
		}
		b.counter++
		proposal := normalizeOrgProposalRecord(orgProposal{
			ID:                            fmt.Sprintf("org-proposal-%d", b.counter),
			Kind:                          body.Kind,
			Title:                         body.Title,
			Summary:                       body.Summary,
			Rationale:                     body.Rationale,
			ProposedBy:                    body.ProposedBy,
			Channel:                       channel,
			TargetType:                    body.TargetType,
			TargetID:                      body.TargetID,
			ProposedChange:                body.ProposedChange,
			Status:                        "proposed",
			RequiresTopologyAuthorization: body.RequiresTopologyAuthorization,
			SourceTaskID:                  body.SourceTaskID,
			SourceMessageID:               body.SourceMessageID,
			CreatedAt:                     now,
			UpdatedAt:                     now,
		}, now)
		b.orgProposals = append(b.orgProposals, proposal)
		summary := proposal.Title
		if proposal.RequiresTopologyAuthorization {
			summary += " [topology authorization required]"
		}
		b.appendActionLocked("org_proposal_created", "office", proposal.Channel, proposal.ProposedBy, truncateSummary(summary, 140), proposal.ID)
		if err := b.saveLocked(); err != nil {
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"proposal": proposal, "persisted": true})
	case "approve", "reject":
		id := strings.TrimSpace(body.ID)
		actor := strings.TrimSpace(body.Actor)
		if actor == "" {
			actor = "human"
		}
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		for i := range b.orgProposals {
			if b.orgProposals[i].ID != id {
				continue
			}
			status := "approved"
			if action == "reject" {
				status = "rejected"
			}
			b.orgProposals[i].Status = status
			b.orgProposals[i].DecidedBy = actor
			b.orgProposals[i].DecidedAt = now
			b.orgProposals[i].DecisionReason = strings.TrimSpace(body.DecisionReason)
			b.orgProposals[i].UpdatedAt = now
			kind := "org_proposal_" + status
			if b.orgProposals[i].RequiresTopologyAuthorization && status == "approved" {
				kind = "org_proposal_authorized"
			}
			b.appendActionLocked(kind, "office", b.orgProposals[i].Channel, actor, truncateSummary(b.orgProposals[i].Title+" ["+status+"]", 140), b.orgProposals[i].ID)
			if err := b.saveLocked(); err != nil {
				http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"proposal": b.orgProposals[i], "persisted": true})
			return
		}
		http.Error(w, "proposal not found", http.StatusNotFound)
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
	}
}

func (b *Broker) handleCEOConversationConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Kind            string `json:"kind"`
		Channel         string `json:"channel"`
		CreatedBy       string `json:"created_by"`
		Actor           string `json:"actor"`
		SourceMessageID string `json:"source_message_id"`
		ThreadID        string `json:"thread_id"`
		Title           string `json:"title"`
		Summary         string `json:"summary"`
		Details         string `json:"details"`
		Outcome         string `json:"outcome"`
		Owner           string `json:"owner"`
		Reason          string `json:"reason"`
		Blocking        bool   `json:"blocking"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	if kind == "" {
		kind = "task"
	}
	actor := strings.TrimSpace(firstNonEmpty(body.CreatedBy, body.Actor, "human"))
	channel := normalizeChannelSlug(body.Channel)
	if channel == "" {
		channel = "general"
	}
	title := strings.TrimSpace(firstNonEmpty(body.Title, body.Summary))
	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	details := strings.TrimSpace(firstNonEmpty(body.Details, body.Summary))
	if source := strings.TrimSpace(body.SourceMessageID); source != "" {
		details = strings.TrimSpace(details + "\n\nSource conversation message: " + source)
	}

	switch kind {
	case "task":
		task, duplicate, err := b.EnsurePlannedTask(plannedTaskInput{
			Channel:          channel,
			Title:            title,
			Details:          details,
			Owner:            strings.TrimSpace(body.Owner),
			CreatedBy:        actor,
			ThreadID:         firstNonEmpty(strings.TrimSpace(body.ThreadID), strings.TrimSpace(body.SourceMessageID)),
			TaskType:         "follow_up",
			ExecutionMode:    "office",
			ReviewState:      "needs_review",
			SourceDecisionID: "",
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		_ = b.RecordAction("ceo_conversation_converted", "ceo", channel, actor, truncateSummary(title+" [task]", 140), task.ID, nil, "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"kind": "task", "task": task, "duplicate": duplicate})
	case "decision":
		decision, err := b.RecordDecision("ceo_conversation", channel, title, firstNonEmpty(body.Reason, details), actor, nil, false, body.Blocking)
		if err != nil {
			http.Error(w, "failed to persist decision", http.StatusInternalServerError)
			return
		}
		_ = b.RecordAction("ceo_conversation_converted", "ceo", channel, actor, truncateSummary(title+" [decision]", 140), strings.TrimSpace(body.SourceMessageID), nil, decision.ID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"kind": "decision", "decision": decision})
	case "request":
		b.mu.Lock()
		if b.findChannelLocked(channel) == nil {
			b.mu.Unlock()
			http.Error(w, "channel not found", http.StatusNotFound)
			return
		}
		b.counter++
		now := time.Now().UTC().Format(time.RFC3339)
		req := normalizeRequestRecord(humanInterview{
			ID:        fmt.Sprintf("request-%d", b.counter),
			Kind:      "decision",
			Status:    "pending",
			From:      actor,
			Channel:   channel,
			Title:     title,
			Question:  firstNonEmpty(body.Summary, title),
			Context:   details,
			Blocking:  body.Blocking,
			Required:  body.Blocking,
			ReplyTo:   strings.TrimSpace(body.SourceMessageID),
			CreatedAt: now,
			UpdatedAt: now,
		})
		b.scheduleRequestLifecycleLocked(&req)
		b.requests = append(b.requests, req)
		b.pendingInterview = firstBlockingRequest(b.requests)
		b.appendActionLocked("ceo_conversation_converted", "ceo", channel, actor, truncateSummary(title+" [request]", 140), req.ID)
		if err := b.saveLocked(); err != nil {
			b.mu.Unlock()
			http.Error(w, "failed to persist broker state", http.StatusInternalServerError)
			return
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"kind": "request", "request": req})
	default:
		http.Error(w, "kind must be task, decision, or request", http.StatusBadRequest)
	}
}
