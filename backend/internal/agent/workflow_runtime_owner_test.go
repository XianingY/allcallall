package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

func TestWorkflowRuntimeOwnerDoesNotUpgradeLegacyRun(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Run.RuntimeOwner != WorkflowRuntimeLegacyGo {
		t.Fatalf("expected legacy owner at creation, got %q", created.Run.RuntimeOwner)
	}
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithWorkflowRuntime(runtime)
	result, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("legacy-owned run failed after enabling Python: %v", err)
	}
	if runtime.calls != 0 || result.Run.RuntimeOwner != WorkflowRuntimeLegacyGo {
		t.Fatalf("Python runtime took over legacy run: calls=%d run=%+v", runtime.calls, result.Run)
	}
}

func TestWorkflowRuntimeOwnerFailsClosedWhenPythonUnavailable(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Run.RuntimeOwner != WorkflowRuntimePythonLangGraph {
		t.Fatalf("expected Python owner at creation, got %q", created.Run.RuntimeOwner)
	}
	svc.WithWorkflowRuntime(nil)
	if _, err := svc.ProcessWorkflowRun(ctx, created.Run.ID); !errors.Is(err, ErrWorkflowRuntimeUnavailable) {
		t.Fatalf("expected unavailable fail-closed error, got %v", err)
	}
	var stored models.WorkflowRun
	if err := db.Take(&stored, created.Run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.WorkflowRunStatusFailed || stored.RuntimeOwner != WorkflowRuntimePythonLangGraph {
		t.Fatalf("Python-owned run fell through to legacy: %+v", stored)
	}
}

func TestAgentRuntimeLazilyLoadsForPythonOwnedWorker(t *testing.T) {
	t.Setenv("AGENT_RUNTIME", WorkflowRuntimePythonLangGraph)
	svc, _ := newWorkflowTestService(t)
	svc.WithWorkflowRuntime(nil)

	runtime, ok := svc.externalAgentRuntime()
	if !ok || runtime.Name() != WorkflowRuntimePythonLangGraph {
		t.Fatalf("Python-owned worker did not restore its runtime: runtime=%T ok=%v", runtime, ok)
	}
}

func TestWorkflowCheckpointBusyReleasesLeaseWithoutAttemptCost(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{runErr: &CheckpointExecutionBusyError{Body: `{"detail":{"code":"checkpoint_execution_busy"}}`}}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessWorkflowRun(ctx, created.Run.ID); !errors.Is(err, ErrCheckpointExecutionBusy) {
		t.Fatalf("expected checkpoint busy, got %v", err)
	}
	var pending models.WorkflowRun
	if err := db.Take(&pending, created.Run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != models.WorkflowRunStatusPending || pending.Attempts != 0 || pending.ExecutionLeaseToken != "" || pending.RuntimeRequestJSON == "" {
		t.Fatalf("busy run was terminalized or charged an attempt: %+v", pending)
	}
	runtime.runErr = nil
	paused, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("busy retry failed: %v", err)
	}
	if paused.Run.Status != models.WorkflowRunStatusRequiresAction || runtime.calls != 2 {
		t.Fatalf("busy retry did not resume initial execution: run=%+v calls=%d", paused.Run, runtime.calls)
	}
	if len(runtime.runRequests) != 2 || runtime.runRequests[0].ExecutionID != runtime.runRequests[1].ExecutionID {
		t.Fatalf("busy retry changed execution id: %+v", runtime.runRequests)
	}
}

func TestWorkflowCheckpointTooLargeFallsBackToLegacyEngine(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	// The external LangGraph runtime reports an oversized checkpoint.
	runtime := &fakeMeetingBriefRuntime{runErr: &CheckpointTransactionTooLargeError{Body: `{"detail":{"code":"checkpoint_transaction_too_large"}}`}}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Run.RuntimeOwner != WorkflowRuntimePythonLangGraph {
		t.Fatalf("expected Python owner at creation, got %q", created.Run.RuntimeOwner)
	}
	result, err := svc.ProcessWorkflowRun(ctx, created.Run.ID)
	if err != nil {
		t.Fatalf("expected checkpoint-too-large to degrade to legacy engine, got error: %v", err)
	}
	if result.Run.Status == models.WorkflowRunStatusFailed {
		t.Fatalf("checkpoint-too-large permanently failed instead of degrading: %+v", result.Run)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected exactly one external call before fallback, got %d", runtime.calls)
	}
	var fallbackCount int64
	if err := db.Model(&models.WorkflowHistoryEvent{}).
		Where("workflow_run_id = ? AND event_type = ?", created.Run.ID, models.WorkflowHistoryEventCheckpointFallback).
		Count(&fallbackCount).Error; err != nil {
		t.Fatal(err)
	}
	if fallbackCount != 1 {
		t.Fatalf("expected exactly one checkpoint_fallback history event, got %d", fallbackCount)
	}
}

func TestWorkflowCheckpointTooLargeFailsClosedWhenDisabled(t *testing.T) {
	t.Setenv("WORKFLOW_CHECKPOINT_FALLBACK", "false")
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	runtime := &fakeMeetingBriefRuntime{runErr: &CheckpointTransactionTooLargeError{Body: `{"detail":{"code":"checkpoint_transaction_too_large"}}`}}
	svc.WithWorkflowRuntime(runtime)
	conversation := seedWorkflowConversation(t, db)
	seedReadyMeetingTranscript(t, db, conversation, 88)
	created, err := svc.StartWorkflowAgent(ctx, conversation.OrganizationID, 7, WorkflowInput{
		ConversationID: conversation.ID,
		Preset:         WorkflowPresetMeetingBrief,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProcessWorkflowRun(ctx, created.Run.ID); !errors.Is(err, ErrCheckpointTransactionTooLarge) {
		t.Fatalf("expected checkpoint-too-large error when fallback disabled, got %v", err)
	}
	var stored models.WorkflowRun
	if err := db.Take(&stored, created.Run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.WorkflowRunStatusFailed {
		t.Fatalf("expected failed run when fallback disabled, got %s", stored.Status)
	}
}

func TestWorkflowTaskAttemptCannotFinishWithStaleRunLease(t *testing.T) {
	ctx := context.Background()
	svc, db := newWorkflowTestService(t)
	run := models.WorkflowRun{
		OrganizationID: 42, UserID: 7, ConversationID: 1,
		Status: models.WorkflowRunStatusRunning, RuntimeOwner: WorkflowRuntimePythonLangGraph,
		ExecutionLeaseToken: "lease:current",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	task := models.WorkflowTask{
		WorkflowRunID: run.ID, OrganizationID: run.OrganizationID, Name: models.WorkflowTaskCommitResult,
		Status: models.WorkflowTaskStatusRunning, Attempts: 2,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	staleRun := run
	staleRun.ExecutionLeaseToken = "lease:stale"
	staleTask := task
	staleTask.Attempts = 1
	if err := svc.finishWorkflowTaskAttempt(ctx, staleRun, staleTask, models.WorkflowTaskStatusFailed, nil, errors.New("late failure")); !errors.Is(err, ErrWorkflowRuntimeConflict) {
		t.Fatalf("expected stale task completion to be fenced, got %v", err)
	}
	var stored models.WorkflowTask
	if err := db.Take(&stored, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.WorkflowTaskStatusRunning || stored.Attempts != 2 || stored.ErrorMessage != "" {
		t.Fatalf("stale worker overwrote current task: %+v", stored)
	}
	if err := svc.finishWorkflowTaskAttempt(ctx, run, task, models.WorkflowTaskStatusReady, map[string]any{"ok": true}, nil); err != nil {
		t.Fatalf("current task owner could not complete: %v", err)
	}
}
