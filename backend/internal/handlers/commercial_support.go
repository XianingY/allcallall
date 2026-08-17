package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func (h *CommercialHandler) requireSupportToken(c *gin.Context) bool {
	if !requireSupportNetwork(c) {
		return false
	}
	expected := strings.TrimSpace(os.Getenv("SUPPORT_API_TOKEN"))
	if expected == "" {
		JSONErrorWithCode(c, http.StatusServiceUnavailable, "SUPPORT_TOKEN_NOT_CONFIGURED", "support api token is not configured")
		return false
	}
	if strings.TrimSpace(c.GetHeader("X-Support-Token")) != expected {
		JSONErrorWithCode(c, http.StatusUnauthorized, "SUPPORT_UNAUTHORIZED", "unauthorized support request")
		return false
	}
	return true
}

func (h *CommercialHandler) handleSupportReports(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	reports, err := h.commerce.ListSupportReports(c.Request.Context(), 100)
	if err != nil {
		h.logger.Error().Err(err).Msg("list support reports failed")
		JSONError(c, http.StatusInternalServerError, "failed to load support reports")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"reports": reports, "categories": h.commerce.ReportCategories()})
}

func (h *CommercialHandler) handleSupportUserSummary(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	userID, err := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err != nil {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	summary, err := h.commerce.GetSupportUserSummary(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", userID).Msg("get support user summary failed")
		JSONError(c, http.StatusInternalServerError, "failed to load support user summary")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"summary": summary})
}

func (h *CommercialHandler) handleSupportRevokeUserSessions(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	userID, err := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err != nil || userID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	result, err := h.commerce.RevokeSupportRefreshSessions(c.Request.Context(), userID, nil)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", userID).Msg("support revoke user sessions failed")
		JSONError(c, http.StatusInternalServerError, "failed to revoke user sessions")
		return
	}
	h.logger.Warn().Uint64("user_id", userID).Int64("revoked_sessions", result.RevokedSessions).Msg("support revoked user refresh sessions")
	JSONSuccess(c, http.StatusOK, gin.H{"revocation": result})
}

func (h *CommercialHandler) handleSupportRevokeUserSession(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	userID, err := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err != nil || userID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid user id")
		return
	}
	sessionID, err := strconv.ParseUint(c.Param("sessionId"), 10, 64)
	if err != nil || sessionID == 0 {
		JSONError(c, http.StatusBadRequest, "invalid session id")
		return
	}
	result, err := h.commerce.RevokeSupportRefreshSessions(c.Request.Context(), userID, &sessionID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("user_id", userID).Uint64("session_id", sessionID).Msg("support revoke user session failed")
		JSONError(c, http.StatusInternalServerError, "failed to revoke user session")
		return
	}
	h.logger.Warn().Uint64("user_id", userID).Uint64("session_id", sessionID).Int64("revoked_sessions", result.RevokedSessions).Msg("support revoked user refresh session")
	JSONSuccess(c, http.StatusOK, gin.H{"revocation": result})
}

func (h *CommercialHandler) handleSupportCall(c *gin.Context) {
	if !h.requireSupportToken(c) {
		return
	}
	callID := strings.TrimSpace(c.Param("callId"))
	if callID == "" {
		JSONError(c, http.StatusBadRequest, "invalid call id")
		return
	}
	call, err := h.commerce.GetSupportCall(c.Request.Context(), callID)
	if err != nil {
		h.logger.Error().Err(err).Str("call_id", callID).Msg("get support call failed")
		JSONError(c, http.StatusInternalServerError, "failed to load support call")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{
		"call":                call.Call,
		"translation_slices":  call.TranslationSlices,
		"transcript_segments": call.TranscriptSegments,
		"followup":            call.Followup,
		"tasks":               call.Tasks,
	})
}

func (h *CommercialHandler) sendSupportReportEmail(reporterID, reportedUserID uint64, category, details string) {
	if h.mail == nil {
		return
	}

	legal := h.commerce.CurrentLegal()
	subject := fmt.Sprintf("AllCallAll 举报通知 / Abuse Report [%s]", strings.TrimSpace(category))
	body := fmt.Sprintf(`
		<html>
			<body style="font-family: Arial, sans-serif; color: #0f172a;">
				<h2>新的用户举报</h2>
				<p><strong>分类：</strong>%s</p>
				<p><strong>举报人 ID：</strong>%d</p>
				<p><strong>被举报用户 ID：</strong>%d</p>
				<p><strong>补充说明：</strong>%s</p>
			</body>
		</html>
	`, template.HTMLEscapeString(strings.TrimSpace(category)), reporterID, reportedUserID, template.HTMLEscapeString(strings.TrimSpace(details)))

	if err := h.mail.SendHTMLEmail(legal.SupportEmail, subject, body); err != nil {
		if h.metrics != nil {
			h.metrics.Inc("abuse_report_email_fail_total")
		}
		h.logger.Error().
			Err(err).
			Uint64("reporter_id", reporterID).
			Uint64("reported_user_id", reportedUserID).
			Str("category", strings.TrimSpace(category)).
			Msg("support abuse report email failed")
	}
}
