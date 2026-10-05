package runtime

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/events"
)

func StartOutboxWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	if processor == nil {
		return
	}
	idleMs := intFromEnv("OUTBOX_WORKER_IDLE_MS", 500)
	idleInterval := time.Duration(idleMs) * time.Millisecond
	log.Info().
		Int("idle_ms", idleMs).
		Msg("outbox worker enabled (continuous drain, idle wait when empty)")
	go processor.Run(ctx, idleInterval)
}

func StartAgentWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor, services ...*agent.Service) {
	agentOrderedEvents := []string{EventAgentRunRequested, EventWorkflowRequested, EventMCPExecutionTerminal}
	ConfigureOutboxProcessorFromEnvWithConcurrency(
		processor,
		workerIDFromEnv("agent-worker"),
		intFromEnv("OUTBOX_WORKER_CONCURRENCY", 2),
		intFromEnv("OUTBOX_WORKER_QUEUE_DEPTH", 4),
		time.Duration(intFromEnv("OUTBOX_WORKER_IDLE_MS", 500))*time.Millisecond,
		time.Duration(intFromEnv("OUTBOX_WORKER_ERROR_BACKOFF_MS", 1000))*time.Millisecond,
		durationFromEnv("OUTBOX_WORKER_LEASE_SEC", 120)*time.Second,
		agentOrderedEvents,
		EventAgentRunRequested, EventWorkflowRequested, EventMCPExecutionTerminal,
	)
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
	collabOrderedEvents := []string{EventAgentRunCompleted, EventMessageCreated, EventRecordingTranscriptionRequested}
	ConfigureOutboxProcessorFromEnvWithConcurrency(
		processor,
		workerIDFromEnv("outbox-worker"),
		intFromEnv("OUTBOX_WORKER_CONCURRENCY", 8),
		intFromEnv("OUTBOX_WORKER_QUEUE_DEPTH", 16),
		time.Duration(intFromEnv("OUTBOX_WORKER_IDLE_MS", 500))*time.Millisecond,
		time.Duration(intFromEnv("OUTBOX_WORKER_ERROR_BACKOFF_MS", 1000))*time.Millisecond,
		durationFromEnv("OUTBOX_WORKER_LEASE_SEC", 120)*time.Second,
		collabOrderedEvents,
		EventAgentRunCompleted, EventMessageCreated, EventRecordingTranscriptionRequested,
	)
	StartOutboxWorker(ctx, log.With().Str("worker", "outbox").Logger(), processor)
}

func StartSearchOutboxWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	// Search indexing events are idempotent and don't require aggregate ordering.
	ConfigureOutboxProcessorFromEnvWithConcurrency(
		processor,
		workerIDFromEnv("search-worker"),
		intFromEnv("OUTBOX_WORKER_CONCURRENCY", 4),
		intFromEnv("OUTBOX_WORKER_QUEUE_DEPTH", 8),
		time.Duration(intFromEnv("OUTBOX_WORKER_IDLE_MS", 500))*time.Millisecond,
		time.Duration(intFromEnv("OUTBOX_WORKER_ERROR_BACKOFF_MS", 1000))*time.Millisecond,
		durationFromEnv("OUTBOX_WORKER_LEASE_SEC", 120)*time.Second,
		nil, // no ordered events — search indexing is idempotent
		EventSearchMessageIndex,
	)
	StartOutboxWorker(ctx, log.With().Str("worker", "search").Logger(), processor)
}

func StartSettlementBridgeWorker(ctx context.Context, log zerolog.Logger, processor *events.Processor) {
	settlementOrderedEvents := []string{EventSettlementRoomEnd}
	ConfigureOutboxProcessorFromEnvWithConcurrency(
		processor,
		workerIDFromEnv("settlement-bridge"),
		intFromEnv("OUTBOX_WORKER_CONCURRENCY", 2),
		intFromEnv("OUTBOX_WORKER_QUEUE_DEPTH", 4),
		time.Duration(intFromEnv("OUTBOX_WORKER_IDLE_MS", 500))*time.Millisecond,
		time.Duration(intFromEnv("OUTBOX_WORKER_ERROR_BACKOFF_MS", 1000))*time.Millisecond,
		durationFromEnv("OUTBOX_WORKER_LEASE_SEC", 120)*time.Second,
		settlementOrderedEvents,
		EventSettlementRoomEnd,
	)
	StartOutboxWorker(ctx, log.With().Str("worker", "settlement-bridge").Logger(), processor)
}

func StartCleanupWorker(ctx context.Context, log zerolog.Logger, collaborationSvc *collaboration.Service, refreshSessions *auth.RefreshSessionService) {
	StartRecordingCleanupWorker(ctx, log, collaborationSvc)
	StartRefreshSessionCleanupWorker(ctx, log, refreshSessions)
	StartMessageRetentionWorker(ctx, log, collaborationSvc)
	StartAuditRetentionWorker(ctx, log, collaborationSvc)
	StartRealtimeEventRetentionWorker(ctx, log, collaborationSvc)
}

// StartRealtimeEventRetentionWorker 周期性清理超过回放窗口的 chat_events。
// 该表只服务 websocket 重连回放，事件被客户端游标越过或早于窗口后即无用途；
// 若不清理则表只增不删（每条实时事件按收件人各一行），最终拖垮库与备份。
// 默认保留 7 天、每小时清理一次，可用环境变量调整或关闭（<=0 禁用）。
// StartRealtimeEventRetentionWorker periodically prunes chat_events past the replay window.
func StartRealtimeEventRetentionWorker(ctx context.Context, log zerolog.Logger, collaborationSvc *collaboration.Service) {
	if collaborationSvc == nil {
		return
	}
	retentionDays := intFromEnvAllowZero("CHAT_EVENT_RETENTION_DAYS", 7)
	if retentionDays <= 0 {
		log.Info().Msg("realtime event retention worker disabled (retention <= 0)")
		return
	}
	intervalMinutes := intFromEnv("CHAT_EVENT_RETENTION_CLEANUP_INTERVAL_MIN", 60)
	batchLimit := intFromEnv("CHAT_EVENT_RETENTION_CLEANUP_BATCH_LIMIT", 500)
	interval := time.Duration(intervalMinutes) * time.Minute
	log.Info().
		Int("retention_days", retentionDays).
		Int("interval_min", intervalMinutes).
		Int("batch_limit", batchLimit).
		Msg("realtime event retention worker enabled")
	go runTicker(ctx, interval, func() {
		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		before := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		purged, err := collaborationSvc.PurgeExpiredRealtimeEvents(runCtx, before, batchLimit)
		if err != nil {
			log.Error().Err(err).Msg("realtime event retention worker failed")
			return
		}
		if purged > 0 {
			log.Info().Int64("purged", purged).Msg("realtime event retention worker completed")
		}
	})
}

// StartAuditRetentionWorker 周期性清理超过最短留存期的组织审计事件。
// 审计留存期默认 180 天（≥《网络安全法》第二十一条 6 个月要求），由环境变量
// AUDIT_LOG_RETENTION_DAYS 覆写；到期即物理删除，不再作为合规证据留存。
// StartAuditRetentionWorker periodically purges org audit events past their retention window.
func StartAuditRetentionWorker(ctx context.Context, log zerolog.Logger, collaborationSvc *collaboration.Service) {
	if collaborationSvc == nil {
		return
	}
	retentionDays := intFromEnvAllowZero("AUDIT_LOG_RETENTION_DAYS", 180)
	if retentionDays <= 0 {
		log.Info().Msg("audit retention worker disabled (retention <= 0)")
		return
	}
	intervalMinutes := intFromEnv("AUDIT_RETENTION_CLEANUP_INTERVAL_MIN", 1440)
	interval := time.Duration(intervalMinutes) * time.Minute
	log.Info().
		Int("retention_days", retentionDays).
		Int("interval_min", intervalMinutes).
		Msg("audit retention worker enabled")
	go runTicker(ctx, interval, func() {
		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		before := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		purged, err := collaborationSvc.PurgeExpiredAuditEvents(runCtx, before, 500)
		if err != nil {
			log.Error().Err(err).Msg("audit retention worker failed")
			return
		}
		if purged > 0 {
			log.Info().Int64("purged", purged).Msg("audit retention worker completed")
		}
	})
}

// StartMessageRetentionWorker 周期性清理到期的消息正文与附件对象。
// 该 worker 是 PIPL 第十九条「最短必要保存期限」在工程侧的执行者：到期即物理清空正文，
// 并把清空后的空文档回写搜索索引，杜绝「库里删了、索引还能搜」的留存漏洞。
// StartMessageRetentionWorker periodically purges expired message bodies and attachments.
func StartMessageRetentionWorker(ctx context.Context, log zerolog.Logger, collaborationSvc *collaboration.Service) {
	if collaborationSvc == nil {
		return
	}
	policy := collaborationSvc.MessageRetentionPolicySnapshot()
	if !policy.Enabled {
		log.Info().Msg("message retention worker disabled")
		return
	}
	intervalMinutes := intFromEnv("MESSAGE_RETENTION_CLEANUP_INTERVAL_MIN", 30)
	batchLimit := intFromEnv("MESSAGE_RETENTION_CLEANUP_BATCH_LIMIT", 500)
	interval := time.Duration(intervalMinutes) * time.Minute
	log.Info().
		Int("interval_min", intervalMinutes).
		Int("batch_limit", batchLimit).
		Dur("text_ttl", policy.TextTTL).
		Dur("media_ttl", policy.MediaTTL).
		Msg("message retention worker enabled")
	go runTicker(ctx, interval, func() {
		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		result, err := collaborationSvc.CleanupExpiredMessages(runCtx, time.Now(), batchLimit)
		if err != nil {
			log.Error().Err(err).Msg("message retention worker failed")
			return
		}
		if result.MessagesPurged > 0 || result.AttachmentsPurged > 0 || result.AttachmentsFailed > 0 {
			log.Info().
				Int("messages_checked", result.MessagesChecked).
				Int("messages_purged", result.MessagesPurged).
				Int("attachments_checked", result.AttachmentsChecked).
				Int("attachments_purged", result.AttachmentsPurged).
				Int("attachments_failed", result.AttachmentsFailed).
				Msg("message retention worker completed")
		}
	})
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
