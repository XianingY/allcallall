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
	StartMessageRetentionWorker(ctx, log, collaborationSvc)
	StartAuditRetentionWorker(ctx, log, collaborationSvc)
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
