package agent

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
	"github.com/allcallall/backend/internal/trace"
)

var (
	ErrConversationAccessDenied = errors.New("conversation access denied")
	ErrAgentRunNotFound         = errors.New("agent run not found")
)

func isDeferredRunExecution(err error) bool {
	return errors.Is(err, ErrCheckpointExecutionBusy) || errors.Is(err, mcpplatform.ErrExecutionInProgress)
}

const (
	agentRunMaxAttempts   = 3
	agentRunLeaseDuration = 5 * time.Minute
)

type ChunkIndexer interface {
	IndexChunk(ctx context.Context, doc search.ContextChunkDocument) error
	SearchChunks(ctx context.Context, query search.ContextChunkSearchQuery) ([]search.ContextChunkSearchResult, error)
}

type StreamPublisher interface {
	PublishToken(ctx context.Context, runID uint64, token string) error
}

type KnowledgeRetriever interface {
	Search(ctx context.Context, organizationID uint64, conversationID *uint64, query string, limit int) ([]knowledge.SearchResult, error)
}

type ToolCapabilityProvider interface {
	IssueForRun(ctx context.Context, organizationID, userID, conversationID uint64, runRef string) (string, error)
}

type Service struct {
	db                 *gorm.DB
	metrics            metrics.Recorder
	planner            Planner
	outbox             *events.Store
	indexer            ChunkIndexer
	knowledgeRetriever KnowledgeRetriever
	reranker           search.Reranker
	streamPublisher    StreamPublisher
	strictProvider     bool
	workflowRuntime    WorkflowRuntime
	toolCapabilities   ToolCapabilityProvider
	mcpPlatform        *mcpplatform.Service
}

type RunInput struct {
	ConversationID uint64
	Goal           string
	Role           string
	IdempotencyKey string
}

type RunResult struct {
	Run         models.AgentRun        `json:"run"`
	Steps       []models.AgentStep     `json:"steps"`
	ToolCalls   []models.AgentToolCall `json:"tool_calls"`
	Trace       []TraceEvent           `json:"trace"`
	Citations   []Citation             `json:"citations"`
	ActionItems []string               `json:"action_items"`
	RiskFlags   []string               `json:"risk_flags"`
}

type conversationContext struct {
	Conversation              models.Conversation
	Notes                     []models.ConversationNote
	Messages                  []models.Message
	Rooms                     []models.CallRoom
	Members                   []models.ConversationMember
	Memories                  []models.AgentMemory
	Followups                 []models.CallFollowup
	TranscriptSegments        []models.CallTranscriptSegment
	MeetingTranscriptSegments []models.MeetingTranscriptSegment
	ContactProfile            *models.ContactProfile
	ContextChunks             []RetrievedContextChunk
	MeetingContext            meetingContextSummary
}

type meetingContextSummary struct {
	LatestCallID                  string
	TranscriptSegmentCount        int
	LatestTranscriptAt            *time.Time
	LatestFollowupPresent         bool
	MeetingTranscriptionStatus    string
	MeetingTranscriptSegmentCount int
	LatestMeetingTranscriptAt     *time.Time
}

type conversationMemoryInput struct {
	Key         string
	Summary     string
	ActionItems []string
	NextStep    string
	RiskFlags   []string
	MemoryType  string
	Importance  int
	SourceType  string
	SourceRefID uint64
}

func NewService(db *gorm.DB, recorders ...metrics.Recorder) *Service {
	var metrics metrics.Recorder
	if len(recorders) > 0 {
		metrics = recorders[0]
	}
	reranker, _ := search.NewRerankerFromEnv()
	return &Service{
		db:              db,
		metrics:         metrics,
		planner:         RulesPlanner{},
		outbox:          events.NewStore(db),
		reranker:        reranker,
		strictProvider:  AgentProviderStrictFromEnv(),
		workflowRuntime: NewWorkflowRuntimeFromEnv(),
	}
}

func (s *Service) WithStrictProvider(strict bool) *Service {
	s.strictProvider = strict
	return s
}

func (s *Service) WithPlanner(p Planner) *Service {
	s.planner = p
	return s
}

func (s *Service) WithChunkIndexer(i ChunkIndexer) *Service {
	s.indexer = i
	return s
}

func (s *Service) WithKnowledgeRetriever(r KnowledgeRetriever) *Service {
	s.knowledgeRetriever = r
	return s
}

func (s *Service) WithReranker(r search.Reranker) *Service {
	s.reranker = r
	return s
}

func (s *Service) WithOutbox(outbox *events.Store) {
	if outbox != nil {
		s.outbox = outbox
	}
}

func (s *Service) WithStreamPublisher(p StreamPublisher) *Service {
	s.streamPublisher = p
	return s
}

func (s *Service) WithWorkflowRuntime(runtime WorkflowRuntime) *Service {
	s.workflowRuntime = runtime
	return s
}

func (s *Service) WithToolCapabilityProvider(provider ToolCapabilityProvider) *Service {
	s.toolCapabilities = provider
	return s
}

func (s *Service) WithMCPPlatform(platform *mcpplatform.Service) *Service {
	s.mcpPlatform = platform
	return s
}

func (s *Service) executeRulesRun(ctx context.Context, run models.AgentRun, goal string) (*RunResult, error) {
	conversationCtx, err := s.loadConversationContext(ctx, run.OrganizationID, run.UserID, run.ConversationID, goal)
	if err != nil {
		return nil, err
	}

	plannerInput := PlannerInput{
		Goal:         goal,
		Conversation: conversationCtx.Conversation,
		Notes:        conversationCtx.Notes,
		Messages:     conversationCtx.Messages,
		Rooms:        conversationCtx.Rooms,
		Members:      conversationCtx.Members,
		Memories:     conversationCtx.Memories,
	}
	plannerPrompt, err := buildPromptForPlanner(s.planner, plannerInput)
	if err != nil {
		return nil, err
	}
	collectStep, err := s.createStep(ctx, run.ID, "collect_context", map[string]any{
		"goal":            goal,
		"conversation_id": run.ConversationID,
		"planner_source":  s.planner.Name(),
		"planner_prompt":  plannerPrompt,
	}, map[string]any{
		"notes":                    len(conversationCtx.Notes),
		"messages":                 len(conversationCtx.Messages),
		"retrieved_context_chunks": len(conversationCtx.ContextChunks),
	})
	if err != nil {
		return nil, err
	}

	planStarted := time.Now()
	output, plannerSource, fallbackSource, err := s.planWithFallback(ctx, plannerInput)
	latencyMs := time.Since(planStarted).Milliseconds()
	if s.metrics != nil {
		s.metrics.Add("agent_planner_latency_ms_total", latencyMs)
		s.metrics.Add("agent_planner_token_estimate_total", int64(plannerPrompt.EstimatedTokens))
	}
	if err != nil {
		return nil, err
	}
	contextToolCalls, err := s.recordContextToolCalls(ctx, run, conversationCtx)
	if err != nil {
		return nil, err
	}
	s.recordAgentToolCalls(contextToolCalls)
	summary := output.Summary
	actionItems := output.ActionItems
	nextStep := output.NextStep
	riskFlags := output.RiskFlags
	if _, err := s.createStep(ctx, run.ID, "plan_next_actions", map[string]any{
		"step_id":         collectStep.ID,
		"planner_source":  plannerSource,
		"fallback_source": fallbackSource,
		"latency_ms":      latencyMs,
	}, map[string]any{
		"action_items": actionItems,
		"next_step":    nextStep,
		"risk_flags":   riskFlags,
	}); err != nil {
		return nil, err
	}

	if _, err := s.executeSideEffectTools(ctx, run, sideEffectToolInput{
		Summary:     summary,
		ActionItems: actionItems,
		NextStep:    nextStep,
		RiskFlags:   riskFlags,
		Citations:   buildCitationsFromContextChunks(conversationCtx.ContextChunks),
	}); err != nil {
		return nil, err
	}

	completedAt := time.Now().UTC()
	updates := map[string]any{
		"status":            models.AgentRunStatusReady,
		"summary":           summary,
		"action_items_json": mustJSONString(actionItems),
		"next_step":         nextStep,
		"risk_flags_json":   mustJSONString(riskFlags),
		"completed_at":      completedAt,
		"lease_until":       nil,
	}
	if err := s.db.WithContext(ctx).Model(&models.AgentRun{}).Where("id = ?", run.ID).Updates(updates).Error; err != nil {
		return nil, err
	}

	run.Status = models.AgentRunStatusReady
	run.Summary = summary
	run.ActionItemsJSON = mustJSONString(actionItems)
	run.NextStep = nextStep
	run.RiskFlagsJSON = mustJSONString(riskFlags)
	run.CompletedAt = &completedAt
	return s.buildRunResult(ctx, run)
}

func buildPromptForPlanner(planner Planner, input PlannerInput) (PlannerPrompt, error) {
	if prompting, ok := planner.(PromptingPlanner); ok {
		return prompting.BuildPrompt(input)
	}
	return BuildPlannerPrompt(input)
}

func (s *Service) planWithFallback(ctx context.Context, input PlannerInput) (PlannerOutput, string, string, error) {
	source := s.planner.Name()
	ctx, span := trace.StartSpan(ctx, "agent.planner.plan", map[string]string{
		"provider":        source,
		"conversation_id": fmt.Sprintf("%d", input.Conversation.ID),
	})
	output, err := s.planner.Plan(ctx, input)
	if err == nil {
		span.End(nil)
		return output, source, "", nil
	}
	if errors.Is(err, ErrPlannerUnavailable) && source != models.AgentRunSourceRules && !s.strictProvider {
		if s.metrics != nil {
			s.metrics.Inc("agent_planner_fallback_total")
		}
		output, fallbackErr := RulesPlanner{}.Plan(ctx, input)
		if fallbackErr == nil {
			span.End(nil)
			return output, source, models.AgentRunSourceRules, nil
		}
		span.End(fallbackErr)
		return PlannerOutput{}, source, models.AgentRunSourceRules, fallbackErr
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_planner_error_total")
	}
	span.End(err)
	return PlannerOutput{}, source, "", err
}

func (s *Service) ensureConversationMember(ctx context.Context, organizationID, userID, conversationID uint64) error {
	var count int64
	if err := s.db.WithContext(ctx).
		Table("conversations").
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.organization_id = ? AND conversations.id = ? AND conversation_members.user_id = ?", organizationID, conversationID, userID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrConversationAccessDenied
	}
	return nil
}

func buildMeetingContextSummary(segments []models.CallTranscriptSegment, followups []models.CallFollowup, meetingSegments []models.MeetingTranscriptSegment, transcription models.RecordingTranscription) meetingContextSummary {
	summary := meetingContextSummary{}
	if len(segments) > 0 {
		summary.LatestCallID = strings.TrimSpace(segments[0].CallID)
		if !segments[0].CreatedAt.IsZero() {
			at := segments[0].CreatedAt.UTC()
			summary.LatestTranscriptAt = &at
		}
	}
	if summary.LatestCallID == "" && len(followups) > 0 {
		summary.LatestCallID = strings.TrimSpace(followups[0].CallID)
	}
	if summary.LatestCallID != "" {
		for _, segment := range segments {
			if strings.TrimSpace(segment.CallID) == summary.LatestCallID {
				summary.TranscriptSegmentCount++
				if segment.CreatedAt.IsZero() {
					continue
				}
				at := segment.CreatedAt.UTC()
				if summary.LatestTranscriptAt == nil || at.After(*summary.LatestTranscriptAt) {
					summary.LatestTranscriptAt = &at
				}
			}
		}
		for _, followup := range followups {
			if strings.TrimSpace(followup.CallID) == summary.LatestCallID {
				summary.LatestFollowupPresent = true
				break
			}
		}
	}
	summary.MeetingTranscriptionStatus = strings.TrimSpace(transcription.Status)
	summary.MeetingTranscriptSegmentCount = len(meetingSegments)
	for _, segment := range meetingSegments {
		if segment.CreatedAt.IsZero() {
			continue
		}
		at := segment.CreatedAt.UTC()
		if summary.LatestMeetingTranscriptAt == nil || at.After(*summary.LatestMeetingTranscriptAt) {
			summary.LatestMeetingTranscriptAt = &at
		}
	}
	return summary
}

func prioritizeMeetingConversationArtifacts(conversationCtx *conversationContext) {
	if conversationCtx == nil {
		return
	}
	latestCallID := conversationCtx.MeetingContext.LatestCallID
	if strings.TrimSpace(latestCallID) != "" {
		sort.SliceStable(conversationCtx.TranscriptSegments, func(i, j int) bool {
			left := strings.TrimSpace(conversationCtx.TranscriptSegments[i].CallID) == latestCallID
			right := strings.TrimSpace(conversationCtx.TranscriptSegments[j].CallID) == latestCallID
			if left != right {
				return left
			}
			if conversationCtx.TranscriptSegments[i].TimestampMS != conversationCtx.TranscriptSegments[j].TimestampMS {
				return conversationCtx.TranscriptSegments[i].TimestampMS > conversationCtx.TranscriptSegments[j].TimestampMS
			}
			return conversationCtx.TranscriptSegments[i].CreatedAt.After(conversationCtx.TranscriptSegments[j].CreatedAt)
		})
		sort.SliceStable(conversationCtx.Followups, func(i, j int) bool {
			left := strings.TrimSpace(conversationCtx.Followups[i].CallID) == latestCallID
			right := strings.TrimSpace(conversationCtx.Followups[j].CallID) == latestCallID
			if left != right {
				return left
			}
			leftAt := latestFollowupTimestamp(conversationCtx.Followups[i])
			rightAt := latestFollowupTimestamp(conversationCtx.Followups[j])
			return leftAt.After(rightAt)
		})
	}
	sort.SliceStable(conversationCtx.MeetingTranscriptSegments, func(i, j int) bool {
		if conversationCtx.MeetingTranscriptSegments[i].RecordingSessionID != conversationCtx.MeetingTranscriptSegments[j].RecordingSessionID {
			return conversationCtx.MeetingTranscriptSegments[i].RecordingSessionID > conversationCtx.MeetingTranscriptSegments[j].RecordingSessionID
		}
		if conversationCtx.MeetingTranscriptSegments[i].StartMS != conversationCtx.MeetingTranscriptSegments[j].StartMS {
			return conversationCtx.MeetingTranscriptSegments[i].StartMS < conversationCtx.MeetingTranscriptSegments[j].StartMS
		}
		return conversationCtx.MeetingTranscriptSegments[i].CreatedAt.After(conversationCtx.MeetingTranscriptSegments[j].CreatedAt)
	})
	sort.SliceStable(conversationCtx.Memories, func(i, j int) bool {
		return meetingMemorySortWeight(conversationCtx.Memories[i].Key) > meetingMemorySortWeight(conversationCtx.Memories[j].Key)
	})
}

func latestFollowupTimestamp(followup models.CallFollowup) time.Time {
	if followup.GeneratedAt != nil && !followup.GeneratedAt.IsZero() {
		return followup.GeneratedAt.UTC()
	}
	if !followup.UpdatedAt.IsZero() {
		return followup.UpdatedAt.UTC()
	}
	return followup.CreatedAt.UTC()
}

func meetingMemorySortWeight(key string) int {
	switch strings.TrimSpace(key) {
	case models.AgentMemoryKeyLatestMeetingBrief:
		return 4
	case models.AgentMemoryKeyFollowUpCommitment:
		return 3
	case models.AgentMemoryKeyOpenRiskRegister:
		return 2
	case models.AgentMemoryKeyLastAgentSummary:
		return 1
	default:
		return 0
	}
}

func (s *Service) recordToolCall(ctx context.Context, toolCall models.AgentToolCall) error {
	if toolCall.Status == "" {
		toolCall.Status = models.AgentRunStatusReady
	}
	if toolCall.ToolSchemaVersion == "" {
		toolCall.ToolSchemaVersion = CurrentToolSchemaVersion
	}
	ensureToolCallID(&toolCall)
	expected := toolCall
	stored := toolCall
	if err := s.db.WithContext(ctx).
		Where("run_id = ? AND call_id = ?", toolCall.RunID, toolCall.CallID).
		Attrs(toolCall).
		FirstOrCreate(&stored).Error; err != nil {
		return err
	}
	if stored.ToolName != expected.ToolName || stored.Status != expected.Status ||
		stored.ToolSchemaVersion != expected.ToolSchemaVersion || stored.InputJSON != expected.InputJSON ||
		stored.OutputJSON != expected.OutputJSON || stored.ErrorMessage != expected.ErrorMessage ||
		!sameOptionalUint64(stored.StepID, expected.StepID) {
		return fmt.Errorf("%w: tool call %q does not match its persisted payload", ErrWorkflowRuntimeConflict, expected.CallID)
	}
	return nil
}

func sameOptionalUint64(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func ensureToolCallID(toolCall *models.AgentToolCall) {
	if toolCall == nil {
		return
	}
	callID := strings.TrimSpace(toolCall.CallID)
	if callID != "" && len(callID) <= 96 {
		toolCall.CallID = callID
		return
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", toolCall.RunID, callID, toolCall.ToolName, toolCall.InputJSON)))
	toolCall.CallID = fmt.Sprintf("agent:%d:%x", toolCall.RunID, digest[:12])
}

func (s *Service) writeConversationMessage(ctx context.Context, run models.AgentRun, summary string, actionItems []string, nextStep string, riskFlags []string, citations []Citation) (models.AgentToolCall, error) {
	input := map[string]any{
		"conversation_id": run.ConversationID,
		"event_type":      "agent.run.completed",
	}
	body := fmt.Sprintf("AI 协作助手已生成跟进建议：%s\n下一步：%s", summary, nextStep)
	message := models.Message{
		OrganizationID: run.OrganizationID,
		ConversationID: run.ConversationID,
		SenderID:       run.UserID,
		Type:           models.MessageTypeSystem,
		Body:           body,
		MetadataJSON: mustJSONString(map[string]any{
			"event_type":   "agent.run.completed",
			"agent_run_id": run.ID,
			"source":       run.Source,
			"action_items": actionItems,
			"next_step":    nextStep,
			"risk_flags":   riskFlags,
			"citations":    citations,
		}),
	}
	now := time.Now().UTC()
	toolCall := models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolWriteConversationMessage,
		Status:    models.AgentRunStatusRunning,
		InputJSON: mustJSONString(input),
	}
	ensureToolCallID(&toolCall)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Conversation{}).
			Where("id = ? AND organization_id = ?", run.ConversationID, run.OrganizationID).
			Updates(map[string]any{
				"last_message_at": now,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}
		if s.outbox != nil {
			if _, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "conversation",
				AggregateID:    run.ConversationID,
				Event:          "agent.run.completed",
				IdempotencyKey: fmt.Sprintf("agent.run.completed:%d", run.ID),
				Payload: map[string]any{
					"organization_id": run.OrganizationID,
					"conversation_id": run.ConversationID,
					"agent_run_id":    run.ID,
				},
			}); err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
				return err
			}
			if _, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "message",
				AggregateID:    message.ID,
				Event:          "message.created",
				IdempotencyKey: fmt.Sprintf("message.created:%d", message.ID),
				Payload: map[string]any{
					"organization_id": run.OrganizationID,
					"conversation_id": run.ConversationID,
					"message_id":      message.ID,
					"sender_id":       run.UserID,
					"type":            message.Type,
					"source":          "agent",
				},
			}); err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
				return err
			}
		}
		toolCall.Status = models.AgentRunStatusReady
		toolCall.OutputJSON = mustJSONString(map[string]any{"message_id": message.ID})
		return tx.Create(&toolCall).Error
	}); err != nil {
		toolCall.Status = models.AgentRunStatusFailed
		toolCall.ErrorMessage = err.Error()
		return toolCall, err
	}
	return toolCall, nil
}

func (s *Service) createFollowUpTask(ctx context.Context, run models.AgentRun, nextStep string) (models.AgentToolCall, error) {
	peerUserID := run.UserID
	var member models.ConversationMember
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id <> ?", run.ConversationID, run.UserID).
		Order("id ASC").
		Take(&member).Error; err == nil {
		peerUserID = member.UserID
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.AgentToolCall{}, err
	}

	taskType := models.FollowupTaskTypeSendMessage
	if strings.Contains(nextStep, "会议") || strings.Contains(strings.ToLower(nextStep), "call") {
		taskType = models.FollowupTaskTypeScheduleNextCall
	}
	dueAt := time.Now().UTC().Add(24 * time.Hour)
	task := models.FollowUpTask{
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		PeerUserID:     peerUserID,
		Type:           taskType,
		Status:         models.FollowupTaskStatusOpen,
		Title:          "Agent 建议跟进",
		Description:    nextStep,
		DueAt:          &dueAt,
	}
	input := map[string]any{
		"conversation_id": run.ConversationID,
		"task_type":       taskType,
	}
	toolCall := models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolCreateFollowUpTask,
		Status:    models.AgentRunStatusRunning,
		InputJSON: mustJSONString(input),
	}
	ensureToolCallID(&toolCall)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		toolCall.Status = models.AgentRunStatusReady
		toolCall.OutputJSON = mustJSONString(map[string]any{"task_id": task.ID})
		return tx.Create(&toolCall).Error
	}); err != nil {
		toolCall.Status = models.AgentRunStatusFailed
		toolCall.ErrorMessage = err.Error()
		return toolCall, err
	}
	return toolCall, nil
}

func (s *Service) upsertConversationMemory(ctx context.Context, run models.AgentRun, input conversationMemoryInput) (models.AgentToolCall, error) {
	key := strings.TrimSpace(input.Key)
	if key == "" {
		key = models.AgentMemoryKeyLastAgentSummary
	}
	metadata := normalizeConversationMemoryInput(key, input, run.ID)
	value := map[string]any{
		"summary":      metadata.Summary,
		"action_items": metadata.ActionItems,
		"next_step":    metadata.NextStep,
		"risk_flags":   metadata.RiskFlags,
	}
	toolCall := models.AgentToolCall{
		RunID:    run.ID,
		ToolName: ToolUpsertConversationMemory,
		Status:   models.AgentRunStatusRunning,
		InputJSON: mustJSONString(map[string]any{
			"conversation_id": run.ConversationID,
			"key":             metadata.Key,
		}),
	}
	ensureToolCallID(&toolCall)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var memory models.AgentMemory
		err := tx.Where("organization_id = ? AND user_id = ? AND conversation_id = ? AND `key` = ?", run.OrganizationID, run.UserID, run.ConversationID, metadata.Key).
			Assign(models.AgentMemory{
				Scope:       models.AgentMemoryScopeConversation,
				MemoryType:  metadata.MemoryType,
				Importance:  metadata.Importance,
				SourceType:  metadata.SourceType,
				SourceRefID: metadata.SourceRefID,
				ValueJSON:   mustJSONString(value),
				LastRunID:   run.ID,
			}).
			FirstOrCreate(&memory, models.AgentMemory{
				OrganizationID: run.OrganizationID,
				UserID:         run.UserID,
				ConversationID: run.ConversationID,
				Key:            metadata.Key,
			}).Error
		if err != nil {
			return err
		}
		toolCall.Status = models.AgentRunStatusReady
		toolCall.OutputJSON = mustJSONString(map[string]any{"memory_id": memory.ID})
		return tx.Create(&toolCall).Error
	}); err != nil {
		toolCall.Status = models.AgentRunStatusFailed
		toolCall.ErrorMessage = err.Error()
		return toolCall, err
	}
	return toolCall, nil
}

func normalizeConversationMemoryInput(key string, input conversationMemoryInput, sourceRefID uint64) conversationMemoryInput {
	input.Key = key
	if strings.TrimSpace(input.MemoryType) == "" {
		switch key {
		case models.AgentMemoryKeyOpenRiskRegister:
			input.MemoryType = models.AgentMemoryTypeRisk
		case models.AgentMemoryKeyFollowUpCommitment:
			input.MemoryType = models.AgentMemoryTypeFollowUp
		default:
			input.MemoryType = models.AgentMemoryTypeSummary
		}
	}
	if input.Importance <= 0 {
		switch key {
		case models.AgentMemoryKeyLatestMeetingBrief:
			input.Importance = 90
		case models.AgentMemoryKeyOpenRiskRegister:
			input.Importance = 85
		case models.AgentMemoryKeyFollowUpCommitment:
			input.Importance = 80
		default:
			input.Importance = 70
		}
	}
	if strings.TrimSpace(input.SourceType) == "" {
		input.SourceType = "workflow_run"
	}
	if input.SourceRefID == 0 {
		input.SourceRefID = sourceRefID
	}
	return input
}

func joinMessageBodies(messages []models.Message) string {
	items := make([]string, 0, len(messages))
	for _, message := range messages {
		items = append(items, message.Body)
	}
	return strings.Join(items, " ")
}

func CompactSnippet(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func UniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func mustJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

func extractCallIDsFromMessages(messages []models.Message) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, message := range messages {
		if message.Type != models.MessageTypeCallEvent || strings.TrimSpace(message.MetadataJSON) == "" {
			continue
		}
		var metadata map[string]any
		if err := json.Unmarshal([]byte(message.MetadataJSON), &metadata); err != nil {
			continue
		}
		callID, _ := metadata["call_id"].(string)
		callID = strings.TrimSpace(callID)
		if callID == "" {
			continue
		}
		if _, ok := seen[callID]; ok {
			continue
		}
		seen[callID] = struct{}{}
		out = append(out, callID)
	}
	return out
}

// DB returns the underlying gorm.DB.
func (s *Service) DB() *gorm.DB {
	return s.db
}
