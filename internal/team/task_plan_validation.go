package team

import (
	"fmt"
	"strings"
)

type plannedTaskSpec struct {
	ExecutionKey    string
	Title           string
	Assignee        string
	Details         string
	TaskType        string
	ExecutionMode   string
	RuntimeProvider string
	RuntimeModel    string
	ReasoningEffort string
	WorkspacePath   string
	DependsOn       []string
}

type validatedPlannedTask struct {
	PlannedID       string
	Channel         string
	ExecutionKey    string
	Title           string
	Owner           string
	Details         string
	TaskType        string
	PipelineID      string
	ExecutionMode   string
	RuntimeProvider string
	RuntimeModel    string
	ReasoningEffort string
	ReviewState     string
	WorkspacePath   string
	ResolvedDepIDs  []string
}

func (b *Broker) validateStrictTaskPlanLocked(baseChannel, createdBy string, specs []plannedTaskSpec) ([]validatedPlannedTask, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("tasks required")
	}

	validated := make([]validatedPlannedTask, 0, len(specs))
	titleRefs := make(map[string][]string, len(specs))
	execKeys := make(map[string]string, len(specs))

	for i, spec := range specs {
		title := strings.TrimSpace(spec.Title)
		if title == "" {
			return nil, fmt.Errorf("planned task %d: title required", i+1)
		}
		owner := strings.TrimSpace(spec.Assignee)
		channel := b.preferredTaskChannelLocked(baseChannel, createdBy, owner, title, spec.Details)
		if b.findChannelLocked(channel) == nil {
			return nil, fmt.Errorf("planned task %q routes to unknown channel %q", title, channel)
		}

		task := teamTask{
			Channel:         channel,
			ExecutionKey:    normalizeExecutionKey(spec.ExecutionKey),
			Title:           title,
			Details:         strings.TrimSpace(spec.Details),
			Owner:           owner,
			CreatedBy:       strings.TrimSpace(createdBy),
			TaskType:        strings.TrimSpace(spec.TaskType),
			ExecutionMode:   strings.TrimSpace(spec.ExecutionMode),
			RuntimeProvider: strings.TrimSpace(spec.RuntimeProvider),
			RuntimeModel:    strings.TrimSpace(spec.RuntimeModel),
			ReasoningEffort: strings.TrimSpace(spec.ReasoningEffort),
			WorkspacePath:   strings.TrimSpace(spec.WorkspacePath),
		}
		if err := normalizeTaskRuntimeOverrides(&task); err != nil {
			return nil, fmt.Errorf("planned task %q: %w", title, err)
		}
		normalizeTaskPlan(&task)
		task.ExecutionKey = deriveTaskExecutionKey(&task)
		if err := validateStrictPlannedTaskMetadata(&task); err != nil {
			return nil, fmt.Errorf("planned task %q: %w", title, err)
		}
		if prior, ok := execKeys[task.ExecutionKey]; ok {
			return nil, fmt.Errorf("planned task %q duplicates execution identity already used by %q", title, prior)
		}
		execKeys[task.ExecutionKey] = title

		plannedID := fmt.Sprintf("planned-%d", i+1)
		validated = append(validated, validatedPlannedTask{
			PlannedID:       plannedID,
			Channel:         channel,
			ExecutionKey:    task.ExecutionKey,
			Title:           task.Title,
			Owner:           task.Owner,
			Details:         task.Details,
			TaskType:        task.TaskType,
			PipelineID:      task.PipelineID,
			ExecutionMode:   task.ExecutionMode,
			RuntimeProvider: task.RuntimeProvider,
			RuntimeModel:    task.RuntimeModel,
			ReasoningEffort: task.ReasoningEffort,
			ReviewState:     task.ReviewState,
			WorkspacePath:   task.WorkspacePath,
		})

		if titleKey := normalizeExecutionKey(title); titleKey != "" {
			titleRefs[titleKey] = append(titleRefs[titleKey], plannedID)
		}
	}

	for i := range validated {
		resolved := make([]string, 0, len(specs[i].DependsOn))
		for _, rawDep := range specs[i].DependsOn {
			dep := strings.TrimSpace(rawDep)
			if dep == "" {
				continue
			}
			resolvedDep, err := b.resolveStrictPlanDependencyLocked(validated[i], dep, titleRefs)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, resolvedDep)
		}
		validated[i].ResolvedDepIDs = compactStringList(resolved)
	}

	if err := validateStrictPlanDependencyCycles(validated); err != nil {
		return nil, err
	}
	if err := validateStrictPlanOwnerConcurrency(validated); err != nil {
		return nil, err
	}

	return validated, nil
}

func validateStrictPlannedTaskMetadata(task *teamTask) error {
	if task == nil {
		return fmt.Errorf("task required")
	}
	mode := strings.ToLower(strings.TrimSpace(task.ExecutionMode))
	workspacePath := strings.TrimSpace(task.WorkspacePath)

	switch mode {
	case "":
		return fmt.Errorf("execution_mode required")
	case "external_workspace":
		if workspacePath == "" {
			return fmt.Errorf("workspace_path required for external_workspace")
		}
		if strings.TrimSpace(task.Owner) == "" {
			return fmt.Errorf("owner required for external_workspace")
		}
	case "local_worktree":
		if workspacePath != "" {
			return fmt.Errorf("workspace_path is only allowed for external_workspace")
		}
		if strings.TrimSpace(task.Owner) == "" {
			return fmt.Errorf("owner required for local_worktree")
		}
	default:
		if workspacePath != "" {
			return fmt.Errorf("workspace_path is only allowed for external_workspace")
		}
	}
	return nil
}

func (b *Broker) resolveStrictPlanDependencyLocked(task validatedPlannedTask, dep string, titleRefs map[string][]string) (string, error) {
	if dep == task.PlannedID || strings.EqualFold(dep, task.Title) {
		return "", fmt.Errorf("planned task %q cannot depend on itself", task.Title)
	}
	if refs := titleRefs[normalizeExecutionKey(dep)]; len(refs) > 0 {
		if len(refs) > 1 {
			return "", fmt.Errorf("planned task %q depends on ambiguous title %q", task.Title, dep)
		}
		if refs[0] == task.PlannedID {
			return "", fmt.Errorf("planned task %q cannot depend on itself", task.Title)
		}
		return refs[0], nil
	}
	if b.strictPlanDependencyExistsLocked(dep) {
		return dep, nil
	}
	return "", fmt.Errorf("planned task %q depends on unknown task or request %q", task.Title, dep)
}

func (b *Broker) strictPlanDependencyExistsLocked(depID string) bool {
	depID = strings.TrimSpace(depID)
	if depID == "" {
		return false
	}
	for _, req := range b.requests {
		if strings.EqualFold(strings.TrimSpace(req.ID), depID) {
			return true
		}
	}
	for _, task := range b.tasks {
		if strings.EqualFold(strings.TrimSpace(task.ID), depID) {
			return true
		}
	}
	return false
}

func validateStrictPlanDependencyCycles(tasks []validatedPlannedTask) error {
	if len(tasks) <= 1 {
		return nil
	}
	byID := make(map[string]validatedPlannedTask, len(tasks))
	for _, task := range tasks {
		byID[task.PlannedID] = task
	}
	visiting := make(map[string]bool, len(tasks))
	visited := make(map[string]bool, len(tasks))

	var visit func(id string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("task plan contains a dependency cycle")
		}
		visiting[id] = true
		task := byID[id]
		for _, depID := range task.ResolvedDepIDs {
			if _, ok := byID[depID]; !ok {
				continue
			}
			if err := visit(depID); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}

	for _, task := range tasks {
		if err := visit(task.PlannedID); err != nil {
			return err
		}
	}
	return nil
}

func validateStrictPlanOwnerConcurrency(tasks []validatedPlannedTask) error {
	if len(tasks) <= 1 {
		return nil
	}
	byID := make(map[string]validatedPlannedTask, len(tasks))
	byOwner := make(map[string][]validatedPlannedTask)
	for _, task := range tasks {
		byID[task.PlannedID] = task
		owner := strings.TrimSpace(task.Owner)
		if owner == "" {
			continue
		}
		byOwner[owner] = append(byOwner[owner], task)
	}

	var dependsOn func(fromID, targetID string, seen map[string]struct{}) bool
	dependsOn = func(fromID, targetID string, seen map[string]struct{}) bool {
		if fromID == targetID {
			return true
		}
		if _, ok := seen[fromID]; ok {
			return false
		}
		seen[fromID] = struct{}{}
		task, ok := byID[fromID]
		if !ok {
			return false
		}
		for _, depID := range task.ResolvedDepIDs {
			if depID == targetID {
				return true
			}
			if _, ok := byID[depID]; !ok {
				continue
			}
			if dependsOn(depID, targetID, seen) {
				return true
			}
		}
		return false
	}

	for owner, items := range byOwner {
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				left := items[i]
				right := items[j]
				if dependsOn(left.PlannedID, right.PlannedID, map[string]struct{}{}) {
					continue
				}
				if dependsOn(right.PlannedID, left.PlannedID, map[string]struct{}{}) {
					continue
				}
				return fmt.Errorf("planned task batch opens concurrent work for owner %q without an explicit dependency chain", owner)
			}
		}
	}
	return nil
}
