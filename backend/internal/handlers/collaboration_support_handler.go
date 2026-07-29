package handlers

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *CollaborationHandler) requireSupportToken(c *gin.Context) bool {
	if !requireSupportNetwork(c) {
		return false
	}
	expected := strings.TrimSpace(os.Getenv("SUPPORT_API_TOKEN"))
	if expected == "" {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "SUPPORT_TOKEN_NOT_CONFIGURED", "support api token is not configured")
		return false
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(c.GetHeader("X-Support-Token"))), []byte(expected)) != 1 {
		JSONErrorWithCode(c, http.StatusUnauthorized, "SUPPORT_UNAUTHORIZED", "unauthorized support request")
		return false
	}
	return true
}

func (h *CollaborationHandler) handleSupportRoom(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	item, err := h.service.GetSupportRoom(c.Request.Context(), roomID)
	if err != nil {
		JSONError(c, http.StatusInternalServerError, "failed to load support room")
		return
	}
	response := supportRoomResponse{
		State:        toRoomStateResponse(*item.State),
		RecentEvents: item.RecentEvents,
	}
	if item.Recording != nil {
		recording := toRecordingResponse(*item.Recording)
		response.Recording = &recording
	}
	JSONSuccess(c, http.StatusOK, gin.H{"room": response})
}

func (h *CollaborationHandler) handleSupportRecording(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	recordingID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid recording id")
		return
	}
	item, err := h.service.GetSupportRecording(c.Request.Context(), recordingID)
	if err != nil {
		JSONError(c, http.StatusInternalServerError, "failed to load support recording")
		return
	}
	response := supportRecordingResponse{
		Recording: toRecordingResponse(item.Recording),
	}
	if item.Room != nil {
		room := toRoomListItemResponse(*item.Room)
		response.Room = &room
	}
	if item.Policy != nil {
		policy := toOrganizationPolicyResponse(*item.Policy)
		response.Policy = &policy
	}
	response.Exports = item.Exports
	JSONSuccess(c, http.StatusOK, gin.H{"recording": response})
}
