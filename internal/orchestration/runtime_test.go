package orchestration

import "testing"

func TestRuntime_DispatchPending_Success(t *testing.T) {
	router := NewTaskRouter()
	router.RegisterAgent("alice", []SkillDeclaration{{Name: "general", Proficiency: 1.0}})

	exec := NewExecutor(OrchestratorConfig{MaxConcurrentAgents: 5})
	_ = exec.Submit(TaskDefinition{ID: "t1", Title: "t1", RequiredSkills: []string{"general"}})

	budget := NewBudgetTracker(BudgetLimit{MaxTokens: 1000})

	rt := NewRuntime(router, exec, budget)
	results := rt.DispatchPending()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Skipped {
		t.Errorf("expected dispatch, got skip: %s", results[0].Reason)
	}
	if results[0].AgentSlug != "alice" {
		t.Errorf("expected alice, got %s", results[0].AgentSlug)
	}

	active := exec.GetActive()
	if len(active) != 1 || active[0].ID != "t1" {
		t.Errorf("expected t1 in active, got %v", active)
	}
}

func TestRuntime_DispatchPending_NoCapableAgent(t *testing.T) {
	router := NewTaskRouter()

	exec := NewExecutor(OrchestratorConfig{MaxConcurrentAgents: 5})
	_ = exec.Submit(TaskDefinition{ID: "t1", Title: "t1", RequiredSkills: []string{"general"}})

	budget := NewBudgetTracker(BudgetLimit{MaxTokens: 1000})

	rt := NewRuntime(router, exec, budget)
	results := rt.DispatchPending()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Skipped {
		t.Error("expected skip when no capable agent")
	}
	if results[0].Reason != "no capable agent" {
		t.Errorf("expected 'no capable agent', got %q", results[0].Reason)
	}
}

func TestRuntime_DispatchPending_BudgetExceeded(t *testing.T) {
	router := NewTaskRouter()
	router.RegisterAgent("alice", []SkillDeclaration{{Name: "general", Proficiency: 1.0}})

	exec := NewExecutor(OrchestratorConfig{MaxConcurrentAgents: 5})
	_ = exec.Submit(TaskDefinition{ID: "t1", Title: "t1", RequiredSkills: []string{"general"}})

	budget := NewBudgetTracker(BudgetLimit{MaxTokens: 10})
	budget.Record("alice", 20, 0) // exceed the limit

	rt := NewRuntime(router, exec, budget)
	results := rt.DispatchPending()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Skipped {
		t.Error("expected skip when budget exceeded")
	}
	if results[0].Reason != "budget exceeded" {
		t.Errorf("expected 'budget exceeded', got %q", results[0].Reason)
	}
}

func TestRuntime_DispatchPending_CapacityBlocked(t *testing.T) {
	router := NewTaskRouter()
	router.RegisterAgent("alice", []SkillDeclaration{{Name: "general", Proficiency: 1.0}})

	exec := NewExecutor(OrchestratorConfig{MaxConcurrentAgents: 1})
	_ = exec.Submit(TaskDefinition{ID: "t1", Title: "t1", RequiredSkills: []string{"general"}})
	_ = exec.Submit(TaskDefinition{ID: "t2", Title: "t2", RequiredSkills: []string{"general"}})

	budget := NewBudgetTracker(BudgetLimit{MaxTokens: 1000})

	rt := NewRuntime(router, exec, budget)
	results := rt.DispatchPending()

	dispatched := 0
	skipped := 0
	for _, r := range results {
		if r.Skipped {
			skipped++
		} else {
			dispatched++
		}
	}

	if dispatched != 1 {
		t.Errorf("expected 1 dispatched, got %d", dispatched)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped (capacity limit), got %d", skipped)
	}
}
