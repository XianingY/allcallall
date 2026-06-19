package interviewbench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

type Config struct {
	Conversations int
	BatchSize     int
	Provider      string
	KeepDB        bool
}

type Output struct {
	StartedAt           string           `json:"started_at"`
	Provider            string           `json:"provider"`
	Conversations       int              `json:"conversations"`
	QueuedRuns          int64            `json:"queued_runs"`
	ReadyRuns           int64            `json:"ready_runs"`
	FailedRuns          int64            `json:"failed_runs"`
	ProcessedEvents     int              `json:"processed_events"`
	PendingOutboxEvents int64            `json:"pending_outbox_events"`
	FailedOutboxEvents  int64            `json:"failed_outbox_events"`
	AgentSteps          int64            `json:"agent_steps"`
	AgentToolCalls      int64            `json:"agent_tool_calls"`
	SystemMessages      int64            `json:"system_messages"`
	FollowUpTasks       int64            `json:"follow_up_tasks"`
	AgentMemories       int64            `json:"agent_memories"`
	AgentContextChunks  int64            `json:"agent_context_chunks"`
	TotalDurationMs     int64            `json:"total_duration_ms"`
	QueueLatency        LatencyStats     `json:"queue_latency"`
	ExecuteRunLatency   LatencyStats     `json:"execute_run_latency"`
	Counters            map[string]int64 `json:"counters"`
	DatabasePath        string           `json:"database_path,omitempty"`
}

type LatencyStats struct {
	Count int   `json:"count"`
	MinMs int64 `json:"min_ms"`
	P50Ms int64 `json:"p50_ms"`
	P95Ms int64 `json:"p95_ms"`
	MaxMs int64 `json:"max_ms"`
}

func Run(ctx context.Context, cfg Config) (*Output, error) {
	cfg = normalizeConfig(cfg)
	started := time.Now().UTC()
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("allcallall-interview-bench-%d.db", started.UnixNano()))
	if !cfg.KeepDB {
		defer func() {
			_ = os.Remove(dbPath)
		}()
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrateTables(db); err != nil {
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}

	planner, err := agent.NewPlanner(cfg.Provider)
	if err != nil {
		return nil, fmt.Errorf("create planner: %w", err)
	}
	counters := metrics.NewCounterStore()
	agentSvc := agent.NewService(db, counters)
	agentSvc.WithPlanner(planner)
	processor := events.NewProcessor(events.NewStore(db), counters)
	processor.WithBatchSize(cfg.BatchSize)
	executeDurations := make([]time.Duration, 0, cfg.Conversations)
	registerHandlers(processor, agentSvc, &executeDurations)

	queueDurations := make([]time.Duration, 0, cfg.Conversations)
	if err := seedRuns(ctx, db, agentSvc, cfg.Conversations, &queueDurations); err != nil {
		return nil, err
	}

	processed, err := drainOutbox(ctx, processor)
	if err != nil {
		return nil, err
	}

	return buildOutput(ctx, db, started, cfg, processed, queueDurations, executeDurations, counters.Snapshot(), dbPath)
}

func normalizeConfig(cfg Config) Config {
	if cfg.Conversations <= 0 {
		cfg.Conversations = 25
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.Provider == "" {
		cfg.Provider = models.AgentRunSourceRules
	}
	return cfg
}

func migrateTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Conversation{},
		&models.ConversationMember{},
		&models.ConversationNote{},
		&models.Message{},
		&models.CallRoom{},
		&models.RecordingTranscription{},
		&models.MeetingTranscriptSegment{},
		&models.ContactProfile{},
		&models.FollowUpTask{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.AgentContextChunk{},
		&models.AgentPromptVersion{},
		&models.ToolSchemaVersion{},
		&models.EventOutbox{},
	)
}

func registerHandlers(processor *events.Processor, agentSvc *agent.Service, executeDurations *[]time.Duration) {
	processor.Register("agent.run.requested", func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			AgentRunID uint64 `json:"agent_run_id"`
		}
		if err := jsonUnmarshalEvent(event.PayloadJSON, &payload); err != nil {
			return err
		}
		started := time.Now()
		_, err := agentSvc.ExecuteRun(ctx, payload.AgentRunID)
		if err == nil {
			*executeDurations = append(*executeDurations, time.Since(started))
		}
		return err
	})
	processor.Register("agent.run.completed", func(context.Context, models.EventOutbox) error {
		return nil
	})
	processor.Register("message.created", func(context.Context, models.EventOutbox) error {
		return nil
	})
}

func seedRuns(ctx context.Context, db *gorm.DB, agentSvc *agent.Service, count int, queueDurations *[]time.Duration) error {
	for i := 0; i < count; i++ {
		actorID := uint64(1000 + i)
		contactID := uint64(2000 + i)
		conversation, err := seedConversation(ctx, db, uint64(1), actorID, contactID, i)
		if err != nil {
			return err
		}
		started := time.Now()
		if _, err := agentSvc.RunConversationAssistant(ctx, conversation.OrganizationID, actorID, agent.RunInput{
			ConversationID: conversation.ID,
			Goal:           "summarize support handoff and create next action",
			IdempotencyKey: fmt.Sprintf("interview-bench:%d", i),
		}); err != nil {
			return fmt.Errorf("queue agent run %d: %w", i, err)
		}
		*queueDurations = append(*queueDurations, time.Since(started))
	}
	return nil
}

func seedConversation(ctx context.Context, db *gorm.DB, organizationID, actorID, contactID uint64, index int) (*models.Conversation, error) {
	assigneeID := actorID
	conversation := models.Conversation{
		OrganizationID: organizationID,
		Type:           models.ConversationTypeChannel,
		Title:          fmt.Sprintf("Interview bench escalation %03d", index),
		Status:         models.ConversationStatusOpen,
		AssigneeUserID: &assigneeID,
		Priority:       models.ConversationPriorityHigh,
		ContactID:      &contactID,
		CreatedBy:      actorID,
	}
	if err := db.WithContext(ctx).Create(&conversation).Error; err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	if err := db.WithContext(ctx).Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         actorID,
		Role:           models.OrganizationRoleOwner,
	}).Error; err != nil {
		return nil, fmt.Errorf("create actor member: %w", err)
	}
	if err := db.WithContext(ctx).Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         contactID,
		Role:           models.OrganizationRoleMember,
	}).Error; err != nil {
		return nil, fmt.Errorf("create contact member: %w", err)
	}
	if err := db.WithContext(ctx).Create(&models.ContactProfile{
		OrganizationID:     organizationID,
		OwnerID:            actorID,
		ContactUserID:      contactID,
		Company:            "Interview Customer Co.",
		Role:               "Operations Lead",
		Timezone:           "Asia/Singapore",
		RelationshipStatus: "active",
	}).Error; err != nil {
		return nil, fmt.Errorf("create contact profile: %w", err)
	}
	if err := db.WithContext(ctx).Create(&models.ConversationNote{
		OrganizationID: organizationID,
		ConversationID: conversation.ID,
		AuthorID:       actorID,
		Body:           "Customer asked to schedule next call tomorrow and confirm owner before proposal.",
	}).Error; err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}
	if err := db.WithContext(ctx).Create(&models.Message{
		OrganizationID: organizationID,
		ConversationID: conversation.ID,
		SenderID:       contactID,
		Type:           models.MessageTypeText,
		Body:           "Please prepare risk summary before the next call.",
	}).Error; err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	room := models.CallRoom{
		OrganizationID: organizationID,
		ConversationID: &conversation.ID,
		Title:          fmt.Sprintf("Bench meeting %03d", index),
		Status:         models.RoomStatusEnded,
		CreatedBy:      actorID,
	}
	if err := db.WithContext(ctx).Create(&room).Error; err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	return &conversation, nil
}

func drainOutbox(ctx context.Context, processor *events.Processor) (int, error) {
	processedTotal := 0
	for i := 0; i < 100; i++ {
		processed, err := processor.ProcessOnce(ctx)
		if err != nil {
			return processedTotal, fmt.Errorf("process outbox: %w", err)
		}
		processedTotal += processed
		if processed == 0 {
			break
		}
	}
	return processedTotal, nil
}

func buildOutput(ctx context.Context, db *gorm.DB, started time.Time, cfg Config, processed int, queueDurations []time.Duration, executeDurations []time.Duration, counters map[string]int64, dbPath string) (*Output, error) {
	output := &Output{
		StartedAt:         started.Format(time.RFC3339Nano),
		Provider:          cfg.Provider,
		Conversations:     cfg.Conversations,
		ProcessedEvents:   processed,
		TotalDurationMs:   time.Since(started).Milliseconds(),
		QueueLatency:      summarizeLatency(queueDurations),
		ExecuteRunLatency: summarizeLatency(executeDurations),
		Counters:          counters,
	}
	if cfg.KeepDB {
		output.DatabasePath = dbPath
	}
	counts := []struct {
		dst   *int64
		model any
		where string
		args  []any
	}{
		{&output.QueuedRuns, &models.AgentRun{}, "", nil},
		{&output.ReadyRuns, &models.AgentRun{}, "status = ?", []any{models.AgentRunStatusReady}},
		{&output.FailedRuns, &models.AgentRun{}, "status = ?", []any{models.AgentRunStatusFailed}},
		{&output.PendingOutboxEvents, &models.EventOutbox{}, "status = ?", []any{models.EventOutboxStatusPending}},
		{&output.FailedOutboxEvents, &models.EventOutbox{}, "status = ?", []any{models.EventOutboxStatusFailed}},
		{&output.AgentSteps, &models.AgentStep{}, "", nil},
		{&output.AgentToolCalls, &models.AgentToolCall{}, "", nil},
		{&output.SystemMessages, &models.Message{}, "type = ?", []any{models.MessageTypeSystem}},
		{&output.FollowUpTasks, &models.FollowUpTask{}, "", nil},
		{&output.AgentMemories, &models.AgentMemory{}, "", nil},
		{&output.AgentContextChunks, &models.AgentContextChunk{}, "", nil},
	}
	for _, item := range counts {
		query := db.WithContext(ctx).Model(item.model)
		if item.where != "" {
			query = query.Where(item.where, item.args...)
		}
		if err := query.Count(item.dst).Error; err != nil {
			return nil, err
		}
	}
	return output, nil
}

func summarizeLatency(items []time.Duration) LatencyStats {
	if len(items) == 0 {
		return LatencyStats{}
	}
	values := make([]int64, len(items))
	for i, item := range items {
		values[i] = item.Milliseconds()
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i] < values[j]
	})
	return LatencyStats{
		Count: len(values),
		MinMs: values[0],
		P50Ms: percentile(values, 0.50),
		P95Ms: percentile(values, 0.95),
		MaxMs: values[len(values)-1],
	}
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	return values[index]
}

func jsonUnmarshalEvent(raw string, dst any) error {
	return json.Unmarshal([]byte(raw), dst)
}
