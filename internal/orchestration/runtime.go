package orchestration

// Runtime composes TaskRouter, Executor, and BudgetTracker into a dispatch loop.
type Runtime struct {
	router   *TaskRouter
	executor *Executor
	budget   *BudgetTracker
}

// NewRuntime returns a Runtime wired to the provided components.
func NewRuntime(router *TaskRouter, executor *Executor, budget *BudgetTracker) *Runtime {
	return &Runtime{router: router, executor: executor, budget: budget}
}

// DispatchResult holds the outcome of a single dispatch attempt.
type DispatchResult struct {
	TaskID    string
	AgentSlug string
	Skipped   bool
	Reason    string // non-empty when Skipped is true
}

// DispatchPending routes all pending tasks to the best available agents.
// Tasks with no capable agent or whose agent has exceeded budget are skipped.
// Returns one DispatchResult per pending task.
func (r *Runtime) DispatchPending() []DispatchResult {
	pending := r.executor.GetPending()
	results := make([]DispatchResult, 0, len(pending))

	for _, task := range pending {
		res := DispatchResult{TaskID: task.ID}

		best := r.router.FindBestAgent(task)
		if best == nil {
			res.Skipped = true
			res.Reason = "no capable agent"
			results = append(results, res)
			continue
		}

		if !r.budget.CanProceed(best.AgentSlug) {
			res.Skipped = true
			res.Reason = "budget exceeded"
			results = append(results, res)
			continue
		}

		ok, err := r.executor.Checkout(task.ID, best.AgentSlug)
		if err != nil || !ok {
			res.Skipped = true
			if err != nil {
				res.Reason = err.Error()
			} else {
				res.Reason = "checkout rejected"
			}
			results = append(results, res)
			continue
		}

		res.AgentSlug = best.AgentSlug
		results = append(results, res)
	}

	return results
}
