package orchestration

import (
	"fmt"
	"sync"
	"time"
)

// ExecutorEvent is emitted when a task transitions state.
type ExecutorEvent struct {
	Type      string // "task:start", "task:complete", "task:fail", "task:timeout"
	TaskID    string
	AgentSlug string
	Result    string
	Error     string
}

type executorListenerEntry struct {
	id int
	fn func(ExecutorEvent)
}

// Executor manages task lifecycle with concurrency limits and timeout enforcement.
type Executor struct {
	config             OrchestratorConfig
	tasks              map[string]*TaskDefinition
	locks              map[string]string // taskID → agentSlug
	activeCount        int
	localWorktreeCount int // durable turn guard: max 1 local_worktree task active at once
	listeners          []executorListenerEntry
	nextListenerID     int
	timers             map[string]*time.Timer
	stopped            bool
	mu                 sync.Mutex
}

// NewExecutor returns an Executor using the provided configuration.
func NewExecutor(config OrchestratorConfig) *Executor {
	return &Executor{
		config: config,
		tasks:  make(map[string]*TaskDefinition),
		locks:  make(map[string]string),
		timers: make(map[string]*time.Timer),
	}
}

// OnEvent registers a handler for task events. Returns an unsubscribe function.
func (e *Executor) OnEvent(handler func(ExecutorEvent)) func() {
	e.mu.Lock()
	id := e.nextListenerID
	e.nextListenerID++
	e.listeners = append(e.listeners, executorListenerEntry{id: id, fn: handler})
	e.mu.Unlock()
	return func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		for i, entry := range e.listeners {
			if entry.id != id {
				continue
			}
			e.listeners = append(e.listeners[:i], e.listeners[i+1:]...)
			return
		}
	}
}

// Submit adds a task to the executor's queue (status "pending").
// Returns an error if a task with the same ID already exists.
func (e *Executor) Submit(task TaskDefinition) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped {
		return fmt.Errorf("executor is stopped")
	}
	if _, exists := e.tasks[task.ID]; exists {
		return fmt.Errorf("task %q already submitted", task.ID)
	}
	task.Status = "pending"
	copy := task
	e.tasks[task.ID] = &copy
	return nil
}

// Checkout atomically assigns taskID to agentSlug and starts the task.
// Returns (true, nil) on success, (false, nil) if the task is already locked
// or the concurrency limit is reached, or (false, err) on other errors.
func (e *Executor) Checkout(taskID, agentSlug string) (bool, error) {
	e.mu.Lock()

	if e.stopped {
		e.mu.Unlock()
		return false, fmt.Errorf("executor is stopped")
	}

	task, ok := e.tasks[taskID]
	if !ok {
		e.mu.Unlock()
		return false, fmt.Errorf("task %q not found", taskID)
	}
	if task.Status != "pending" {
		e.mu.Unlock()
		return false, nil
	}
	if _, locked := e.locks[taskID]; locked {
		e.mu.Unlock()
		return false, nil
	}
	if e.config.MaxConcurrentAgents > 0 && e.activeCount >= e.config.MaxConcurrentAgents {
		e.mu.Unlock()
		return false, nil
	}
	if task.ExecutionMode == "local_worktree" && e.localWorktreeCount >= 1 {
		e.mu.Unlock()
		return false, nil
	}

	task.Status = "in_progress"
	task.AssignedAgent = agentSlug
	e.locks[taskID] = agentSlug
	e.activeCount++
	if task.ExecutionMode == "local_worktree" {
		e.localWorktreeCount++
	}

	// Schedule timeout if configured.
	if e.config.TaskTimeout > 0 {
		timer := time.AfterFunc(e.config.TaskTimeout, func() {
			e.handleTimeout(taskID, agentSlug)
		})
		e.timers[taskID] = timer
	}

	listeners := e.snapshotListenersLocked()
	e.mu.Unlock()
	e.emit(listeners, ExecutorEvent{Type: "task:start", TaskID: taskID, AgentSlug: agentSlug})
	return true, nil
}

// Release marks a task as completed or failed and frees the agent slot.
// Pass a non-nil result string for success, a non-nil err string for failure.
func (e *Executor) Release(taskID string, result *string, errStr *string) error {
	e.mu.Lock()
	task, ok := e.tasks[taskID]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("task %q not found", taskID)
	}
	if _, locked := e.locks[taskID]; !locked {
		e.mu.Unlock()
		return fmt.Errorf("task %q is not checked out", taskID)
	}

	agentSlug := e.locks[taskID]
	delete(e.locks, taskID)
	if e.activeCount > 0 {
		e.activeCount--
	}
	if task.ExecutionMode == "local_worktree" && e.localWorktreeCount > 0 {
		e.localWorktreeCount--
	}

	// Cancel timeout timer.
	if timer, ok := e.timers[taskID]; ok {
		timer.Stop()
		delete(e.timers, taskID)
	}

	task.CompletedAt = time.Now().UnixMilli()

	var event ExecutorEvent
	if errStr != nil && *errStr != "" {
		task.Status = "failed"
		event = ExecutorEvent{
			Type:      "task:fail",
			TaskID:    taskID,
			AgentSlug: agentSlug,
			Error:     *errStr,
		}
	} else {
		task.Status = "completed"
		if result != nil {
			task.Result = *result
		}
		event = ExecutorEvent{
			Type:      "task:complete",
			TaskID:    taskID,
			AgentSlug: agentSlug,
			Result:    task.Result,
		}
	}
	listeners := e.snapshotListenersLocked()
	e.mu.Unlock()
	e.emit(listeners, event)
	return nil
}

// GetActive returns a snapshot of all currently in-progress tasks.
func (e *Executor) GetActive() []TaskDefinition {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []TaskDefinition
	for _, t := range e.tasks {
		if t.Status == "in_progress" {
			out = append(out, *t)
		}
	}
	return out
}

// GetPending returns a snapshot of all tasks waiting to be checked out.
func (e *Executor) GetPending() []TaskDefinition {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []TaskDefinition
	for _, t := range e.tasks {
		if t.Status == "pending" {
			out = append(out, *t)
		}
	}
	return out
}

// GetTask returns the TaskDefinition for the given ID, or false if not found.
func (e *Executor) GetTask(id string) (TaskDefinition, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t, ok := e.tasks[id]
	if !ok {
		return TaskDefinition{}, false
	}
	return *t, true
}

// StopAll cancels all in-progress tasks and marks them as failed.
func (e *Executor) StopAll() error {
	e.mu.Lock()

	e.stopped = true

	for id, timer := range e.timers {
		timer.Stop()
		delete(e.timers, id)
	}

	events := make([]ExecutorEvent, 0, len(e.locks))
	for taskID, agentSlug := range e.locks {
		task := e.tasks[taskID]
		task.Status = "failed"
		task.CompletedAt = time.Now().UnixMilli()
		delete(e.locks, taskID)
		if e.activeCount > 0 {
			e.activeCount--
		}
		events = append(events, ExecutorEvent{
			Type:      "task:fail",
			TaskID:    taskID,
			AgentSlug: agentSlug,
			Error:     "executor stopped",
		})
	}
	listeners := e.snapshotListenersLocked()
	e.mu.Unlock()
	for _, event := range events {
		e.emit(listeners, event)
	}
	return nil
}

// handleTimeout is called by the per-task timer; fires a timeout event.
func (e *Executor) handleTimeout(taskID, agentSlug string) {
	e.mu.Lock()

	task, ok := e.tasks[taskID]
	if !ok {
		e.mu.Unlock()
		return
	}
	if task.Status != "in_progress" {
		e.mu.Unlock()
		return
	}

	task.Status = "failed"
	task.CompletedAt = time.Now().UnixMilli()
	delete(e.locks, taskID)
	delete(e.timers, taskID)
	if e.activeCount > 0 {
		e.activeCount--
	}
	if task.ExecutionMode == "local_worktree" && e.localWorktreeCount > 0 {
		e.localWorktreeCount--
	}

	listeners := e.snapshotListenersLocked()
	e.mu.Unlock()
	e.emit(listeners, ExecutorEvent{
		Type:      "task:timeout",
		TaskID:    taskID,
		AgentSlug: agentSlug,
		Error:     "task timed out",
	})
}

func (e *Executor) snapshotListenersLocked() []func(ExecutorEvent) {
	out := make([]func(ExecutorEvent), 0, len(e.listeners))
	for _, entry := range e.listeners {
		if entry.fn != nil {
			out = append(out, entry.fn)
		}
	}
	return out
}

func (e *Executor) emit(listeners []func(ExecutorEvent), event ExecutorEvent) {
	for _, fn := range listeners {
		go fn(event)
	}
}
