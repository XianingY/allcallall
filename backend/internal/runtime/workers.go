package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/mcpplatform"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/mq"
	"github.com/allcallall/backend/internal/search"
	"github.com/allcallall/backend/internal/settlement"
	"github.com/allcallall/backend/internal/trace"
)

const (
	EventAgentRunRequested               = "agent.run.requested"
	EventWorkflowRequested               = agent.EventWorkflowRunRequested
	EventMCPExecutionTerminal            = mcpplatform.EventMCPExecutionTerminal
	EventAgentRunCompleted               = "agent.run.completed"
	EventMessageCreated                  = "message.created"
	EventSearchMessageIndex              = "search.message.index_requested"
	EventRAGSourceIngest                 = knowledge.EventSourceIngestRequested
	EventRAGChunkIndex                   = knowledge.EventChunkIndexRequested
	EventSettlementRoomEnd               = "settlement.room.ended"
	EventRecordingTranscriptionRequested = collaboration.EventRecordingTranscriptionRequested
	EventWeeklyTaskTriggered             = "weekly_task.triggered"
	EventChatMessageCreated              = events.EventChatMessageCreated
)

func EmbeddedWorkersEnabledFromEnv() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("EMBEDDED_WORKERS")))
	return raw == "" || raw == "1" || raw == "true" || raw == "yes"
}

func RegisterSearchOutboxHandlers(processor *events.Processor, collaborationSvc *collaboration.Service, searchSvc *search.Service, log zerolog.Logger) {
	if processor == nil || collaborationSvc == nil || searchSvc == nil {
		return
	}
	processor.Register(EventSearchMessageIndex, func(ctx context.Context, event models.EventOutbox) error {
		messageID := event.AggregateID
		if messageID == 0 {
			var payload struct {
				MessageID uint64 `json:"message_id"`
			}
			if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
				return err
			}
			messageID = payload.MessageID
		}
		if messageID == 0 {
			return fmt.Errorf("message id missing in search payload")
		}
		doc, err := collaborationSvc.BuildMessageSearchDocument(ctx, messageID)
		if err != nil {
			return err
		}
		if err := searchSvc.IndexMessage(ctx, doc); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("message_id", messageID).
			Msg("outbox message indexed for search")
		return nil
	})
}

func RegisterSettlementKafkaOutboxHandlers(processor *events.Processor, settlementSvc *settlement.Service, log zerolog.Logger) {
	if processor == nil || settlementSvc == nil {
		return
	}
	processor.Register(EventSettlementRoomEnd, func(ctx context.Context, event models.EventOutbox) error {
		var payload settlement.RoomEndedEvent
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		if err := settlementSvc.PublishRoomEnded(ctx, payload); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("room_id", payload.RoomID).
			Uint64("user_id", payload.UserID).
			Msg("outbox room settlement published to kafka")
		return nil
	})
}

// RegisterEventsKafkaBridge 把领域事件生产化到 Kafka（当 producer 非 nil）。
//   - weekly_task.triggered 始终注册处理（记录日志；有 producer 时发布到 Kafka），
//     以统一接管 main 中内联的仅日志 handler，保证事件总线始终有 handler、不会被判失败。
//   - 仅当 cfg.BridgeChat 且 producer 非 nil 时，注册 chat.message.created -> Kafka。
//
// 注意：outbox processor 每个事件仅支持一个 handler，故 weekly_task.triggered 在此统一处理。
func RegisterEventsKafkaBridge(processor *events.Processor, producer mq.Producer, cfg config.EventsConfig, log zerolog.Logger) {
	if processor == nil {
		return
	}
	topicFor := func(name string) string {
		prefix := strings.TrimSpace(cfg.TopicPrefix)
		if prefix == "" {
			return name
		}
		return prefix + "." + name
	}
	producerEnabled := producer != nil

	processor.Register(EventWeeklyTaskTriggered, func(ctx context.Context, event models.EventOutbox) error {
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("task_id", event.AggregateID).
			Msg("weekly task triggered (event delivered)")
		if !producerEnabled {
			return nil
		}
		payload, err := json.Marshal(map[string]any{
			"event":   event.Event,
			"task_id": event.AggregateID,
			"payload": json.RawMessage(event.PayloadJSON),
		})
		if err != nil {
			return err
		}
		msg := mq.Message{
			Key:     []byte(strconv.FormatUint(event.AggregateID, 10)),
			Value:   payload,
			Headers: map[string]string{"event": event.Event},
		}
		if err := producer.Publish(ctx, topicFor("weekly_task"), msg); err != nil {
			return err
		}
		log.Info().Uint64("task_id", event.AggregateID).Str("topic", topicFor("weekly_task")).Msg("weekly_task event published to kafka")
		return nil
	})

	if cfg.BridgeChat && producerEnabled {
		processor.Register(EventChatMessageCreated, func(ctx context.Context, event models.EventOutbox) error {
			payload, err := json.Marshal(map[string]any{
				"event":   event.Event,
				"payload": json.RawMessage(event.PayloadJSON),
			})
			if err != nil {
				return err
			}
			msg := mq.Message{
				Key:     []byte(strconv.FormatUint(event.AggregateID, 10)),
				Value:   payload,
				Headers: map[string]string{"event": event.Event},
			}
			if err := producer.Publish(ctx, topicFor("chat_message"), msg); err != nil {
				return err
			}
			log.Info().Uint64("message_id", event.AggregateID).Str("topic", topicFor("chat_message")).Msg("chat.message.created event published to kafka")
			return nil
		})
	}
}

func RegisterAgentOutboxHandlers(processor *events.Processor, agentSvc *agent.Service, log zerolog.Logger) {
	if processor == nil || agentSvc == nil {
		return
	}
	processor.Register(EventAgentRunRequested, func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			AgentRunID uint64 `json:"agent_run_id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		if payload.AgentRunID == 0 {
			return fmt.Errorf("agent run id missing in outbox payload")
		}
		if _, err := agentSvc.ExecuteRun(ctx, payload.AgentRunID); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("agent_run_id", payload.AgentRunID).
			Msg("outbox agent run executed")
		return nil
	})
	processor.Register(EventWorkflowRequested, func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			WorkflowRunID uint64 `json:"workflow_run_id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		if payload.WorkflowRunID == 0 {
			return fmt.Errorf("workflow run id missing in outbox payload")
		}
		if _, err := agentSvc.ProcessWorkflowRun(ctx, payload.WorkflowRunID); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("workflow_run_id", payload.WorkflowRunID).
			Msg("outbox workflow run executed")
		return nil
	})
	processor.Register(EventMCPExecutionTerminal, func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			ExecutionID     string  `json:"execution_id"`
			MCPExecutionID  uint64  `json:"mcp_execution_id"`
			AgentRunID      *uint64 `json:"agent_run_id"`
			WorkflowRunID   *uint64 `json:"workflow_run_id"`
			ExecutionStatus string  `json:"status"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		if payload.MCPExecutionID == 0 {
			payload.MCPExecutionID = event.AggregateID
		}
		if payload.MCPExecutionID == 0 {
			return fmt.Errorf("MCP execution id missing in terminal outbox payload")
		}
		if err := agentSvc.RequeueParentRunAfterMCPExecution(ctx, agent.MCPExecutionTerminalInput{
			ExecutionID:   payload.ExecutionID,
			AgentRunID:    payload.AgentRunID,
			WorkflowRunID: payload.WorkflowRunID,
		}, time.Now().UTC()); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("mcp_execution_id", payload.MCPExecutionID).
			Str("execution_id", payload.ExecutionID).
			Str("status", payload.ExecutionStatus).
			Msg("outbox MCP terminal execution scheduled parent recovery")
		return nil
	})
}

func RegisterKnowledgeOutboxHandlers(processor *events.Processor, knowledgeSvc *knowledge.Service, log zerolog.Logger) {
	if processor == nil || knowledgeSvc == nil {
		return
	}
	processor.Register(EventRAGSourceIngest, func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			SourceID uint64 `json:"source_id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		if payload.SourceID == 0 {
			return fmt.Errorf("rag source id missing in outbox payload")
		}
		if err := knowledgeSvc.ProcessSourceIngest(ctx, payload.SourceID); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("source_id", payload.SourceID).
			Msg("outbox rag source ingested")
		return nil
	})
	processor.Register(EventRAGChunkIndex, func(ctx context.Context, event models.EventOutbox) error {
		var payload struct {
			ChunkID uint64 `json:"chunk_id"`
		}
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return err
		}
		if payload.ChunkID == 0 {
			return fmt.Errorf("rag chunk id missing in outbox payload")
		}
		if err := knowledgeSvc.ProcessChunkIndex(ctx, payload.ChunkID); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("chunk_id", payload.ChunkID).
			Msg("outbox rag chunk indexed")
		return nil
	})
}

func RegisterCollaborationOutboxHandlers(processor *events.Processor, collaborationSvc *collaboration.Service, log zerolog.Logger) {
	if processor == nil || collaborationSvc == nil {
		return
	}
	processor.Register(EventAgentRunCompleted, func(ctx context.Context, event models.EventOutbox) error {
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("aggregate_id", event.AggregateID).
			Str("event", event.Event).
			Msg("outbox agent event observed")
		return nil
	})
	processor.Register(EventMessageCreated, func(ctx context.Context, event models.EventOutbox) error {
		messageID := event.AggregateID
		if messageID == 0 {
			var payload struct {
				MessageID uint64 `json:"message_id"`
			}
			if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
				return err
			}
			messageID = payload.MessageID
		}
		if messageID == 0 {
			return fmt.Errorf("message id missing in outbox payload")
		}
		if err := collaborationSvc.PublishMessageCreatedFromOutbox(ctx, messageID); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("message_id", messageID).
			Str("event", event.Event).
			Msg("outbox message realtime delivered")
		return nil
	})
	processor.Register(EventRecordingTranscriptionRequested, func(ctx context.Context, event models.EventOutbox) error {
		recordingID := event.AggregateID
		if recordingID == 0 {
			var payload collaboration.RecordingTranscriptionRequestedPayload
			if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
				return err
			}
			recordingID = payload.RecordingID
		}
		if recordingID == 0 {
			return fmt.Errorf("recording id missing in transcription payload")
		}
		if err := collaborationSvc.ProcessRecordingTranscription(ctx, recordingID); err != nil {
			return err
		}
		log.Info().
			Str("request_id", trace.RequestID(ctx)).
			Uint64("outbox_id", event.ID).
			Uint64("recording_id", recordingID).
			Str("event", event.Event).
			Msg("outbox recording transcription processed")
		return nil
	})
}

func ConfigureOutboxProcessorFromEnv(processor *events.Processor, workerID string, eventFilter ...string) {
	if processor == nil {
		return
	}
	processor.WithEventFilter(eventFilter...)
	processor.WithWorker(workerID, durationFromEnv("OUTBOX_WORKER_LEASE_SEC", 120)*time.Second)
	processor.WithBatchSize(intFromEnv("OUTBOX_WORKER_BATCH_SIZE", 100))
	processor.WithRetry(
		intFromEnv("OUTBOX_WORKER_MAX_ATTEMPTS", 3),
		durationFromEnv("OUTBOX_WORKER_RETRY_DELAY_SEC", 60)*time.Second,
	)
}

func StartOutboxWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	if processor == nil {
		return
	}
	intervalSeconds := intFromEnv("OUTBOX_WORKER_INTERVAL_SEC", 30)
	interval := time.Duration(intervalSeconds) * time.Second
	log.Info().
		Int("interval_sec", intervalSeconds).
		Msg("outbox worker enabled")
	go processor.Run(ctx, interval)
}

func StartAgentWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor, services ...*agent.Service) {
	ConfigureOutboxProcessorFromEnv(processor, workerIDFromEnv("agent-worker"), EventAgentRunRequested, EventWorkflowRequested, EventMCPExecutionTerminal)
	StartOutboxWorker(ctx, log.With().Str("worker", "agent").Logger(), processor)
	if len(services) > 0 {
		StartAgentRecoveryWorker(ctx, log, services[0])
	}
}

func StartAgentRecoveryWorker(ctx context.Context, log zerolog.Logger, agentSvc *agent.Service) {
	if agentSvc == nil {
		return
	}
	intervalSeconds := intFromEnv("AGENT_RECOVERY_SWEEP_INTERVAL_SEC", 30)
	batchSize := intFromEnv("AGENT_RECOVERY_SWEEP_BATCH_SIZE", 100)
	interval := time.Duration(intervalSeconds) * time.Second
	log.Info().
		Int("interval_sec", intervalSeconds).
		Int("batch_size", batchSize).
		Msg("agent run recovery sweep enabled")
	go runTicker(ctx, interval, func() {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		result, err := agentSvc.RequeueExpiredAgentAndWorkflowRuns(runCtx, time.Now().UTC(), batchSize)
		if err != nil {
			log.Error().Err(err).Msg("agent run recovery sweep failed")
			return
		}
		if result.AgentRuns > 0 || result.WorkflowRuns > 0 {
			log.Info().
				Int("agent_runs", result.AgentRuns).
				Int("workflow_runs", result.WorkflowRuns).
				Msg("agent run recovery sweep scheduled expired runs")
		}
	})
}

func StartCollaborationOutboxWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	ConfigureOutboxProcessorFromEnv(processor, workerIDFromEnv("outbox-worker"), EventAgentRunCompleted, EventMessageCreated, EventRecordingTranscriptionRequested)
	StartOutboxWorker(ctx, log.With().Str("worker", "outbox").Logger(), processor)
}

func StartSearchOutboxWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	ConfigureOutboxProcessorFromEnv(processor, workerIDFromEnv("search-worker"), EventSearchMessageIndex)
	StartOutboxWorker(ctx, log.With().Str("worker", "search").Logger(), processor)
}

func StartSettlementBridgeWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	ConfigureOutboxProcessorFromEnv(processor, workerIDFromEnv("settlement-bridge"), EventSettlementRoomEnd)
	StartOutboxWorker(ctx, log.With().Str("worker", "settlement-bridge").Logger(), processor)
}

func StartCleanupWorker(ctx context.Context, log zerolog.Logger, collaborationSvc *collaboration.Service, refreshSessions *auth.RefreshSessionService) {
	StartRecordingCleanupWorker(ctx, log, collaborationSvc)
	StartRefreshSessionCleanupWorker(ctx, log, refreshSessions)
}

func StartRecordingCleanupWorker(ctx context.Context, log zerolog.Logger, collaborationSvc *collaboration.Service) {
	if collaborationSvc == nil {
		return
	}
	intervalMinutes := intFromEnv("RECORDING_CLEANUP_INTERVAL_MIN", 60)
	interval := time.Duration(intervalMinutes) * time.Minute
	log.Info().
		Int("interval_min", intervalMinutes).
		Msg("recording cleanup worker enabled")
	go runTicker(ctx, interval, func() {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		result, err := collaborationSvc.CleanupExpiredRecordings(runCtx, time.Now(), 200)
		if err != nil {
			log.Error().Err(err).Msg("recording cleanup worker failed")
			return
		}
		if result.Deleted > 0 {
			log.Info().
				Int("checked", result.Checked).
				Int("deleted", result.Deleted).
				Msg("recording cleanup worker completed")
		}
	})
}

func StartRefreshSessionCleanupWorker(ctx context.Context, log zerolog.Logger, refreshSessions *auth.RefreshSessionService) {
	if refreshSessions == nil {
		return
	}
	intervalMinutes := intFromEnv("REFRESH_SESSION_CLEANUP_INTERVAL_MIN", 1440)
	retentionDays := intFromEnvAllowZero("REFRESH_SESSION_REVOKED_RETENTION_DAYS", 7)
	interval := time.Duration(intervalMinutes) * time.Minute
	revokedRetention := time.Duration(retentionDays) * 24 * time.Hour
	log.Info().
		Int("interval_min", intervalMinutes).
		Int("revoked_retention_days", retentionDays).
		Msg("refresh session cleanup worker enabled")
	go runTicker(ctx, interval, func() {
		runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := refreshSessions.CleanupExpired(runCtx, time.Now(), revokedRetention, 500)
		if err != nil {
			log.Error().Err(err).Msg("refresh session cleanup worker failed")
			return
		}
		if result.Deleted > 0 {
			log.Info().
				Int("deleted", result.Deleted).
				Msg("refresh session cleanup worker completed")
		}
	})
}

func runTicker(ctx context.Context, interval time.Duration, run func()) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func workerIDFromEnv(fallback string) string {
	if raw := strings.TrimSpace(os.Getenv("WORKER_ID")); raw != "" {
		return raw
	}
	return fallback
}

func intFromEnv(key string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func intFromEnvAllowZero(key string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			return parsed
		}
	}
	return fallback
}

func durationFromEnv(key string, fallbackSeconds int) time.Duration {
	return time.Duration(intFromEnv(key, fallbackSeconds))
}
