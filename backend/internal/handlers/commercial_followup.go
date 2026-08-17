package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func (h *CommercialHandler) handleCallHistory(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	tier, err := h.commerce.ActiveTier(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("get tier failed")
		JSONError(c, http.StatusInternalServerError, "failed to load call history")
		return
	}
	if tier != models.EntitlementPremium && days > 30 {
		days = 30
	}
	history, err := h.commerce.ListCallHistory(c.Request.Context(), claims.UserID, days)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("call history failed")
		JSONError(c, http.StatusInternalServerError, "failed to load call history")
		return
	}
	response := make([]callHistoryResponse, 0, len(history))
	for _, item := range history {
		response = append(response, toCallHistoryResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"calls": response})
}

func (h *CommercialHandler) handleGetFollowup(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	callID := strings.TrimSpace(c.Param("callId"))
	followup, err := h.commerce.GetFollowup(c.Request.Context(), claims.UserID, callID)
	if err != nil {
		if errors.Is(err, commerce.ErrFollowupNotFound) {
			JSONError(c, http.StatusNotFound, "follow-up not found")
			return
		}
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Str("call_id", callID).Msg("get follow-up failed")
		JSONError(c, http.StatusInternalServerError, "failed to load follow-up")
		return
	}
	taskResponse := make([]followUpTaskResponse, 0, len(followup.Tasks))
	for _, item := range followup.Tasks {
		taskResponse = append(taskResponse, toFollowUpTaskResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"followup": toCallFollowupResponse(followup.Followup), "tasks": taskResponse})
}

func (h *CommercialHandler) handleGenerateFollowup(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	callID := strings.TrimSpace(c.Param("callId"))
	if err := h.commerce.GenerateFollowupForCall(c.Request.Context(), callID, false); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Str("call_id", callID).Msg("generate follow-up failed")
		JSONError(c, http.StatusInternalServerError, "failed to generate follow-up")
		return
	}
	followup, err := h.commerce.GetFollowup(c.Request.Context(), claims.UserID, callID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Str("call_id", callID).Msg("reload follow-up failed")
		JSONError(c, http.StatusInternalServerError, "failed to load follow-up")
		return
	}
	taskResponse := make([]followUpTaskResponse, 0, len(followup.Tasks))
	for _, item := range followup.Tasks {
		taskResponse = append(taskResponse, toFollowUpTaskResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"followup": toCallFollowupResponse(followup.Followup), "tasks": taskResponse})
}

func (h *CommercialHandler) handleRegenerateFollowup(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	callID := strings.TrimSpace(c.Param("callId"))
	if err := h.commerce.GenerateFollowupForCall(c.Request.Context(), callID, true); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Str("call_id", callID).Msg("regenerate follow-up failed")
		JSONError(c, http.StatusInternalServerError, "failed to regenerate follow-up")
		return
	}
	followup, err := h.commerce.GetFollowup(c.Request.Context(), claims.UserID, callID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Str("call_id", callID).Msg("reload regenerated follow-up failed")
		JSONError(c, http.StatusInternalServerError, "failed to load follow-up")
		return
	}
	taskResponse := make([]followUpTaskResponse, 0, len(followup.Tasks))
	for _, item := range followup.Tasks {
		taskResponse = append(taskResponse, toFollowUpTaskResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"followup": toCallFollowupResponse(followup.Followup), "tasks": taskResponse})
}

type blockRequest struct {
	BlockedUserID uint64 `json:"blocked_user_id"`
}

func (h *CommercialHandler) handleCreateBlock(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req blockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.BlockedUserID == 0 || req.BlockedUserID == claims.UserID {
		JSONError(c, http.StatusBadRequest, "invalid blocked user id")
		return
	}
	if err := h.commerce.CreateBlock(c.Request.Context(), claims.UserID, req.BlockedUserID); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("create block failed")
		JSONError(c, http.StatusInternalServerError, "failed to block user")
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"success": true})
}

func parseOptionalTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	utc := parsed.UTC()
	return &utc, nil
}

func (h *CommercialHandler) handleListFollowUps(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := h.commerce.ListFollowUpTasks(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("list follow-up tasks failed")
		JSONError(c, http.StatusInternalServerError, "failed to load follow-up tasks")
		return
	}
	response := make([]followUpListItemResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toFollowUpListItemResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"items": response})
}

type followUpTaskRequest struct {
	PeerUserID   uint64  `json:"peer_user_id"`
	CallID       string  `json:"call_id"`
	Type         string  `json:"type"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	DueAt        *string `json:"due_at"`
	ReminderMode string  `json:"reminder_mode"`
}

func (h *CommercialHandler) handleCreateFollowUp(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req followUpTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	dueAt, err := parseOptionalTime(req.DueAt)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid due_at")
		return
	}
	task, err := h.commerce.CreateFollowUpTask(c.Request.Context(), &models.FollowUpTask{
		UserID:       claims.UserID,
		PeerUserID:   req.PeerUserID,
		CallID:       strings.TrimSpace(req.CallID),
		Type:         strings.TrimSpace(req.Type),
		Status:       models.FollowupTaskStatusOpen,
		Title:        strings.TrimSpace(req.Title),
		Description:  strings.TrimSpace(req.Description),
		DueAt:        dueAt,
		ReminderMode: strings.TrimSpace(req.ReminderMode),
	})
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("create follow-up task failed")
		JSONError(c, http.StatusInternalServerError, "failed to create follow-up task")
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"task": toFollowUpTaskResponse(*task)})
}

type updateFollowUpTaskRequest struct {
	Status       string  `json:"status"`
	Description  string  `json:"description"`
	DueAt        *string `json:"due_at"`
	ReminderMode string  `json:"reminder_mode"`
}

func (h *CommercialHandler) handleUpdateFollowUp(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	taskID, err := strconv.ParseUint(c.Param("taskId"), 10, 64)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	var req updateFollowUpTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	dueAt, err := parseOptionalTime(req.DueAt)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid due_at")
		return
	}
	task, err := h.commerce.UpdateFollowUpTask(c.Request.Context(), claims.UserID, taskID, map[string]any{
		"status":        strings.TrimSpace(req.Status),
		"description":   strings.TrimSpace(req.Description),
		"due_at":        dueAt,
		"reminder_mode": strings.TrimSpace(req.ReminderMode),
	})
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Uint64("task_id", taskID).Msg("update follow-up task failed")
		JSONError(c, http.StatusInternalServerError, "failed to update follow-up task")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"task": toFollowUpTaskResponse(*task)})
}

func (h *CommercialHandler) handleListBlocks(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	blocks, err := h.commerce.ListBlocks(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("list blocks failed")
		JSONError(c, http.StatusInternalServerError, "failed to list blocked users")
		return
	}
	type blockResponse struct {
		ID                     uint64     `json:"id"`
		BlockedUserID          uint64     `json:"blocked_user_id"`
		BlockedUserEmail       string     `json:"blocked_user_email,omitempty"`
		BlockedUserDisplayName string     `json:"blocked_user_display_name,omitempty"`
		BlockedUserStatus      string     `json:"blocked_user_status,omitempty"`
		BlockedUserDeletedAt   *time.Time `json:"blocked_user_deleted_at,omitempty"`
		CreatedAt              time.Time  `json:"created_at"`
	}
	response := make([]blockResponse, 0, len(blocks))
	for _, block := range blocks {
		item := blockResponse{
			ID:            block.ID,
			BlockedUserID: block.BlockedUserID,
			CreatedAt:     block.CreatedAt,
		}
		if h.users != nil {
			blockedUser, userErr := h.users.GetByID(c.Request.Context(), block.BlockedUserID)
			if userErr == nil && blockedUser != nil {
				item.BlockedUserEmail = blockedUser.Email
				item.BlockedUserDisplayName = blockedUser.DisplayName
				item.BlockedUserStatus = blockedUser.Status
				item.BlockedUserDeletedAt = blockedUser.DeletedAt
			}
		}
		response = append(response, item)
	}
	JSONSuccess(c, http.StatusOK, gin.H{"blocks": response})
}

func (h *CommercialHandler) handleRemoveBlock(c *gin.Context) {
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	blockedID, err := strconv.ParseUint(c.Param("blockedUserId"), 10, 64)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid blocked user id")
		return
	}
	if err := h.commerce.RemoveBlock(c.Request.Context(), claims.UserID, blockedID); err != nil {
		h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("remove block failed")
		JSONError(c, http.StatusInternalServerError, "failed to unblock user")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}
