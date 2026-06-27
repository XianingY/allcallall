package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/media"
)

func (h *CollaborationHandler) handleCreateRoom(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	var req collaboration.CreateRoomInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	state, err := h.service.CreateRoom(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusCreated, gin.H{"room": toRoomStateResponse(*state)})
}

func (h *CollaborationHandler) handleListRooms(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	items, err := h.service.ListRooms(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	response := make([]roomStateResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toRoomStateResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"rooms": response})
}

func (h *CollaborationHandler) handleJoinRoom(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	state, err := h.service.JoinRoom(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		code := ""
		status := http.StatusBadRequest
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			code = "ROOM_ACCESS_DENIED"
		} else if errors.Is(err, collaboration.ErrRoomParticipantLimit) {
			code = "ROOM_PARTICIPANT_LIMIT_REACHED"
			status = http.StatusConflict
		}
		JSONErrorWithCode(c, status, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"room": toRoomStateResponse(*state)})
}

func (h *CollaborationHandler) handleLeaveRoom(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	state, err := h.service.LeaveRoom(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			code = "ROOM_ACCESS_DENIED"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"room": toRoomStateResponse(*state)})
}

func (h *CollaborationHandler) handleRoomOffer(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	var req struct {
		SDP string `json:"sdp"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.service.HandleRoomOffer(c.Request.Context(), orgID, claims.UserID, roomID, req.SDP)
	if err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			code = "ROOM_ACCESS_DENIED"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{
		"room":   toRoomStateResponse(*result.State),
		"answer": result.Answer,
	})
}

func (h *CollaborationHandler) handleRoomIce(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	var payload media.ICECandidateInit
	if err := c.ShouldBindJSON(&payload); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.AddRoomICECandidate(c.Request.Context(), orgID, claims.UserID, roomID, payload); err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			code = "ROOM_ACCESS_DENIED"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleRoomMediaState(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	var req collaboration.RoomMediaStateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.UpdateRoomMediaState(c.Request.Context(), orgID, claims.UserID, roomID, req); err != nil {
		code := "ROOM_MEDIA_SYNC_FAILED"
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			code = "ROOM_ACCESS_DENIED"
		} else if strings.Contains(strings.ToLower(err.Error()), "required") {
			code = "ROOM_PARTICIPANT_STATE_INVALID"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleRoomSignalEvent(c *gin.Context, eventType string) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.SaveRoomSignalEvent(c.Request.Context(), orgID, claims.UserID, roomID, eventType, payload); err != nil {
		JSONError(c, http.StatusBadRequest, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}

func (h *CollaborationHandler) handleRoomState(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	state, err := h.service.GetRoomState(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			code = "ROOM_ACCESS_DENIED"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"room": toRoomStateResponse(*state)})
}
