package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/collaboration"
)

func (h *CollaborationHandler) handleStartRecording(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	item, err := h.service.StartRecording(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrRecordingNotAllowed) {
			code = "RECORDING_NOT_ALLOWED"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"recording": toRecordingResponse(*item)})
}

func (h *CollaborationHandler) handleStopRecording(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	roomID, err := parseUintParam(c.Param("roomId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid room id")
		return
	}
	item, err := h.service.StopRecording(c.Request.Context(), orgID, claims.UserID, roomID)
	if err != nil {
		code := ""
		if errors.Is(err, collaboration.ErrRecordingNotAllowed) {
			code = "RECORDING_NOT_ALLOWED"
		}
		JSONErrorWithCode(c, http.StatusBadRequest, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"recording": toRecordingResponse(*item)})
}

func (h *CollaborationHandler) handleListRecordings(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	items, err := h.service.ListRecordings(c.Request.Context(), orgID, claims.UserID)
	if err != nil {
		JSONErrorWithCode(c, http.StatusBadRequest, "RECORDING_LIST_FAILED", err.Error())
		return
	}
	response := make([]recordingResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toRecordingResponse(item))
	}
	JSONSuccess(c, http.StatusOK, gin.H{"recordings": response})
}

func (h *CollaborationHandler) handleGetRecording(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	recordingID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid recording id")
		return
	}
	item, err := h.service.GetRecording(c.Request.Context(), orgID, claims.UserID, recordingID)
	if err != nil {
		JSONErrorWithCode(c, http.StatusBadRequest, "RECORDING_NOT_FOUND", err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"recording": toRecordingResponse(*item)})
}

func (h *CollaborationHandler) handleGetRecordingTranscript(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	recordingID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid recording id")
		return
	}
	afterID, err := parseOptionalTranscriptCursor(c.Query("after_id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid after_id")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 {
			JSONError(c, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	page, err := h.service.GetRecordingTranscript(c.Request.Context(), orgID, claims.UserID, recordingID, afterID, limit)
	if err != nil {
		JSONErrorWithCode(c, http.StatusNotFound, "RECORDING_TRANSCRIPT_NOT_FOUND", err.Error())
		return
	}
	JSONSuccess(c, http.StatusOK, page)
}

func (h *CollaborationHandler) handleRetryRecordingTranscription(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	recordingID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid recording id")
		return
	}
	item, err := h.service.RetryRecordingTranscription(c.Request.Context(), orgID, claims.UserID, recordingID)
	if err != nil {
		status := http.StatusBadRequest
		code := "RECORDING_TRANSCRIPTION_RETRY_FAILED"
		if errors.Is(err, collaboration.ErrRecordingNotAllowed) {
			status = http.StatusForbidden
			code = "RECORDING_TRANSCRIPTION_RETRY_FORBIDDEN"
		} else if errors.Is(err, collaboration.ErrTranscriptionNotRetryable) {
			status = http.StatusConflict
			code = "RECORDING_TRANSCRIPTION_NOT_RETRYABLE"
		}
		JSONErrorWithCode(c, status, code, err.Error())
		return
	}
	JSONSuccess(c, http.StatusAccepted, gin.H{"transcription": item})
}

func parseOptionalTranscriptCursor(raw string) (uint64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}

func (h *CollaborationHandler) handleDownloadRecordingFile(c *gin.Context) {
	claims, orgID, ok := h.requireCurrentOrganization(c)
	if !ok {
		return
	}
	recordingID, err := parseUintParam(c.Param("id"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid recording id")
		return
	}
	fileID, err := parseUintParam(c.Param("fileId"))
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid file id")
		return
	}
	_, file, err := h.service.GetRecordingFile(c.Request.Context(), orgID, claims.UserID, recordingID, fileID)
	if err != nil {
		JSONErrorWithCode(c, http.StatusNotFound, "RECORDING_DOWNLOAD_NOT_FOUND", "recording file not found")
		return
	}
	objectRef := collaboration.RecordingFileObjectRef(*file)
	signedURL, signedErr := h.service.GetRecordingDownloadURL(c.Request.Context(), objectRef)
	if signedErr == nil && (strings.HasPrefix(signedURL, "http://") || strings.HasPrefix(signedURL, "https://")) {
		expiresAt := time.Now().UTC().Add(15 * time.Minute)
		if err := h.service.RecordRecordingExportAudit(c.Request.Context(), recordingID, claims.UserID, fileID, &expiresAt); err != nil {
			h.logger.Error().Err(err).Uint64("recording_id", recordingID).Uint64("file_id", fileID).Msg("recording export audit failed")
			JSONErrorWithCode(c, http.StatusInternalServerError, "RECORDING_EXPORT_AUDIT_FAILED", "recording export audit failed")
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, signedURL)
		return
	}
	path, ok := h.service.ResolveLocalRecordingPath(objectRef)
	if !ok || strings.TrimSpace(path) == "" {
		JSONErrorWithCode(c, http.StatusNotFound, "RECORDING_DOWNLOAD_NOT_FOUND", "recording file path missing")
		return
	}
	info, statErr := os.Stat(path)
	if statErr != nil || info.IsDir() {
		JSONErrorWithCode(c, http.StatusNotFound, "RECORDING_DOWNLOAD_NOT_FOUND", "recording file not found")
		return
	}
	filename := filepath.Base(path)
	if file.ContentType != "" {
		c.Header("Content-Type", file.ContentType)
	}
	if err := h.service.RecordRecordingExportAudit(c.Request.Context(), recordingID, claims.UserID, fileID, nil); err != nil {
		h.logger.Error().Err(err).Uint64("recording_id", recordingID).Uint64("file_id", fileID).Msg("recording export audit failed")
		JSONErrorWithCode(c, http.StatusInternalServerError, "RECORDING_EXPORT_AUDIT_FAILED", "recording export audit failed")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.File(path)
}
