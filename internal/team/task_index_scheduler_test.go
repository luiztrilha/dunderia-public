package team

import (
	"testing"
	"time"
)

func TestBrokerTaskLookupUsesIndexedPaths(t *testing.T) {
	b := NewBroker()
	b.mu.Lock()
	b.tasks = []teamTask{
		{ID: "task-1", Channel: "general", Owner: "ceo", Status: "open", TaskType: "follow_up", Title: "CEO follow-up"},
		{ID: "task-2", Channel: "engineering", Owner: "eng", Status: "in_progress", TaskType: "delivery", Title: "Implement broker indexes"},
		{ID: "task-3", Channel: "engineering", Owner: "eng", Status: "review", TaskType: "delivery", Title: "Review broker indexes"},
	}
	b.counter = 3
	b.rebuildTaskIndexesLocked()
	b.mu.Unlock()

	task, ok := b.TaskByID("task-2")
	if !ok || task.Title != "Implement broker indexes" {
		t.Fatalf("expected indexed task lookup to return task-2, got %+v ok=%t", task, ok)
	}

	task, ok = b.FindTask("engineering", "task-2")
	if !ok || task.Owner != "eng" {
		t.Fatalf("expected indexed channel task lookup to return eng task, got %+v ok=%t", task, ok)
	}

	active, ok := b.ActiveTaskByOwner("eng", "engineering")
	if !ok || active.ID != "task-2" {
		t.Fatalf("expected indexed active task lookup to return task-2, got %+v ok=%t", active, ok)
	}

	followUps := b.TasksByType("follow_up")
	if len(followUps) != 1 || followUps[0].ID != "task-1" {
		t.Fatalf("expected indexed task-type lookup to return task-1, got %+v", followUps)
	}
}

func TestDueSchedulerJobsPrunesDuplicateAndExpiredTerminalJobs(t *testing.T) {
	b := NewBroker()
	now := time.Now().UTC()

	b.mu.Lock()
	b.scheduler = []schedulerJob{
		{
			Slug:    "stale-done",
			Kind:    "task_follow_up",
			Status:  "done",
			LastRun: now.Add(-3 * time.Hour).Format(time.RFC3339),
		},
		{
			Slug:       "dup-job",
			Kind:       "task_follow_up",
			TargetType: "task",
			TargetID:   "task-1",
			Channel:    "general",
			Status:     "scheduled",
			NextRun:    now.Add(-2 * time.Minute).Format(time.RFC3339),
			DueAt:      now.Add(-2 * time.Minute).Format(time.RFC3339),
		},
		{
			Slug:       "dup-job",
			Kind:       "task_follow_up",
			TargetType: "task",
			TargetID:   "task-1",
			Channel:    "general",
			Status:     "scheduled",
			NextRun:    now.Add(-time.Minute).Format(time.RFC3339),
			DueAt:      now.Add(-time.Minute).Format(time.RFC3339),
		},
		{
			Slug:       "future-job",
			Kind:       "task_follow_up",
			TargetType: "task",
			TargetID:   "task-2",
			Channel:    "general",
			Status:     "scheduled",
			NextRun:    now.Add(time.Minute).Format(time.RFC3339),
			DueAt:      now.Add(time.Minute).Format(time.RFC3339),
		},
	}
	b.mu.Unlock()

	due := b.DueSchedulerJobs()
	if len(due) != 1 || due[0].Slug != "dup-job" {
		t.Fatalf("expected only deduped due job, got %+v", due)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.scheduler) != 2 {
		t.Fatalf("expected scheduler to prune stale/duplicate jobs, got %+v", b.scheduler)
	}
	if b.scheduler[0].Slug != "dup-job" || b.scheduler[1].Slug != "future-job" {
		t.Fatalf("unexpected remaining scheduler jobs: %+v", b.scheduler)
	}
}
