package handlers

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/apperror"
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
