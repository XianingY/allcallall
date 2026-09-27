package handlers

import (
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/apperror"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/trace"
)

// JSONError 返回标准错误响应
// JSONError sends a JSON error message with status code.
func JSONError(c *gin.Context, status int, message string) {
	JSONErrorWithCode(c, status, "", message)
}

// JSONErrorWithCode sends a JSON error message with a stable machine-readable code.
func JSONErrorWithCode(c *gin.Context, status int, code string, message string) {
	code = strings.TrimSpace(code)
	if code == "" {
		code = defaultErrorCode(status)
	}
	requestID := c.GetString("X-Request-ID")
	if requestID == "" && c.Request != nil {
		requestID = trace.RequestID(c.Request.Context())
	}
	c.JSON(status, gin.H{
		"error":      message,
		"code":       code,
		"request_id": requestID,
		"success":    false,
	})
}

// JSONAppError automatically maps an error to the appropriate JSONErrorWithCode response.
func JSONAppError(c *gin.Context, err error) {
	if appErr, ok := err.(*apperror.AppError); ok {
		JSONErrorWithCode(c, appErr.HTTPStatus, appErr.Code, appErr.Message)
		return
	}
	// Fallback for unhandled errors
	JSONErrorWithCode(c, http.StatusInternalServerError, apperror.ErrCodeInternalServerError, err.Error())
}

func defaultErrorCode(status int) string {
	text := http.StatusText(status)
	if strings.TrimSpace(text) == "" {
		return "ERROR"
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToUpper(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	code := strings.Trim(builder.String(), "_")
	if code == "" {
		return "ERROR"
	}
	return code
}

// JSONSuccess 返回成功响应
// JSONSuccess sends JSON with optional data.
func JSONSuccess(c *gin.Context, status int, data interface{}) {
	if data == nil {
		data = gin.H{"success": true}
	}
	if status == 0 {
		status = http.StatusOK
	}
	c.JSON(status, data)
}

// collaborationClientErrors is a fail-closed whitelist: only these fixed,
// user-actionable messages may be echoed to clients. Unmatched errors fall
// back to a generic message so internals (SQL, file paths) never reach the wire.
var collaborationClientErrors = []error{
	collaboration.ErrOrganizationAccessDenied,
	collaboration.ErrConversationAccessDenied,
	collaboration.ErrInvalidConversationType,
	collaboration.ErrConversationMembersRequired,
	collaboration.ErrDirectConversationMemberCount,
	collaboration.ErrAssigneeNotConversationMember,
	collaboration.ErrInvalidConversationStatus,
	collaboration.ErrInvalidConversationPriority,
	collaboration.ErrInvalidMessageType,
	collaboration.ErrMessageBodyRequired,
	collaboration.ErrReplyTargetNotFound,
	collaboration.ErrAttachmentsUnavailable,
	collaboration.ErrInvalidRole,
	collaboration.ErrLastOwnerRequired,
	collaboration.ErrTeamNameRequired,
	gorm.ErrRecordNotFound,
}

// collaborationClientMessage returns the whitelist sentinel's own message
// (not err.Error(), which may carry fmt.Errorf wrapper prefixes).
func collaborationClientMessage(err error) (string, bool) {
	for _, sentinel := range collaborationClientErrors {
		if errors.Is(err, sentinel) {
			return sentinel.Error(), true
		}
	}
	return "", false
}

// writeServiceError: whitelisted domain errors keep 400 + their own message;
// everything else is logged server-side and answered with a generic 500 so
// internals never leak to the client.
func (h *CollaborationHandler) writeServiceError(c *gin.Context, err error, genericMessage string) {
	if message, ok := collaborationClientMessage(err); ok {
		JSONError(c, http.StatusBadRequest, message)
		return
	}
	h.logger.Error().Err(err).Str("path", c.Request.URL.Path).Msg(genericMessage)
	JSONError(c, http.StatusInternalServerError, genericMessage)
}
