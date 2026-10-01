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
		JSONBindingError(c, err)
		return
	}
	state, err := h.service.CreateRoom(c.Request.Context(), orgID, claims.UserID, req)
	if err != nil {
		JSONServiceError(c, err, "failed to complete the room operation")
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
		JSONServiceError(c, err, "failed to complete the room operation")
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
		switch {
		case errors.Is(err, collaboration.ErrRoomAccessDenied):
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_ACCESS_DENIED", "room access denied")
		case errors.Is(err, collaboration.ErrRoomParticipantLimit):
			JSONServiceErrorCode(c, err, http.StatusConflict, "ROOM_PARTICIPANT_LIMIT_REACHED", "room participant limit reached")
		default:
			h.writeServiceError(c, err, "failed to join room")
		}
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
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_ACCESS_DENIED", "room access denied")
		} else {
			h.writeServiceError(c, err, "failed to leave room")
		}
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
		JSONBindingError(c, err)
		return
	}
	result, err := h.service.HandleRoomOffer(c.Request.Context(), orgID, claims.UserID, roomID, req.SDP)
	if err != nil {
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_ACCESS_DENIED", "room access denied")
		} else {
			h.writeServiceError(c, err, "failed to handle room offer")
		}
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{
		"room":        toRoomStateResponse(*result.State),
		"answer":      result.Answer,
		"trickle_ice": result.TrickleICE,
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
		JSONBindingError(c, err)
		return
	}
	if err := h.service.AddRoomICECandidate(c.Request.Context(), orgID, claims.UserID, roomID, payload); err != nil {
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_ACCESS_DENIED", "room access denied")
		} else {
			h.writeServiceError(c, err, "failed to add room ICE candidate")
		}
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
		JSONBindingError(c, err)
		return
	}
	if err := h.service.UpdateRoomMediaState(c.Request.Context(), orgID, claims.UserID, roomID, req); err != nil {
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_ACCESS_DENIED", "room access denied")
		} else if strings.Contains(strings.ToLower(err.Error()), "required") {
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_PARTICIPANT_STATE_INVALID", "room participant state invalid")
		} else {
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_MEDIA_SYNC_FAILED", "room media state sync failed")
		}
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
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_ACCESS_DENIED", "room access denied")
		} else {
			h.writeServiceError(c, err, "failed to get room state")
		}
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"room": toRoomStateResponse(*state)})
}

func (h *CollaborationHandler) handleRoomRenegotiationAnswer(c *gin.Context) {
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
		JSONBindingError(c, err)
		return
	}
	if err := h.service.HandleRoomRenegotiationAnswer(c.Request.Context(), orgID, claims.UserID, roomID, req.SDP); err != nil {
		if errors.Is(err, collaboration.ErrRoomAccessDenied) {
			JSONServiceErrorCode(c, err, http.StatusBadRequest, "ROOM_ACCESS_DENIED", "room access denied")
		} else {
			h.writeServiceError(c, err, "failed to handle room renegotiation answer")
		}
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"success": true})
}
