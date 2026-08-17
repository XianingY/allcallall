package runtime

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/knowledge"
	"github.com/allcallall/backend/internal/mcpplatform"
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
