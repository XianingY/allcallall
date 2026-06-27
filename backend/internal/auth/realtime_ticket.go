package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const RealtimeTicketTTL = 60 * time.Second

var (
	ErrInvalidRealtimeChannel = errors.New("invalid realtime channel")
	ErrRealtimeTicketInvalid  = errors.New("realtime ticket invalid or expired")
)

type RealtimeTicketService struct {
	redis *redis.Client
}

type realtimeTicketPayload struct {
	UserID  uint64 `json:"user_id"`
	Email   string `json:"email"`
	Channel string `json:"channel"`
}

func NewRealtimeTicketService(client *redis.Client) *RealtimeTicketService {
	return &RealtimeTicketService{redis: client}
}

func (s *RealtimeTicketService) Issue(ctx context.Context, claims *Claims, channel string) (string, time.Time, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel != "chat" && channel != "signaling" {
		return "", time.Time{}, ErrInvalidRealtimeChannel
	}
	if s == nil || s.redis == nil || claims == nil {
		return "", time.Time{}, ErrRealtimeTicketInvalid
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("generate realtime ticket: %w", err)
	}
	ticket := base64.RawURLEncoding.EncodeToString(raw)
	payload, err := json.Marshal(realtimeTicketPayload{UserID: claims.UserID, Email: claims.Email, Channel: channel})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal realtime ticket: %w", err)
	}
	if err := s.redis.Set(ctx, realtimeTicketKey(ticket), payload, RealtimeTicketTTL).Err(); err != nil {
		return "", time.Time{}, fmt.Errorf("store realtime ticket: %w", err)
	}
	return ticket, time.Now().Add(RealtimeTicketTTL), nil
}

func (s *RealtimeTicketService) Consume(ctx context.Context, ticket, channel string) (*Claims, error) {
	if s == nil || s.redis == nil || strings.TrimSpace(ticket) == "" {
		return nil, ErrRealtimeTicketInvalid
	}
	raw, err := s.redis.GetDel(ctx, realtimeTicketKey(ticket)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrRealtimeTicketInvalid
		}
		return nil, fmt.Errorf("consume realtime ticket: %w", err)
	}
	var payload realtimeTicketPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Channel != channel || payload.UserID == 0 || payload.Email == "" {
		return nil, ErrRealtimeTicketInvalid
	}
	return &Claims{UserID: payload.UserID, Email: payload.Email}, nil
}

func realtimeTicketKey(ticket string) string {
	digest := sha256.Sum256([]byte(ticket))
	return fmt.Sprintf("realtime-ticket:%x", digest[:])
}

// RealtimeMiddleware authenticates a single-use Web ticket and keeps JWT query compatibility for native clients.
func RealtimeMiddleware(tickets *RealtimeTicketService, validator TokenValidator, channel string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ticket := strings.TrimSpace(c.Query("ticket")); ticket != "" {
			claims, err := tickets.Consume(c.Request.Context(), ticket, channel)
			if err != nil {
				abortAuthError(c, "REALTIME_TICKET_INVALID", "invalid or expired realtime ticket")
				return
			}
			SetClaimsToContext(c, claims)
			c.Next()
			return
		}

		token := extractToken(c.Request.Header.Get("Authorization"))
		if token == "" {
			token = strings.TrimSpace(c.Query("token"))
		}
		if token == "" {
			abortAuthError(c, authTokenMissingCode, "missing realtime ticket or bearer token")
			return
		}
		claims, err := validator.ValidateAccessToken(c.Request.Context(), token)
		if err != nil {
			abortAuthError(c, authTokenInvalidCode, "invalid token")
			return
		}
		SetClaimsToContext(c, claims)
		c.Next()
	}
}

func RealtimeTicketErrorStatus(err error) int {
	if errors.Is(err, ErrInvalidRealtimeChannel) {
		return http.StatusBadRequest
	}
	return http.StatusServiceUnavailable
}
