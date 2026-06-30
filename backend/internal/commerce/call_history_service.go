package commerce

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// CallHistoryEntry extends a call session with follow-up status information.
type CallHistoryEntry struct {
	models.CallSession
	FollowupStatus string     `json:"followup_status,omitempty"`
	NextTaskDueAt  *time.Time `json:"next_task_due_at,omitempty"`
	IsOverdue      bool       `json:"is_overdue"`
}

// CallHistoryService manages call sessions, transcripts, and call history.
type CallHistoryService struct {
	repo      *Repository
	metrics   metrics.Recorder
	followups followupGenerator
}

// NewCallHistoryService creates a new CallHistoryService.
func NewCallHistoryService(repo *Repository, metrics metrics.Recorder, followups followupGenerator) *CallHistoryService {
	return &CallHistoryService{repo: repo, metrics: metrics, followups: followups}
}

// RegisterCallInvite records a new call invite.
func (s *CallHistoryService) RegisterCallInvite(ctx context.Context, callID string, caller *models.User, callee *models.User) error {
	now := time.Now().UTC()
	record := &models.CallSession{
		CallID:            callID,
		CallerID:          caller.ID,
		CalleeID:          callee.ID,
		CallerEmail:       caller.Email,
		CalleeEmail:       callee.Email,
		CallerDisplayName: caller.DisplayName,
		CalleeDisplayName: callee.DisplayName,
		Status:            models.CallStatusInvited,
		StartedAt:         now,
		LastEventAt:       now,
	}

	return s.repo.RegisterCallInvite(ctx, record)
}

// RecordTranscriptSegment stores a transcript segment for a call.
func (s *CallHistoryService) RecordTranscriptSegment(ctx context.Context, segment models.CallTranscriptSegment) error {
	if strings.TrimSpace(segment.CallID) == "" || segment.UserID == 0 || segment.PeerUserID == 0 {
		return errors.New("call transcript segment requires call and user ids")
	}
	segment.CallID = strings.TrimSpace(segment.CallID)
	segment.FromEmail = strings.TrimSpace(strings.ToLower(segment.FromEmail))
	segment.ToEmail = strings.TrimSpace(strings.ToLower(segment.ToEmail))
	if segment.TimestampMS <= 0 {
		segment.TimestampMS = time.Now().UnixMilli()
	}
	return s.repo.CreateTranscriptSegment(ctx, &segment)
}

// MarkFollowupSecondCallCompleted marks callback tasks as done when a second call is completed.
func (s *CallHistoryService) MarkFollowupSecondCallCompleted(ctx context.Context, userID, peerUserID uint64, callID string, completedAt time.Time) error {
	if userID == 0 || peerUserID == 0 {
		return nil
	}
	callID = strings.TrimSpace(callID)
	windowStart := completedAt.Add(-7 * 24 * time.Hour)
	return s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		count, err := s.repo.CountRecentCallsBetweenUsers(ctx, userID, peerUserID, windowStart, callID)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		return s.repo.UpdateFollowUpTasksByUserPeerType(ctx, userID, peerUserID, models.FollowupTaskTypeCallback, models.FollowupTaskStatusOpen, map[string]any{
			"status":       models.FollowupTaskStatusDone,
			"completed_at": completedAt.UTC(),
			"updated_at":   completedAt.UTC(),
		})
	})
}

// UpdateCallStatus updates a call session's status and triggers follow-up generation on call end.
func (s *CallHistoryService) UpdateCallStatus(ctx context.Context, callID string, status string, endReason string) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"status":        status,
		"last_event_at": now,
		"updated_at":    now,
	}
	if endReason != "" {
		updates["end_reason"] = endReason
	}
	switch status {
	case models.CallStatusAnswered:
		updates["answered_at"] = now
	case models.CallStatusEnded, models.CallStatusRejected, models.CallStatusMissed, models.CallStatusFailed:
		updates["ended_at"] = now
	}
	if err := s.repo.UpdateCallStatus(ctx, callID, updates); err != nil {
		return err
	}
	if status == models.CallStatusEnded || status == models.CallStatusRejected || status == models.CallStatusMissed || status == models.CallStatusFailed {
		return s.followups.GenerateFollowupForCall(ctx, callID, false)
	}
	return nil
}

// ListCallHistory returns call history for a user within the given number of days.
func (s *CallHistoryService) ListCallHistory(ctx context.Context, userID uint64, days int) ([]CallHistoryEntry, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	sessions, err := s.repo.ListCallSessionsByUser(ctx, userID, since)
	if err != nil || len(sessions) == 0 {
		return []CallHistoryEntry{}, err
	}

	callIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		callIDs = append(callIDs, session.CallID)
	}
	followups, err := s.repo.GetCallFollowupsByCalls(ctx, callIDs, userID)
	if err != nil {
		return nil, err
	}
	followupMap := make(map[string]models.CallFollowup, len(followups))
	for _, item := range followups {
		followupMap[item.CallID] = item
	}
	tasks, err := s.repo.GetFollowUpTasksByCalls(ctx, callIDs, userID, []string{models.FollowupTaskStatusOpen, models.FollowupTaskStatusSnoozed})
	if err != nil {
		return nil, err
	}
	taskMap := make(map[string]models.FollowUpTask, len(tasks))
	for _, item := range tasks {
		if existing, ok := taskMap[item.CallID]; ok && existing.DueAt != nil && item.DueAt != nil && existing.DueAt.Before(*item.DueAt) {
			continue
		}
		taskMap[item.CallID] = item
	}

	now := time.Now().UTC()
	result := make([]CallHistoryEntry, 0, len(sessions))
	for _, session := range sessions {
		entry := CallHistoryEntry{CallSession: session}
		if followup, ok := followupMap[session.CallID]; ok {
			entry.FollowupStatus = followup.Status
		}
		if task, ok := taskMap[session.CallID]; ok {
			entry.NextTaskDueAt = task.DueAt
			entry.IsOverdue = task.DueAt != nil && task.Status == models.FollowupTaskStatusOpen && task.DueAt.Before(now)
			if entry.FollowupStatus == "" {
				entry.FollowupStatus = task.Status
			}
		}
		result = append(result, entry)
	}
	return result, nil
}
