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
	// Fallback for unhandled errors: fail closed with a fixed message so
	// internals (SQL, paths) never reach the wire.
	_ = c.Error(err)
	JSONErrorWithCode(c, http.StatusInternalServerError, apperror.ErrCodeInternalServerError, "internal server error")
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

// JSONBindingError answers request binding failures with 400. Binding errors
// mean the client sent malformed JSON or violated field validation: that is
// the client's fault and must never be reported as a 500. The raw validator
// text is dropped from the wire to stay fail-closed.
func JSONBindingError(c *gin.Context, err error) {
	_ = c.Error(err)
	JSONError(c, http.StatusBadRequest, "invalid request body")
}

// JSONServiceError is the fail-closed way to answer any handler error.
//
// Handlers used to call JSONError(c, 400, err.Error()) in ~85 places, which
// put GORM SQL, file paths and internal constraint text on the wire. This
// keeps whitelisted domain sentinels (400 + their own message) and answers
// everything else with a generic message, recording the real error through
// gin's error accumulator where middleware can log it.
//
// Unknown errors answer 500 rather than 400: if we cannot classify it, we
// cannot claim it was the client's fault. Do NOT pass ShouldBindJSON errors
// here — use JSONBindingError, which answers 400.
func JSONServiceError(c *gin.Context, err error, genericMessage string) {
	if message, ok := clientErrorMessage(err); ok {
		JSONError(c, http.StatusBadRequest, message)
		return
	}
	_ = c.Error(err)
	JSONError(c, http.StatusInternalServerError, genericMessage)
}

// clientErrorMessage generalises collaborationClientMessage with sentinels
// that any package may return.
func clientErrorMessage(err error) (string, bool) {
	if message, ok := collaborationClientMessage(err); ok {
		return message, true
	}
	for _, sentinel := range []error{gorm.ErrRecordNotFound} {
		if errors.Is(err, sentinel) {
			return "记录不存在 / not found", true
		}
	}
	return "", false
}

// writeServiceError: whitelisted domain errors keep 400 + their own message;
// everything else is logged server-side and answered with a generic 500 so
// internals never leak to the client.
func (h *CollaborationHandler) writeServiceError(c *gin.Context, err error, genericMessage string) {
	if message, ok := clientErrorMessage(err); ok {
		JSONError(c, http.StatusBadRequest, message)
		return
	}
	h.logger.Error().Err(err).Str("path", c.Request.URL.Path).Msg(genericMessage)
	JSONError(c, http.StatusInternalServerError, genericMessage)
}

// JSONServiceErrorCode answers a classified service error with a stable code
// and a FIXED message. The caller has already classified the error with
// errors.Is, so status/code semantics are trusted; the wrapped err.Error()
// may still carry internal detail and is recorded via gin's error
// accumulator instead of being echoed to the client.
func JSONServiceErrorCode(c *gin.Context, err error, status int, code string, message string) {
	_ = c.Error(err)
	JSONErrorWithCode(c, status, code, message)
}
