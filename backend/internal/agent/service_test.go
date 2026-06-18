package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

func newAgentServiceTestEnv(t *testing.T) (*Service, *gorm.DB, *metrics.CounterStore) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "agent.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Conversation{},
		&models.ConversationMember{},
		&models.ConversationNote{},
		&models.Message{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.AgentContextChunk{},
		&models.AgentPromptVersion{},
		&models.ToolSchemaVersion{},
		&models.FollowUpTask{},
		&models.CallRoom{},
		&models.CallFollowup{},
		&models.CallTranscriptSegment{},
		&models.ContactProfile{},
		&models.EventOutbox{},
		&models.ChatEvent{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	counters := metrics.NewCounterStore()
	return NewService(db, counters), db, counters
}

func seedAgentConversation(t *testing.T, db *gorm.DB) models.Conversation {
	t.Helper()

	assigneeID := uint64(7)
	if err := db.FirstOrCreate(&models.User{}, models.User{
		ID:           assigneeID,
		Email:        "agent-owner@example.com",
		PasswordHash: "hash",
		DisplayName:  "Agent Owner",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create agent user failed: %v", err)
	}
	conversation := models.Conversation{
		OrganizationID: 1,
		Type:           models.ConversationTypeChannel,
		Title:          "APAC onboarding escalation",
		Status:         models.ConversationStatusOpen,
		AssigneeUserID: &assigneeID,
		Priority:       models.ConversationPriorityHigh,
		CreatedBy:      7,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if err := db.Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         7,
		Role:           models.OrganizationRoleOwner,
	}).Error; err != nil {
		t.Fatalf("create conversation member failed: %v", err)
	}
	if err := db.Create(&models.ConversationNote{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		AuthorID:       7,
		Body:           "Customer asked to schedule next call tomorrow and confirm owner.",
	}).Error; err != nil {
		t.Fatalf("create conversation note failed: %v", err)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       7,
		Type:           models.MessageTypeText,
		Body:           "Please prepare risk summary before the next call.",
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	return conversation
}

func TestRunConversationAssistantQueuesAndExecutesExplainableRun(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	recorder := trace.NewMemorySpanRecorder()
	ctx := trace.WithSpanRecorder(trace.WithRequestID(context.Background(), "req-agent-queue-1"), recorder)
	queued, err := svc.RunConversationAssistant(ctx, conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "summarize current support handoff",
	})
	if err != nil {
		t.Fatalf("queue assistant failed: %v", err)
	}
	if queued.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("unexpected queued status: %s", queued.Run.Status)
	}
	if queued.Run.Goal != "summarize current support handoff" {
		t.Fatalf("unexpected goal: %q", queued.Run.Goal)
	}
	if queued.Run.RequestID != "req-agent-queue-1" {
		t.Fatalf("unexpected queued request id: %q", queued.Run.RequestID)
	}
	if len(queued.Steps) != 0 || len(queued.ToolCalls) != 0 {
		t.Fatalf("queued run should not have execution details yet: steps=%d tool_calls=%d", len(queued.Steps), len(queued.ToolCalls))
	}
	var requested models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", "agent.run.requested", queued.Run.ID).Take(&requested).Error; err != nil {
		t.Fatalf("load requested outbox event failed: %v", err)
	}
	if requested.RequestID != "req-agent-queue-1" {
		t.Fatalf("unexpected requested outbox request id: %q", requested.RequestID)
	}

	result, err := svc.ExecuteRun(ctx, queued.Run.ID)
	if err != nil {
		t.Fatalf("execute assistant failed: %v", err)
	}

	if result.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("unexpected run status: %s", result.Run.Status)
	}
	if result.Run.Source != models.AgentRunSourceRules {
		t.Fatalf("unexpected run source: %s", result.Run.Source)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("unexpected steps count: got=%d want=2", len(result.Steps))
	}
	if !strings.Contains(result.Steps[0].InputJSON, "planner_prompt") || !strings.Contains(result.Steps[0].InputJSON, "estimated_tokens") {
		t.Fatalf("collect_context step missing planner prompt metadata: %s", result.Steps[0].InputJSON)
	}
	if len(result.ToolCalls) != 7 {
		t.Fatalf("unexpected tool calls: %+v", result.ToolCalls)
	}
	toolNames := map[string]bool{}
	for _, toolCall := range result.ToolCalls {
		toolNames[toolCall.ToolName] = true
	}
	for _, name := range []string{ToolQueryRecentMeetings, ToolQueryConversationMembers, ToolQueryContactProfile, ToolQueryContextChunks, ToolWriteConversationMessage, ToolCreateFollowUpTask, ToolUpsertConversationMemory} {
		if !toolNames[name] {
			t.Fatalf("missing tool call %q in %+v", name, result.ToolCalls)
		}
	}
	if len(result.Trace) == 0 {
		t.Fatalf("expected agent trace timeline")
	}
	traceNames := map[string]bool{}
	for _, event := range result.Trace {
		traceNames[event.Name] = true
	}
	for _, name := range []string{"agent.run.created", "collect_context", ToolWriteConversationMessage, "agent.run.ready"} {
		if !traceNames[name] {
			t.Fatalf("missing trace event %q in %+v", name, result.Trace)
		}
	}
	if len(result.ActionItems) == 0 || len(result.RiskFlags) == 0 {
		t.Fatalf("expected action items and risk flags, got action_items=%v risk_flags=%v", result.ActionItems, result.RiskFlags)
	}
	if !strings.Contains(result.Run.NextStep, "下一次") {
		t.Fatalf("expected schedule-aware next step, got %q", result.Run.NextStep)
	}

	var systemMessage models.Message
	if err := db.Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Take(&systemMessage).Error; err != nil {
		t.Fatalf("load system message failed: %v", err)
	}
	if !strings.Contains(systemMessage.MetadataJSON, "agent.run.completed") {
		t.Fatalf("system message metadata missing agent event: %s", systemMessage.MetadataJSON)
	}
	var task models.FollowUpTask
	if err := db.Where("organization_id = ? AND user_id = ?", conversation.OrganizationID, 7).Take(&task).Error; err != nil {
		t.Fatalf("load follow-up task failed: %v", err)
	}
	if task.Type != models.FollowupTaskTypeScheduleNextCall {
		t.Fatalf("unexpected task type: %s", task.Type)
	}
	var memory models.AgentMemory
	if err := db.Where("organization_id = ? AND conversation_id = ? AND `key` = ?", conversation.OrganizationID, conversation.ID, "last_agent_summary").Take(&memory).Error; err != nil {
		t.Fatalf("load agent memory failed: %v", err)
	}
	var chunkCount int64
	if err := db.Model(&models.AgentContextChunk{}).Where("organization_id = ? AND conversation_id = ?", conversation.OrganizationID, conversation.ID).Count(&chunkCount).Error; err != nil {
		t.Fatalf("count context chunks failed: %v", err)
	}
	if chunkCount < 2 {
		t.Fatalf("expected notes/messages to be indexed as context chunks, got %d", chunkCount)
	}
	var ragToolCall models.AgentToolCall
	if err := db.Where("run_id = ? AND tool_name = ?", result.Run.ID, ToolQueryContextChunks).Take(&ragToolCall).Error; err != nil {
		t.Fatalf("load RAG context tool call failed: %v", err)
	}
	if !strings.Contains(ragToolCall.OutputJSON, `"chunks"`) {
		t.Fatalf("RAG tool output missing chunks: %s", ragToolCall.OutputJSON)
	}
	var completedOutbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_id = ?", "agent.run.completed", conversation.ID).Take(&completedOutbox).Error; err != nil {
		t.Fatalf("load outbox event failed: %v", err)
	}
	var messageOutbox models.EventOutbox
	if err := db.Where("event = ? AND aggregate_type = ? AND aggregate_id = ?", "message.created", "message", systemMessage.ID).Take(&messageOutbox).Error; err != nil {
		t.Fatalf("load message.created outbox event failed: %v", err)
	}

	snapshot := counters.Snapshot()
	if snapshot["agent_run_queued_total"] != 1 {
		t.Fatalf("agent_run_queued_total mismatch: %v", snapshot)
	}
	if snapshot["agent_run_started_total"] != 1 {
		t.Fatalf("agent_run_started_total mismatch: %v", snapshot)
	}
	if snapshot["agent_run_total"] != 1 {
		t.Fatalf("agent_run_total mismatch: %v", snapshot)
	}
	if snapshot["agent_planner_token_estimate_total"] <= 0 {
		t.Fatalf("agent_planner_token_estimate_total mismatch: %v", snapshot)
	}
	if _, ok := snapshot["agent_planner_latency_ms_total"]; !ok {
		t.Fatalf("agent_planner_latency_ms_total missing: %v", snapshot)
	}
	if snapshot["agent_tool_call_total"] != 7 {
		t.Fatalf("agent_tool_call_total mismatch: %v", snapshot)
	}
	if snapshot["agent_memory_write_total"] != 1 {
		t.Fatalf("agent_memory_write_total mismatch: %v", snapshot)
	}
	spanNames := map[string]bool{}
	for _, span := range recorder.Records() {
		spanNames[span.Name] = true
	}
	for _, name := range []string{"agent.execute_run", "agent.planner.plan", "agent.tools.execute_side_effects"} {
		if !spanNames[name] {
			t.Fatalf("missing span %q in %+v", name, recorder.Records())
		}
	}
}

func TestCompactSnippetKeepsUTF8Valid(t *testing.T) {
	got := compactSnippet("AI 协作助手已生成跟进建议", 8)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("snippet contains replacement rune: %q", got)
	}
	if len([]rune(got)) > 8 {
		t.Fatalf("snippet exceeds rune limit: %q", got)
	}
}

func TestConversationRAGIndexesBusinessSourcesAndReturnsCitations(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	contactID := uint64(8)
	callID := "call-rag-usable-1"

	if err := db.FirstOrCreate(&models.User{}, models.User{
		ID:           contactID,
		Email:        "buyer@example.com",
		PasswordHash: "hash",
		DisplayName:  "Buyer",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("create buyer failed: %v", err)
	}
	if err := db.Model(&models.Conversation{}).Where("id = ?", conversation.ID).Update("contact_id", contactID).Error; err != nil {
		t.Fatalf("bind contact failed: %v", err)
	}
	if err := db.Create(&models.ContactProfile{
		OrganizationID:     conversation.OrganizationID,
		OwnerID:            7,
		ContactUserID:      contactID,
		Company:            "Globex APAC",
		Role:               "Security approver",
		RelationshipStatus: "evaluating",
		Note:               "客户重点关注安全、数据留存、法务审批节奏和月底预算窗口。",
	}).Error; err != nil {
		t.Fatalf("create contact profile failed: %v", err)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       7,
		Type:           models.MessageTypeCallEvent,
		Body:           "Call completed with security and legal follow-up.",
		MetadataJSON:   mustJSONString(map[string]any{"call_id": callID, "event_type": "call.ended"}),
	}).Error; err != nil {
		t.Fatalf("create call event failed: %v", err)
	}
	if err := db.Create(&models.CallFollowup{
		CallID:          callID,
		OrganizationID:  conversation.OrganizationID,
		UserID:          7,
		PeerUserID:      contactID,
		Status:          models.FollowupStatusReady,
		Source:          "test",
		SummaryCN:       "客户要求补充安全说明、数据留存策略，并在月底预算窗口前完成法务审批。",
		ActionItemsJSON: mustJSONString([]string{"发送一页式安全说明", "安排技术答疑"}),
		NextStep:        "约技术答疑并同步法务审批材料。",
		RiskFlagsJSON:   mustJSONString([]string{"legal_approval_delay", "budget_window"}),
	}).Error; err != nil {
		t.Fatalf("create followup failed: %v", err)
	}
	if err := db.Create(&models.CallTranscriptSegment{
		CallID:         callID,
		UserID:         7,
		PeerUserID:     contactID,
		FromEmail:      "buyer@example.com",
		ToEmail:        "agent-owner@example.com",
		OriginalText:   "We need a clear data retention explanation before legal approval.",
		TranslatedText: "法务审批前需要清晰的数据留存说明。",
		SourceLang:     "en",
		TargetLang:     "zh",
		TimestampMS:    1200,
	}).Error; err != nil {
		t.Fatalf("create transcript failed: %v", err)
	}

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		Goal:           "客户的数据留存、安全和法务审批风险应该怎么推进？",
	})
	if err != nil {
		t.Fatalf("queue RAG run failed: %v", err)
	}
	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("execute RAG run failed: %v", err)
	}

	sourceTypes := map[string]bool{}
	for _, citation := range result.Citations {
		sourceTypes[citation.SourceType] = true
		if citation.Title == "" || citation.Snippet == "" {
			t.Fatalf("citation missing title/snippet: %+v", citation)
		}
	}
	for _, sourceType := range []string{contextChunkSourceFollowup, contextChunkSourceContactProfile, contextChunkSourceTranscript} {
		if !sourceTypes[sourceType] {
			t.Fatalf("missing citation source %s in %+v", sourceType, result.Citations)
		}
	}

	var chunks []models.AgentContextChunk
	if err := db.Where("conversation_id = ?", conversation.ID).Find(&chunks).Error; err != nil {
		t.Fatalf("load chunks failed: %v", err)
	}
	indexed := map[string]bool{}
	for _, chunk := range chunks {
		indexed[chunk.SourceType] = true
	}
	for _, sourceType := range []string{contextChunkSourceMessage, contextChunkSourceNote, contextChunkSourceFollowup, contextChunkSourceContactProfile, contextChunkSourceTranscript} {
		if !indexed[sourceType] {
			t.Fatalf("missing indexed source %s in %+v", sourceType, indexed)
		}
	}

	var ragToolCall models.AgentToolCall
	if err := db.Where("run_id = ? AND tool_name = ?", result.Run.ID, ToolQueryContextChunks).Take(&ragToolCall).Error; err != nil {
		t.Fatalf("load RAG tool call failed: %v", err)
	}
	if !strings.Contains(ragToolCall.OutputJSON, `"title"`) || !strings.Contains(ragToolCall.OutputJSON, `"created_at"`) {
		t.Fatalf("RAG tool output should include citation fields: %s", ragToolCall.OutputJSON)
	}

	var systemMessage models.Message
	if err := db.Where("conversation_id = ? AND type = ?", conversation.ID, models.MessageTypeSystem).Order("id DESC").Take(&systemMessage).Error; err != nil {
		t.Fatalf("load system message failed: %v", err)
	}
	if !strings.Contains(systemMessage.MetadataJSON, `"citations"`) || !strings.Contains(systemMessage.MetadataJSON, contextChunkSourceFollowup) {
		t.Fatalf("system message metadata missing citations: %s", systemMessage.MetadataJSON)
	}
}

func TestRunConversationAssistantIsIdempotentByKey(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	input := RunInput{ConversationID: conversation.ID, IdempotencyKey: "demo-key-1"}
	first, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, input)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	second, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, input)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if first.Run.ID != second.Run.ID {
		t.Fatalf("expected same run for idempotency key, got first=%d second=%d", first.Run.ID, second.Run.ID)
	}
	if first.Run.Status != models.AgentRunStatusPending || second.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("expected pending idempotent run, got first=%s second=%s", first.Run.Status, second.Run.Status)
	}

	var count int64
	if err := db.Model(&models.AgentRun{}).Where("idempotency_key = ?", "demo-key-1").Count(&count).Error; err != nil {
		t.Fatalf("count agent runs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one run for idempotency key, got %d", count)
	}
	if err := db.Model(&models.EventOutbox{}).Where("event = ? AND aggregate_id = ?", "agent.run.requested", first.Run.ID).Count(&count).Error; err != nil {
		t.Fatalf("count requested outbox events failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one requested outbox event, got %d", count)
	}

	executed, err := svc.ExecuteRun(context.Background(), first.Run.ID)
	if err != nil {
		t.Fatalf("execute idempotent run failed: %v", err)
	}
	replayed, err := svc.ExecuteRun(context.Background(), first.Run.ID)
	if err != nil {
		t.Fatalf("replay execute idempotent run failed: %v", err)
	}
	if executed.Run.ID != replayed.Run.ID || len(replayed.ToolCalls) != 7 {
		t.Fatalf("unexpected replayed run: executed=%d replayed=%d tool_calls=%d", executed.Run.ID, replayed.Run.ID, len(replayed.ToolCalls))
	}
	if err := db.Model(&models.AgentToolCall{}).Where("run_id = ?", first.Run.ID).Count(&count).Error; err != nil {
		t.Fatalf("count tool calls failed: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected exactly seven tool calls after repeated execute, got %d", count)
	}
}

func TestOutboxProcessorExecutesQueuedAgentRun(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	collaborationSvc := collaboration.NewService(db, nil)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "outbox-worker-demo",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}

	processor := events.NewProcessor(events.NewStore(db), counters)
	processor.Register("agent.run.requested", func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			AgentRunID uint64 `json:"agent_run_id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		_, err := svc.ExecuteRun(ctx, payload.AgentRunID)
		return err
	})
	processor.Register("agent.run.completed", func(context.Context, models.EventOutbox) error {
		return nil
	})
	processor.Register("message.created", func(ctx context.Context, event models.EventOutbox) error {
		return collaborationSvc.PublishMessageCreatedFromOutbox(ctx, event.AggregateID)
	})

	processed, err := processor.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process requested event failed: %v", err)
	}
	if processed != 1 {
		t.Fatalf("unexpected first process count: %d", processed)
	}
	result, err := svc.GetRun(context.Background(), conversation.OrganizationID, 7, queued.Run.ID)
	if err != nil {
		t.Fatalf("load executed run failed: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady || len(result.ToolCalls) != 7 {
		t.Fatalf("expected ready run after outbox worker, got status=%s tool_calls=%d", result.Run.Status, len(result.ToolCalls))
	}

	processed, err = processor.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("process completion events failed: %v", err)
	}
	if processed != 2 {
		t.Fatalf("expected completed and message events, got %d", processed)
	}
	var pending int64
	if err := db.Model(&models.EventOutbox{}).Where("status = ?", models.EventOutboxStatusPending).Count(&pending).Error; err != nil {
		t.Fatalf("count pending outbox failed: %v", err)
	}
	if pending != 0 {
		t.Fatalf("expected outbox drained, pending=%d", pending)
	}
	var replayEvents int64
	if err := db.Model(&models.ChatEvent{}).Where("event = ?", "message.created").Count(&replayEvents).Error; err != nil {
		t.Fatalf("count replay events failed: %v", err)
	}
	if replayEvents != 1 {
		t.Fatalf("expected agent system message to enter realtime replay, got %d events", replayEvents)
	}
}

type failOncePlanner struct {
	calls int
}

func (p *failOncePlanner) Name() string {
	return models.AgentRunSourceRules
}

func (p *failOncePlanner) Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error) {
	p.calls++
	if p.calls == 1 {
		return PlannerOutput{}, errors.New("temporary planner failure")
	}
	return RulesPlanner{}.Plan(ctx, input)
}

type blockingPlanner struct{}

func (blockingPlanner) Name() string {
	return models.AgentRunSourceRules
}

func (blockingPlanner) Plan(ctx context.Context, input PlannerInput) (PlannerOutput, error) {
	<-ctx.Done()
	return PlannerOutput{}, ctx.Err()
}

func TestExecuteRunRetriesFailedRun(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	planner := &failOncePlanner{}
	svc.WithPlanner(planner)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "retry-failed-run",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	if _, err := svc.ExecuteRun(context.Background(), queued.Run.ID); err == nil {
		t.Fatal("expected first execution to fail")
	}
	var failed models.AgentRun
	if err := db.Take(&failed, queued.Run.ID).Error; err != nil {
		t.Fatalf("load failed run failed: %v", err)
	}
	if failed.Status != models.AgentRunStatusFailed || failed.Attempts != 1 || failed.LeaseUntil != nil {
		t.Fatalf("unexpected failed run state: %+v", failed)
	}

	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("retry execution failed: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady || result.Run.Attempts != 2 {
		t.Fatalf("expected retry to complete run, got status=%s attempts=%d", result.Run.Status, result.Run.Attempts)
	}
}

func TestExecuteRunMarksPlannerTimeoutFailed(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	svc.WithPlanner(blockingPlanner{})

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "planner-timeout",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := svc.ExecuteRun(ctx, queued.Run.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected planner deadline exceeded, got %v", err)
	}

	var failed models.AgentRun
	if err := db.Take(&failed, queued.Run.ID).Error; err != nil {
		t.Fatalf("load timed out run failed: %v", err)
	}
	if failed.Status != models.AgentRunStatusFailed || failed.Attempts != 1 || failed.LeaseUntil != nil {
		t.Fatalf("expected failed run with cleared lease after planner timeout, got %+v", failed)
	}
	if !strings.Contains(failed.ErrorMessage, "context deadline exceeded") {
		t.Fatalf("expected timeout error message, got %q", failed.ErrorMessage)
	}
	snapshot := counters.Snapshot()
	if snapshot["agent_run_failed_total"] != 1 || snapshot["agent_planner_error_total"] != 1 {
		t.Fatalf("unexpected timeout metrics: %v", snapshot)
	}
}

func TestExecuteRunRecoversStaleRunningRun(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
		IdempotencyKey: "recover-stale-running",
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	staleLease := time.Now().UTC().Add(-time.Minute)
	if err := db.Model(&models.AgentRun{}).Where("id = ?", queued.Run.ID).Updates(map[string]any{
		"status":      models.AgentRunStatusRunning,
		"attempts":    1,
		"lease_until": staleLease,
	}).Error; err != nil {
		t.Fatalf("mark run stale failed: %v", err)
	}

	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("recover stale run failed: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady || result.Run.Attempts != 2 {
		t.Fatalf("expected stale run recovery, got status=%s attempts=%d", result.Run.Status, result.Run.Attempts)
	}
}

func TestExecuteRunFallsBackWhenPlannerUnavailable(t *testing.T) {
	svc, db, counters := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)
	planner, err := NewPlanner(models.AgentRunSourceOpenAICompatible)
	if err != nil {
		t.Fatalf("new planner failed: %v", err)
	}
	svc.WithPlanner(planner)

	queued, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 7, RunInput{
		ConversationID: conversation.ID,
	})
	if err != nil {
		t.Fatalf("queue run failed: %v", err)
	}
	result, err := svc.ExecuteRun(context.Background(), queued.Run.ID)
	if err != nil {
		t.Fatalf("expected rules fallback, got error: %v", err)
	}
	if result.Run.Status != models.AgentRunStatusReady {
		t.Fatalf("unexpected fallback run status: %s", result.Run.Status)
	}
	if !strings.Contains(result.Steps[1].InputJSON, `"fallback_source":"rules"`) {
		t.Fatalf("plan step missing fallback source: %s", result.Steps[1].InputJSON)
	}
	snapshot := counters.Snapshot()
	if snapshot["agent_planner_fallback_total"] != 1 {
		t.Fatalf("agent_planner_fallback_total mismatch: %v", snapshot)
	}
}

func TestRunConversationAssistantRejectsNonMember(t *testing.T) {
	svc, db, _ := newAgentServiceTestEnv(t)
	conversation := seedAgentConversation(t, db)

	_, err := svc.RunConversationAssistant(context.Background(), conversation.OrganizationID, 99, RunInput{
		ConversationID: conversation.ID,
	})
	if !errors.Is(err, ErrConversationAccessDenied) {
		t.Fatalf("expected access denied, got %v", err)
	}
}
