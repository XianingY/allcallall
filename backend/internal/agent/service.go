package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
	"github.com/allcallall/backend/internal/trace"
)

var (
	ErrConversationAccessDenied = errors.New("conversation access denied")
	ErrAgentRunNotFound         = errors.New("agent run not found")
)

const (
	agentRunMaxAttempts   = 3
	agentRunLeaseDuration = 5 * time.Minute
)

type counterRecorder interface {
	Inc(name string)
	Add(name string, delta int64)
}

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

type Service struct {
	db                 *gorm.DB
	metrics            counterRecorder
	planner            Planner
	outbox             *events.Store
	indexer            ChunkIndexer
	knowledgeRetriever KnowledgeRetriever
	streamPublisher    StreamPublisher
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
	Conversation       models.Conversation
	Notes              []models.ConversationNote
	Messages           []models.Message
	Rooms              []models.CallRoom
	Members            []models.ConversationMember
	Memories           []models.AgentMemory
	Followups          []models.CallFollowup
	TranscriptSegments []models.CallTranscriptSegment
	ContactProfile     *models.ContactProfile
	ContextChunks      []RetrievedContextChunk
	MeetingContext     meetingContextSummary
}

type meetingContextSummary struct {
	LatestCallID           string
	TranscriptSegmentCount int
	LatestTranscriptAt     *time.Time
	LatestFollowupPresent  bool
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

func NewService(db *gorm.DB, counters ...counterRecorder) *Service {
	var metrics counterRecorder
	if len(counters) > 0 {
		metrics = counters[0]
	}
	return &Service{
		db:      db,
		metrics: metrics,
		planner: RulesPlanner{},
		outbox:  events.NewStore(db),
	}
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

func (s *Service) WithOutbox(outbox *events.Store) {
	if outbox != nil {
		s.outbox = outbox
	}
}

func (s *Service) WithStreamPublisher(p StreamPublisher) *Service {
	s.streamPublisher = p
	return s
}

func (s *Service) RunConversationAssistant(ctx context.Context, organizationID, userID uint64, in RunInput) (*RunResult, error) {
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		goal = "summarize_conversation_next_steps"
	}
	role := strings.TrimSpace(in.Role)
	if role == "" {
		role = "primary"
	}
	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	if in.ConversationID == 0 {
		return nil, ErrConversationAccessDenied
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, in.ConversationID); err != nil {
		return nil, err
	}
	if err := s.ensureWorkflowMetadataRegistered(ctx); err != nil {
		return nil, err
	}
	if idempotencyKey != "" {
		if existing, err := s.findRunByIdempotencyKey(ctx, organizationID, userID, in.ConversationID, idempotencyKey); err != nil {
			return nil, err
		} else if existing != nil {
			return s.buildRunResult(ctx, *existing)
		}
	}

	run := models.AgentRun{
		OrganizationID:    organizationID,
		UserID:            userID,
		ConversationID:    in.ConversationID,
		IdempotencyKey:    idempotencyKey,
		RequestID:         trace.RequestID(ctx),
		Source:            s.planner.Name(),
		Role:              role,
		Status:            models.AgentRunStatusPending,
		PromptVersion:     CurrentWorkflowPromptVersion,
		ToolSchemaVersion: CurrentToolSchemaVersion,
		Goal:              goal,
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if s.outbox == nil {
			return nil
		}
		_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "agent_run",
			AggregateID:    run.ID,
			Event:          "agent.run.requested",
			IdempotencyKey: fmt.Sprintf("agent.run.requested:%d", run.ID),
			Payload: map[string]any{
				"organization_id": run.OrganizationID,
				"user_id":         run.UserID,
				"conversation_id": run.ConversationID,
				"agent_run_id":    run.ID,
				"source":          run.Source,
			},
		})
		if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_run_queued_total")
	}
	return s.buildRunResult(ctx, run)
}

func (s *Service) findRunByIdempotencyKey(ctx context.Context, organizationID, userID, conversationID uint64, key string) (*models.AgentRun, error) {
	var run models.AgentRun
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ? AND conversation_id = ? AND idempotency_key = ?", organizationID, userID, conversationID, key).
		Order("id ASC").
		Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (s *Service) GetRun(ctx context.Context, organizationID, userID, runID uint64) (*RunResult, error) {
	var run models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", runID, organizationID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentRunNotFound
		}
		return nil, err
	}
	if err := s.ensureConversationMember(ctx, organizationID, userID, run.ConversationID); err != nil {
		return nil, err
	}
	return s.buildRunResult(ctx, run)
}

func (s *Service) GetRunEvents(ctx context.Context, organizationID, userID, runID uint64) ([]RunEvent, error) {
	result, err := s.GetRun(ctx, organizationID, userID, runID)
	if err != nil {
		return nil, err
	}
	return BuildRunEvents(result), nil
}

func (s *Service) ExecuteRun(ctx context.Context, runID uint64) (*RunResult, error) {
	var run models.AgentRun
	if err := s.db.WithContext(ctx).Where("id = ?", runID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentRunNotFound
		}
		return nil, err
	}
	if run.Status == models.AgentRunStatusReady {
		return s.buildRunResult(ctx, run)
	}
	ctx, span := trace.StartSpan(ctx, "agent.execute_run", map[string]string{
		"agent_run_id":    fmt.Sprintf("%d", run.ID),
		"conversation_id": fmt.Sprintf("%d", run.ConversationID),
		"source":          run.Source,
	})

	startedAt := time.Now().UTC()
	leaseUntil := startedAt.Add(agentRunLeaseDuration)
	update := s.db.WithContext(ctx).Model(&models.AgentRun{}).
		Where(
			"id = ? AND (status = ? OR (status = ? AND attempts < ?) OR (status = ? AND (lease_until IS NULL OR lease_until <= ?)))",
			run.ID,
			models.AgentRunStatusPending,
			models.AgentRunStatusFailed,
			agentRunMaxAttempts,
			models.AgentRunStatusRunning,
			startedAt,
		).
		Updates(map[string]any{
			"status":        models.AgentRunStatusRunning,
			"attempts":      gorm.Expr("attempts + 1"),
			"started_at":    startedAt,
			"lease_until":   leaseUntil,
			"error_message": "",
			"completed_at":  nil,
			"updated_at":    startedAt,
		})
	if update.Error != nil {
		span.End(update.Error)
		return nil, update.Error
	}
	if update.RowsAffected == 0 {
		if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&run).Error; err != nil {
			span.End(err)
			return nil, err
		}
		result, err := s.buildRunResult(ctx, run)
		span.End(err)
		return result, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", run.ID).Take(&run).Error; err != nil {
		span.End(err)
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_run_started_total")
	}

	goal := strings.TrimSpace(run.Goal)
	if goal == "" {
		goal = "summarize_conversation_next_steps"
	}
	var result *RunResult
	var err error
	if s.planner.Name() == models.AgentRunSourceOpenAICompatible {
		result, err = s.executeReActRun(ctx, run, goal)
	} else {
		result, err = s.executeRulesRun(ctx, run, goal)
	}
	if err != nil {
		failedAt := time.Now().UTC()
		// Persist terminal state even when the execution context timed out or was canceled.
		_ = s.db.WithContext(context.WithoutCancel(ctx)).Model(&models.AgentRun{}).
			Where("id = ?", run.ID).
			Updates(map[string]any{
				"status":        models.AgentRunStatusFailed,
				"error_message": err.Error(),
				"completed_at":  failedAt,
				"lease_until":   nil,
			}).Error
		if s.metrics != nil {
			s.metrics.Inc("agent_run_failed_total")
		}
		span.End(err)
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.Inc("agent_run_total")
	}
	span.End(nil)
	return result, nil
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
	if errors.Is(err, ErrPlannerUnavailable) && source != models.AgentRunSourceRules {
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

func (s *Service) loadConversationContext(ctx context.Context, organizationID, userID, conversationID uint64, goal string) (*conversationContext, error) {
	var conv models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&conv).Error; err != nil {
		return nil, err
	}
	var notes []models.ConversationNote
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(20).
		Find(&notes).Error; err != nil {
		return nil, err
	}
	var messages []models.Message
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(50).
		Find(&messages).Error; err != nil {
		return nil, err
	}
	var rooms []models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("created_at DESC").
		Limit(3).
		Find(&rooms).Error; err != nil {
		return nil, err
	}
	var memories []models.AgentMemory
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Limit(10).
		Find(&memories).Error; err != nil {
		return nil, err
	}
	var members []models.ConversationMember
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	callIDs := extractCallIDsFromMessages(messages)
	var followups []models.CallFollowup
	if len(callIDs) > 0 {
		if err := s.db.WithContext(ctx).
			Where("call_id IN ? AND (organization_id = ? OR organization_id = 0)", callIDs, organizationID).
			Order("generated_at DESC, updated_at DESC").
			Limit(10).
			Find(&followups).Error; err != nil {
			return nil, err
		}
	}
	var transcriptSegments []models.CallTranscriptSegment
	if len(callIDs) > 0 {
		if err := s.db.WithContext(ctx).
			Where("call_id IN ?", callIDs).
			Order("timestamp_ms DESC, created_at DESC").
			Limit(40).
			Find(&transcriptSegments).Error; err != nil {
			return nil, err
		}
	}
	var contactProfile *models.ContactProfile
	if conv.ContactID != nil && *conv.ContactID != 0 {
		var profile models.ContactProfile
		if err := s.db.WithContext(ctx).
			Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", organizationID, userID, *conv.ContactID).
			Take(&profile).Error; err == nil {
			contactProfile = &profile
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	conversationCtx := &conversationContext{
		Conversation:       conv,
		Notes:              notes,
		Messages:           messages,
		Rooms:              rooms,
		Members:            members,
		Memories:           memories,
		Followups:          followups,
		TranscriptSegments: transcriptSegments,
		ContactProfile:     contactProfile,
	}
	conversationCtx.MeetingContext = buildMeetingContextSummary(conversationCtx.TranscriptSegments, conversationCtx.Followups)
	prioritizeMeetingConversationArtifacts(conversationCtx)
	if err := s.refreshConversationContextChunks(ctx, conversationCtx); err != nil {
		return nil, err
	}
	contextChunks, err := s.retrieveConversationContextChunks(ctx, conversationCtx, goal, defaultContextChunkLimit)
	if err != nil {
		return nil, err
	}
	conversationCtx.ContextChunks = contextChunks
	return conversationCtx, nil
}

func buildMeetingContextSummary(segments []models.CallTranscriptSegment, followups []models.CallFollowup) meetingContextSummary {
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
	return summary
}

func prioritizeMeetingConversationArtifacts(conversationCtx *conversationContext) {
	if conversationCtx == nil || strings.TrimSpace(conversationCtx.MeetingContext.LatestCallID) == "" {
		return
	}
	latestCallID := conversationCtx.MeetingContext.LatestCallID
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

func (s *Service) createStep(ctx context.Context, runID uint64, name string, input, output any) (models.AgentStep, error) {
	step := models.AgentStep{
		RunID:      runID,
		Name:       name,
		Status:     models.AgentRunStatusReady,
		InputJSON:  mustJSONString(input),
		OutputJSON: mustJSONString(output),
	}
	if err := s.db.WithContext(ctx).Create(&step).Error; err != nil {
		return step, err
	}
	return step, nil
}

func (s *Service) recordContextToolCalls(ctx context.Context, run models.AgentRun, conversationCtx *conversationContext) (int, error) {
	count := 0
	rooms := make([]map[string]any, 0, len(conversationCtx.Rooms))
	for _, room := range conversationCtx.Rooms {
		rooms = append(rooms, map[string]any{
			"room_id": room.ID,
			"title":   room.Title,
			"status":  room.Status,
		})
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolQueryRecentMeetings,
		Status:    models.AgentRunStatusReady,
		InputJSON: mustJSONString(map[string]any{"conversation_id": run.ConversationID, "limit": 3}),
		OutputJSON: mustJSONString(map[string]any{
			"rooms": rooms,
			"count": len(rooms),
		}),
	}); err != nil {
		return count, err
	}
	count++

	peerIDs := make([]uint64, 0, len(conversationCtx.Members))
	for _, member := range conversationCtx.Members {
		if member.UserID != run.UserID {
			peerIDs = append(peerIDs, member.UserID)
		}
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolQueryConversationMembers,
		Status:    models.AgentRunStatusReady,
		InputJSON: mustJSONString(map[string]any{"conversation_id": run.ConversationID}),
		OutputJSON: mustJSONString(map[string]any{
			"member_count":  len(conversationCtx.Members),
			"peer_user_ids": peerIDs,
		}),
	}); err != nil {
		return count, err
	}
	count++
	contactOutput := map[string]any{"status": "skipped", "reason": "conversation has no contact_id"}
	if conversationCtx.Conversation.ContactID != nil && *conversationCtx.Conversation.ContactID != 0 {
		var profile models.ContactProfile
		if err := s.db.WithContext(ctx).
			Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", run.OrganizationID, run.UserID, *conversationCtx.Conversation.ContactID).
			Take(&profile).Error; err == nil {
			contactOutput = map[string]any{
				"status":              "found",
				"contact_user_id":     profile.ContactUserID,
				"company":             profile.Company,
				"role":                profile.Role,
				"timezone":            profile.Timezone,
				"relationship_status": profile.RelationshipStatus,
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			contactOutput = map[string]any{"status": "not_found", "contact_user_id": *conversationCtx.Conversation.ContactID}
		} else {
			return count, err
		}
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:      run.ID,
		ToolName:   ToolQueryContactProfile,
		Status:     models.AgentRunStatusReady,
		InputJSON:  mustJSONString(map[string]any{"conversation_id": run.ConversationID, "contact_id": conversationCtx.Conversation.ContactID}),
		OutputJSON: mustJSONString(contactOutput),
	}); err != nil {
		return count, err
	}
	count++
	chunks := make([]map[string]any, 0, len(conversationCtx.ContextChunks))
	for _, item := range conversationCtx.ContextChunks {
		chunk := map[string]any{
			"chunk_id":       retrievedChunkID(item),
			"source_type":    retrievedChunkSourceType(item),
			"source_id":      retrievedChunkSourceID(item),
			"title":          retrievedChunkTitle(item),
			"score":          item.Score,
			"retrieval_mode": item.RetrievalMode,
			"snippet":        compactSnippet(retrievedChunkContent(item), 180),
			"created_at":     retrievedChunkUpdatedAt(item).Format(time.RFC3339),
		}
		if item.BM25Rank > 0 {
			chunk["bm25_rank"] = item.BM25Rank
		}
		if item.VectorRank > 0 {
			chunk["vector_rank"] = item.VectorRank
		}
		if item.RRFScore > 0 {
			chunk["rrf_score"] = item.RRFScore
		}
		if item.BM25Score > 0 {
			chunk["bm25_score"] = item.BM25Score
		}
		if item.VectorScore > 0 {
			chunk["vector_score"] = item.VectorScore
		}
		if item.FallbackReason != "" {
			chunk["fallback_reason"] = item.FallbackReason
		}
		if item.KnowledgeSource != nil {
			chunk["knowledge_source_id"] = item.KnowledgeSource.ID
			chunk["origin_type"] = item.KnowledgeSource.Kind
			chunk["origin_url"] = item.KnowledgeSource.URI
			chunk["source_title"] = item.KnowledgeSource.Title
		}
		if item.KnowledgeVersion != nil {
			chunk["version"] = item.KnowledgeVersion.Version
		}
		if item.KnowledgeChunk != nil && item.KnowledgeChunk.ConversationID != nil {
			chunk["conversation_id"] = *item.KnowledgeChunk.ConversationID
		}
		chunks = append(chunks, chunk)
	}
	if err := s.recordToolCall(ctx, models.AgentToolCall{
		RunID:     run.ID,
		ToolName:  ToolQueryContextChunks,
		Status:    models.AgentRunStatusReady,
		InputJSON: mustJSONString(map[string]any{"conversation_id": run.ConversationID, "query": run.Goal, "limit": defaultContextChunkLimit}),
		OutputJSON: mustJSONString(map[string]any{
			"chunks": chunks,
			"count":  len(chunks),
		}),
	}); err != nil {
		return count, err
	}
	count++
	return count, nil
}

func (s *Service) recordToolCall(ctx context.Context, toolCall models.AgentToolCall) error {
	if toolCall.Status == "" {
		toolCall.Status = models.AgentRunStatusReady
	}
	if toolCall.ToolSchemaVersion == "" {
		toolCall.ToolSchemaVersion = CurrentToolSchemaVersion
	}
	return s.db.WithContext(ctx).Create(&toolCall).Error
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

func (s *Service) buildRunResult(ctx context.Context, run models.AgentRun) (*RunResult, error) {
	var steps []models.AgentStep
	if err := s.db.WithContext(ctx).Where("run_id = ?", run.ID).Order("id ASC").Find(&steps).Error; err != nil {
		return nil, err
	}
	var toolCalls []models.AgentToolCall
	if err := s.db.WithContext(ctx).Where("run_id = ?", run.ID).Order("id ASC").Find(&toolCalls).Error; err != nil {
		return nil, err
	}
	return &RunResult{
		Run:         run,
		Steps:       steps,
		ToolCalls:   toolCalls,
		Trace:       buildTraceTimeline(run, steps, toolCalls),
		Citations:   buildCitationsFromToolCalls(toolCalls),
		ActionItems: decodeStringSlice(run.ActionItemsJSON),
		RiskFlags:   decodeStringSlice(run.RiskFlagsJSON),
	}, nil
}

func joinMessageBodies(messages []models.Message) string {
	items := make([]string, 0, len(messages))
	for _, message := range messages {
		items = append(items, message.Body)
	}
	return strings.Join(items, " ")
}

func compactSnippet(value string, max int) string {
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

func uniqueStrings(items []string) []string {
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
