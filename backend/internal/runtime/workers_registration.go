package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/mq"
	"github.com/allcallall/backend/internal/search"
	"github.com/allcallall/backend/internal/settlement"
	"github.com/allcallall/backend/internal/trace"
)

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
