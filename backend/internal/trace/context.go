package trace

import (
	"context"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

type contextKey string

const MaxRequestIDLength = 96

const (
	requestIDKey contextKey = "request_id"
	outboxIDKey  contextKey = "outbox_id"
)

func EnsureRequestID(requestID string) string {
	if requestID = NormalizeRequestID(requestID); requestID != "" {
		return requestID
	}
	return uuid.NewString()
}

func NormalizeRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > MaxRequestIDLength {
		return ""
	}
	for _, r := range requestID {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ""
		}
	}
	return requestID
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	requestID = NormalizeRequestID(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestIDKey).(string)
	return NormalizeRequestID(value)
}

func WithOutboxID(ctx context.Context, outboxID uint64) context.Context {
	if outboxID == 0 {
		return ctx
	}
	return context.WithValue(ctx, outboxIDKey, outboxID)
}

func OutboxID(ctx context.Context) uint64 {
	if ctx == nil {
		return 0
	}
	value, _ := ctx.Value(outboxIDKey).(uint64)
	return value
}
