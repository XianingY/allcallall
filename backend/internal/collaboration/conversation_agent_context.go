package collaboration

import (
	"context"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) buildConversationAgentContext(ctx context.Context, organizationID, conversationID uint64, latestFollowup *ConversationFollowupSummary) ConversationAgentContext {
	result := ConversationAgentContext{}
	if latestFollowup != nil {
		result.LatestCallID = latestFollowup.CallID
		if result.LatestCallID != "" {
			var transcriptCount int64
			if err := s.db.WithContext(ctx).
				Model(&models.CallTranscriptSegment{}).
				Where("call_id = ?", result.LatestCallID).
				Count(&transcriptCount).Error; err == nil {
				result.TranscriptSegmentCount = int(transcriptCount)
			}
			var latestSegment models.CallTranscriptSegment
			if err := s.db.WithContext(ctx).
				Select("created_at").
				Where("call_id = ?", result.LatestCallID).
				Order("created_at DESC").
				Take(&latestSegment).Error; err == nil {
				result.LatestTranscriptAt = &latestSegment.CreatedAt
			}
		}
	}
	var memories []models.AgentMemory
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Limit(10).
		Find(&memories).Error; err == nil {
		keys := make([]string, 0, len(memories))
		for _, memory := range memories {
			keys = append(keys, memory.Key)
		}
		result.LatestMemoryKeys = uniqueStrings(keys)
	}
	var workflow models.WorkflowRun
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("COALESCE(completed_at, started_at, updated_at) DESC").
		Take(&workflow).Error; err == nil {
		result.LastWorkflowID = &workflow.ID
		result.LastWorkflowPreset = workflow.Preset
		result.LastAgentStatus = workflow.Status
		switch {
		case workflow.CompletedAt != nil:
			result.LastAgentRunAt = workflow.CompletedAt
		case workflow.StartedAt != nil:
			result.LastAgentRunAt = workflow.StartedAt
		default:
			at := workflow.UpdatedAt
			result.LastAgentRunAt = &at
		}
	} else {
		var run models.AgentRun
		if err := s.db.WithContext(ctx).
			Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
			Order("COALESCE(completed_at, started_at, updated_at) DESC").
			Take(&run).Error; err == nil {
			result.LastAgentStatus = run.Status
			switch {
			case run.CompletedAt != nil:
				result.LastAgentRunAt = run.CompletedAt
			case run.StartedAt != nil:
				result.LastAgentRunAt = run.StartedAt
			default:
				at := run.UpdatedAt
				result.LastAgentRunAt = &at
			}
		}
	}
	if err := s.db.WithContext(ctx).
		Model(&models.ToolApproval{}).
		Joins("JOIN workflow_runs ON workflow_runs.id = tool_approvals.workflow_run_id").
		Where("tool_approvals.organization_id = ? AND workflow_runs.conversation_id = ? AND tool_approvals.status = ?", organizationID, conversationID, models.ToolApprovalStatusPending).
		Count(&result.PendingApprovalCount).Error; err != nil {
		s.logger.Warn().Err(err).Uint64("conversation_id", conversationID).Msg("failed to count pending tool approvals")
	}
	if err := s.db.WithContext(ctx).
		Model(&models.RAGSource{}).
		Where("organization_id = ? AND (conversation_id IS NULL OR conversation_id = ?)", organizationID, conversationID).
		Where("status = ?", models.RAGSourceStatusReady).
		Where("(dedupe_status IS NULL OR dedupe_status <> ?)", models.RAGSourceDedupeStatusConfirmedDuplicate).
		Count(&result.KnowledgeSourceCount).Error; err != nil {
		s.logger.Warn().Err(err).Uint64("conversation_id", conversationID).Msg("failed to count knowledge sources")
	}
	result.applyMeetingTranscriptionContext(s.loadLatestConversationTranscriptionContext(ctx, organizationID, conversationID))
	return result
}
