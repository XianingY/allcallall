package commerce

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

var allowedFollowUpTaskTypes = map[string]struct{}{
	models.FollowupTaskTypeCallback:         {},
	models.FollowupTaskTypeSendMessage:      {},
	models.FollowupTaskTypeScheduleNextCall: {},
}

var allowedFollowUpTaskStatuses = map[string]struct{}{
	models.FollowupTaskStatusOpen:      {},
	models.FollowupTaskStatusDone:      {},
	models.FollowupTaskStatusSnoozed:   {},
	models.FollowupTaskStatusCancelled: {},
}

// followupGenerator is the interface for generating call follow-ups.
// CallHistoryService uses this to avoid a circular dependency with FollowUpService.
type followupGenerator interface {
	GenerateFollowupForCall(ctx context.Context, callID string, force bool) error
}

// FollowupResponse contains a follow-up record and its associated tasks.
type FollowupResponse struct {
	Followup *models.CallFollowup  `json:"followup"`
	Tasks    []models.FollowUpTask `json:"tasks"`
}

// FollowUpListItem is an enriched follow-up task with related call, peer, and contact info.
type FollowUpListItem struct {
	Task      models.FollowUpTask    `json:"task"`
	Call      *models.CallSession    `json:"call,omitempty"`
	Followup  *models.CallFollowup   `json:"followup,omitempty"`
	Peer      *models.User           `json:"peer,omitempty"`
	Contact   *models.ContactProfile `json:"contact,omitempty"`
	IsOverdue bool                   `json:"is_overdue"`
}

func normalizeFollowUpTaskType(taskType string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(taskType))
	if _, ok := allowedFollowUpTaskTypes[normalized]; !ok {
		return "", errors.New("invalid follow-up task type")
	}
	return normalized, nil
}

func normalizeFollowUpTaskStatus(status string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(status))
	if _, ok := allowedFollowUpTaskStatuses[normalized]; !ok {
		return "", errors.New("invalid follow-up task status")
	}
	return normalized, nil
}

// FollowUpService manages call follow-ups and follow-up tasks.
type FollowUpService struct {
	repo    *Repository
	metrics metrics.Recorder
}

// NewFollowUpService creates a new FollowUpService.
func NewFollowUpService(repo *Repository, metrics metrics.Recorder) *FollowUpService {
	return &FollowUpService{repo: repo, metrics: metrics}
}

// GetFollowup returns a follow-up and its tasks for a specific call and user.
func (s *FollowUpService) GetFollowup(ctx context.Context, userID uint64, callID string) (*FollowupResponse, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, ErrFollowupNotFound
	}
	followup, err := s.repo.GetCallFollowup(ctx, callID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFollowupNotFound
		}
		return nil, err
	}
	var tasks []models.FollowUpTask
	if err := s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		var taskErr error
		tasks, taskErr = s.repo.GetFollowUpTasksByCalls(ctx, []string{callID}, userID, nil)
		return taskErr
	}); err != nil {
		return nil, err
	}
	return &FollowupResponse{
		Followup: followup,
		Tasks:    tasks,
	}, nil
}

// ListFollowUpTasks returns all follow-up tasks for a user with related call, peer, and contact info.
func (s *FollowUpService) ListFollowUpTasks(ctx context.Context, userID uint64) ([]FollowUpListItem, error) {
	tasks, err := s.repo.ListFollowUpTasksByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return []FollowUpListItem{}, nil
	}

	callIDs := make([]string, 0, len(tasks))
	peerIDs := make([]uint64, 0, len(tasks))
	callIDSeen := make(map[string]struct{})
	peerIDSeen := make(map[uint64]struct{})
	for _, task := range tasks {
		if task.CallID != "" {
			if _, ok := callIDSeen[task.CallID]; !ok {
				callIDs = append(callIDs, task.CallID)
				callIDSeen[task.CallID] = struct{}{}
			}
		}
		if _, ok := peerIDSeen[task.PeerUserID]; !ok {
			peerIDs = append(peerIDs, task.PeerUserID)
			peerIDSeen[task.PeerUserID] = struct{}{}
		}
	}

	callMap := make(map[string]models.CallSession)
	if len(callIDs) > 0 {
		var calls []models.CallSession
		if err := s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
			for _, callID := range callIDs {
				call, callErr := s.repo.GetCallSession(ctx, callID)
				if callErr != nil && !errors.Is(callErr, gorm.ErrRecordNotFound) {
					return callErr
				}
				if call != nil {
					calls = append(calls, *call)
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
		for _, item := range calls {
			callMap[item.CallID] = item
		}
	}
	followupMap := make(map[string]models.CallFollowup)
	if len(callIDs) > 0 {
		followups, err := s.repo.GetCallFollowupsByCalls(ctx, callIDs, userID)
		if err != nil {
			return nil, err
		}
		for _, item := range followups {
			followupMap[item.CallID] = item
		}
	}
	peerMap := make(map[uint64]models.User)
	if len(peerIDs) > 0 {
		peers, err := s.repo.GetUsersByIDs(ctx, peerIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range peers {
			peerMap[item.ID] = item
		}
	}
	contactMap := make(map[uint64]models.ContactProfile)
	if len(peerIDs) > 0 {
		contacts, err := s.repo.GetContactProfilesByOwnerAndContacts(ctx, userID, peerIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range contacts {
			contactMap[item.ContactUserID] = item
		}
	}

	now := time.Now().UTC()
	items := make([]FollowUpListItem, 0, len(tasks))
	for _, task := range tasks {
		item := FollowUpListItem{
			Task:      task,
			IsOverdue: task.DueAt != nil && task.Status == models.FollowupTaskStatusOpen && task.DueAt.Before(now),
		}
		if call, ok := callMap[task.CallID]; ok {
			item.Call = &call
		}
		if followup, ok := followupMap[task.CallID]; ok {
			item.Followup = &followup
		}
		if peer, ok := peerMap[task.PeerUserID]; ok {
			item.Peer = &peer
		}
		if contact, ok := contactMap[task.PeerUserID]; ok {
			item.Contact = &contact
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		priority := func(item FollowUpListItem) int {
			if item.IsOverdue {
				return 0
			}
			if item.Task.DueAt != nil {
				nowDate := now.Format("2006-01-02")
				dueDate := item.Task.DueAt.UTC().Format("2006-01-02")
				if dueDate == nowDate {
					return 1
				}
				return 2
			}
			if item.Task.Status == models.FollowupTaskStatusDone {
				return 4
			}
			return 3
		}
		left := priority(items[i])
		right := priority(items[j])
		if left != right {
			return left < right
		}
		return items[i].Task.CreatedAt.After(items[j].Task.CreatedAt)
	})

	return items, nil
}

// CreateFollowUpTask creates a new follow-up task with validation.
func (s *FollowUpService) CreateFollowUpTask(ctx context.Context, task *models.FollowUpTask) (*models.FollowUpTask, error) {
	if task == nil || task.UserID == 0 || task.PeerUserID == 0 || strings.TrimSpace(task.Type) == "" {
		return nil, errors.New("invalid follow-up task payload")
	}
	normalizedType, err := normalizeFollowUpTaskType(task.Type)
	if err != nil {
		return nil, err
	}
	task.Type = normalizedType
	task.Status = strings.TrimSpace(task.Status)
	if task.Status == "" {
		task.Status = models.FollowupTaskStatusOpen
	}
	normalizedStatus, err := normalizeFollowUpTaskStatus(task.Status)
	if err != nil {
		return nil, err
	}
	task.Status = normalizedStatus
	task.Title = strings.TrimSpace(task.Title)
	if task.Title == "" {
		task.Title = "跟进联系人"
	}
	task.Description = strings.TrimSpace(task.Description)
	if err := s.repo.CreateFollowUpTask(ctx, task); err != nil {
		return nil, err
	}
	if s.metrics != nil && task.Status == models.FollowupTaskStatusOpen {
		s.metrics.Inc("followup_task_open_total")
	}
	return task, nil
}

// UpdateFollowUpTask applies partial updates to a follow-up task.
func (s *FollowUpService) UpdateFollowUpTask(ctx context.Context, userID, taskID uint64, updates map[string]any) (*models.FollowUpTask, error) {
	if taskID == 0 {
		return nil, errors.New("task id is required")
	}
	task, err := s.repo.GetFollowUpTask(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	patch := map[string]any{"updated_at": time.Now().UTC()}
	if status, ok := updates["status"].(string); ok && strings.TrimSpace(status) != "" {
		normalizedStatus, err := normalizeFollowUpTaskStatus(status)
		if err != nil {
			return nil, err
		}
		patch["status"] = normalizedStatus
		if normalizedStatus == models.FollowupTaskStatusDone {
			patch["completed_at"] = time.Now().UTC()
		}
	}
	if dueAt, ok := updates["due_at"].(*time.Time); ok {
		patch["due_at"] = dueAt
	}
	if reminderMode, ok := updates["reminder_mode"].(string); ok {
		patch["reminder_mode"] = strings.TrimSpace(reminderMode)
	}
	if description, ok := updates["description"].(string); ok {
		patch["description"] = strings.TrimSpace(description)
	}
	if err := s.repo.UpdateFollowUpTask(ctx, taskID, patch); err != nil {
		return nil, err
	}
	task, err = s.repo.GetFollowUpTask(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// GenerateFollowupForCall generates follow-up records for both call participants.
func (s *FollowUpService) GenerateFollowupForCall(ctx context.Context, callID string, force bool) error {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return errors.New("call id is required")
	}

	call, err := s.repo.GetCallSession(ctx, callID)
	if err != nil {
		return err
	}
	if call.Status == models.CallStatusInvited {
		return nil
	}

	for _, userID := range []uint64{call.CallerID, call.CalleeID} {
		if err := s.generateFollowupForUser(ctx, *call, userID, force); err != nil {
			return err
		}
	}
	return nil
}

func (s *FollowUpService) generateFollowupForUser(ctx context.Context, call models.CallSession, userID uint64, force bool) error {
	peerID := call.CalleeID
	peerEmail := call.CalleeEmail
	peerName := call.CalleeDisplayName
	if userID == call.CalleeID {
		peerID = call.CallerID
		peerEmail = call.CallerEmail
		peerName = call.CallerDisplayName
	}

	existing, err := s.repo.GetCallFollowup(ctx, call.CallID, userID)
	if err == nil && !force {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	_ = existing

	wasAnswered := call.AnsweredAt != nil
	var transcriptSegments []models.CallTranscriptSegment
	if wasAnswered || call.Status == models.CallStatusAnswered {
		transcriptSegments, err = s.repo.GetTranscriptSegmentsByCallAndUser(ctx, call.CallID, userID)
		if err != nil {
			return err
		}
	}

	durationSeconds := int64(0)
	if call.AnsweredAt != nil && call.EndedAt != nil {
		durationSeconds = int64(call.EndedAt.Sub(*call.AnsweredAt).Seconds())
	}

	followup := models.CallFollowup{
		CallID:          call.CallID,
		UserID:          userID,
		PeerUserID:      peerID,
		Status:          models.FollowupStatusReady,
		Source:          "metadata",
		TranscriptCount: int64(len(transcriptSegments)),
	}

	if wasAnswered || call.Status == models.CallStatusAnswered {
		if len(transcriptSegments) >= 6 || durationSeconds >= 45 {
			followup.Source = "rules"
			followup.SummaryCN = fmt.Sprintf("与 %s 的通话已完成，围绕业务沟通形成了可复用的跟进摘要。", peerNameOrEmail(peerName, peerEmail))
			followup.SummaryEN = fmt.Sprintf("The call with %s completed and is ready for a follow-up.", peerNameOrEmail(peerName, peerEmail))
			keyPoints := []string{}
			actionItems := []string{"发送一条简短双语跟进消息，确认下一步。"}
			riskFlags := []string{}
			if len(transcriptSegments) > 0 {
				keyPoints = append(keyPoints, truncateSentence(transcriptSegments[0].TranslatedText, 140))
				last := transcriptSegments[len(transcriptSegments)-1]
				if last.TranslatedText != "" && last.TranslatedText != transcriptSegments[0].TranslatedText {
					keyPoints = append(keyPoints, truncateSentence(last.TranslatedText, 140))
				}
			}
			if len(keyPoints) == 0 {
				keyPoints = append(keyPoints, "本次通话已完成，建议尽快发送后续确认消息。")
			}
			if strings.Contains(strings.ToLower(strings.Join(extractTexts(transcriptSegments), " ")), "tomorrow") ||
				strings.Contains(strings.Join(extractTexts(transcriptSegments), " "), "明天") {
				actionItems = []string{"根据通话约定，安排下一次沟通时间。"}
				followup.NextStep = "安排下一次通话时间"
			} else {
				followup.NextStep = "发送双语跟进消息"
			}
			if durationSeconds < 60 {
				riskFlags = append(riskFlags, "通话较短，建议确认关键需求是否理解一致。")
			}
			followup.KeyPointsJSON = mustJSON(keyPoints)
			followup.ActionItemsJSON = mustJSON(actionItems)
			followup.RiskFlagsJSON = mustJSON(riskFlags)
			followup.FollowupDraftCN = fmt.Sprintf("你好，感谢刚才的沟通。我整理了本次通话的重点，建议我们按约定推进下一步。")
			followup.FollowupDraftEN = "Thanks for the call. I have summarized the key points and suggest we move on to the agreed next step."
		} else {
			if s.metrics != nil {
				s.metrics.Inc("followup_metadata_fallback_total")
			}
			followup.SummaryCN = fmt.Sprintf("与 %s 的通话已结束，可手动补充本次业务跟进。", peerNameOrEmail(peerName, peerEmail))
			followup.SummaryEN = fmt.Sprintf("The call with %s has ended. Add a manual follow-up if needed.", peerNameOrEmail(peerName, peerEmail))
			followup.NextStep = "手动确认后续动作"
			followup.KeyPointsJSON = mustJSON([]string{"暂无足够字幕内容，建议手动补充重点。"})
			followup.ActionItemsJSON = mustJSON([]string{"回顾本次通话，并记录下一步动作。"})
			followup.RiskFlagsJSON = mustJSON([]string{})
			followup.FollowupDraftCN = "你好，刚才的通话我已记录。方便的话，我们确认一下下一步安排。"
			followup.FollowupDraftEN = "I noted our recent call. Please let me know the best next step when convenient."
		}
	} else {
		followup.SummaryCN = fmt.Sprintf("与 %s 的通话未完成，建议尽快回拨。", peerNameOrEmail(peerName, peerEmail))
		followup.SummaryEN = fmt.Sprintf("The call with %s did not complete. A callback is recommended.", peerNameOrEmail(peerName, peerEmail))
		followup.NextStep = "安排回拨"
		followup.KeyPointsJSON = mustJSON([]string{"通话未接通，建议尽快回访。"})
		followup.ActionItemsJSON = mustJSON([]string{"重新联系对方并确认可沟通时间。"})
		followup.RiskFlagsJSON = mustJSON([]string{"未完成首次沟通，业务上下文可能中断。"})
		followup.FollowupDraftCN = "你好，刚才未能接通，方便时请回复一个适合沟通的时间。"
		followup.FollowupDraftEN = "We missed each other on the last call. Please share a suitable time for a callback."
	}

	now := time.Now().UTC()
	followup.GeneratedAt = &now

	return s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		current, txErr := s.repo.GetCallFollowup(ctx, call.CallID, userID)
		if txErr == nil {
			followup.ID = current.ID
			if err := s.repo.UpdateCallFollowup(ctx, current, map[string]any{
				"status":            followup.Status,
				"source":            followup.Source,
				"summary_cn":        followup.SummaryCN,
				"summary_en":        followup.SummaryEN,
				"key_points_json":   followup.KeyPointsJSON,
				"action_items_json": followup.ActionItemsJSON,
				"next_step":         followup.NextStep,
				"risk_flags_json":   followup.RiskFlagsJSON,
				"followup_draft_cn": followup.FollowupDraftCN,
				"followup_draft_en": followup.FollowupDraftEN,
				"generated_at":      followup.GeneratedAt,
				"transcript_count":  followup.TranscriptCount,
				"updated_at":        now,
			}); err != nil {
				if s.metrics != nil {
					s.metrics.Inc("followup_generate_fail_total")
				}
				return err
			}
		} else if errors.Is(txErr, gorm.ErrRecordNotFound) {
			if err := s.repo.SaveCallFollowup(ctx, &followup); err != nil {
				if s.metrics != nil {
					s.metrics.Inc("followup_generate_fail_total")
				}
				return err
			}
		} else {
			if s.metrics != nil {
				s.metrics.Inc("followup_generate_fail_total")
			}
			return txErr
		}

		if !wasAnswered || call.Status == models.CallStatusRejected || call.Status == models.CallStatusMissed || call.Status == models.CallStatusFailed {
			if err := s.ensureDefaultFollowupTask(ctx, call, userID, peerID, models.FollowupTaskTypeCallback, "回拨该联系人", "通话未接通，建议尽快回拨。", time.Now().UTC().Add(2*time.Hour)); err != nil {
				return err
			}
		} else {
			taskType := models.FollowupTaskTypeSendMessage
			title := "发送双语跟进消息"
			description := "发送一条双语跟进消息，确认关键结论与下一步。"
			textBlob := strings.ToLower(strings.Join(extractTexts(transcriptSegments), " "))
			if strings.Contains(textBlob, "tomorrow") || strings.Contains(strings.Join(extractTexts(transcriptSegments), " "), "明天") {
				taskType = models.FollowupTaskTypeScheduleNextCall
				title = "安排下一次通话"
				description = "根据本次通话内容，安排下一次沟通时间。"
			}
			if err := s.ensureDefaultFollowupTask(ctx, call, userID, peerID, taskType, title, description, time.Now().UTC().Add(24*time.Hour)); err != nil {
				return err
			}
		}
		if s.metrics != nil {
			s.metrics.Inc("followup_generate_total")
		}
		if len(transcriptSegments) > 0 {
			if err := s.repo.DeleteTranscriptSegmentsByCallAndUser(ctx, call.CallID, userID); err != nil {
				return err
			}
			if s.metrics != nil {
				s.metrics.Add("transcript_segment_purged_total", int64(len(transcriptSegments)))
			}
		}
		return nil
	})
}

func (s *FollowUpService) ensureDefaultFollowupTask(ctx context.Context, call models.CallSession, userID, peerID uint64, taskType, title, description string, dueAt time.Time) error {
	existing, err := s.repo.GetFollowUpTaskByCallAndType(ctx, call.CallID, userID, taskType)
	if err == nil {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	_ = existing
	task := &models.FollowUpTask{
		UserID:       userID,
		PeerUserID:   peerID,
		CallID:       call.CallID,
		Type:         taskType,
		Status:       models.FollowupTaskStatusOpen,
		Title:        title,
		Description:  description,
		DueAt:        &dueAt,
		ReminderMode: "default",
	}
	if err := s.repo.CreateFollowUpTask(ctx, task); err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.Inc("followup_task_open_total")
	}
	return nil
}

func mustJSON(values []string) string {
	raw, _ := json.Marshal(values)
	return string(raw)
}

func extractTexts(segments []models.CallTranscriptSegment) []string {
	texts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.TrimSpace(segment.TranslatedText) != "" {
			texts = append(texts, strings.TrimSpace(segment.TranslatedText))
			continue
		}
		if strings.TrimSpace(segment.OriginalText) != "" {
			texts = append(texts, strings.TrimSpace(segment.OriginalText))
		}
	}
	return texts
}

func truncateSentence(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func peerNameOrEmail(name, email string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return strings.TrimSpace(email)
}
