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
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

const (
	EventAgentRunRequested = "agent.run.requested"
	EventAgentRunCompleted = "agent.run.completed"
	EventMessageCreated    = "message.created"
)

func EmbeddedWorkersEnabledFromEnv() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("EMBEDDED_WORKERS")))
	return raw == "" || raw == "1" || raw == "true" || raw == "yes"
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

func StartAgentWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	ConfigureOutboxProcessorFromEnv(processor, workerIDFromEnv("agent-worker"), EventAgentRunRequested)
	StartOutboxWorker(ctx, log.With().Str("worker", "agent").Logger(), processor)
}

func StartCollaborationOutboxWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	ConfigureOutboxProcessorFromEnv(processor, workerIDFromEnv("outbox-worker"), EventAgentRunCompleted, EventMessageCreated)
	StartOutboxWorker(ctx, log.With().Str("worker", "outbox").Logger(), processor)
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
