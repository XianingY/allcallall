package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

var (
	ErrInvalidRefreshSession      = errors.New("invalid refresh session")
	ErrCannotRevokeCurrentSession = errors.New("cannot revoke current refresh session")
)

type RefreshSessionService struct {
	db      *gorm.DB
	metrics refreshSessionMetrics
}

type refreshSessionMetrics interface {
	Inc(name string)
}

type RefreshSessionInput struct {
	Token     string
	UserAgent string
	IPAddress string
	ExpiresAt time.Time
}

type RefreshSessionCleanupResult struct {
	Deleted int
}

type RefreshSessionView struct {
	ID               uint64     `json:"id"`
	Status           string     `json:"status"`
	Current          bool       `json:"current"`
	UserAgent        string     `json:"user_agent"`
	IPAddress        string     `json:"ip_address"`
	ExpiresAt        time.Time  `json:"expires_at"`
	LastUsedAt       *time.Time `json:"last_used_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
	InvalidUseCount  int        `json:"invalid_use_count"`
	LastInvalidUseAt *time.Time `json:"last_invalid_use_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func NewRefreshSessionService(db *gorm.DB, counters ...refreshSessionMetrics) *RefreshSessionService {
	var metrics refreshSessionMetrics
	if len(counters) > 0 {
		metrics = counters[0]
	}
	return &RefreshSessionService{db: db, metrics: metrics}
}

func (s *RefreshSessionService) Create(ctx context.Context, userID uint64, in RefreshSessionInput) (*models.RefreshSession, error) {
	session := &models.RefreshSession{
		UserID:    userID,
		TokenHash: refreshTokenHash(in.Token),
		UserAgent: trimMax(in.UserAgent, 255),
		IPAddress: trimMax(in.IPAddress, 64),
		ExpiresAt: in.ExpiresAt,
	}
	if session.TokenHash == "" {
		return nil, ErrInvalidRefreshSession
	}
	if session.ExpiresAt.IsZero() {
		return nil, ErrInvalidRefreshSession
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, err
	}
	return session, nil
}

func (s *RefreshSessionService) Validate(ctx context.Context, token string, now time.Time) (*models.RefreshSession, error) {
	var session models.RefreshSession
	err := s.db.WithContext(ctx).
		Where("token_hash = ? AND revoked_at IS NULL AND expires_at > ?", refreshTokenHash(token), now).
		Take(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidRefreshSession
		}
		return nil, err
	}
	return &session, nil
}

func (s *RefreshSessionService) Rotate(ctx context.Context, currentToken string, userID uint64, next RefreshSessionInput, now time.Time) (*models.RefreshSession, error) {
	var replacement *models.RefreshSession
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		nested := &RefreshSessionService{db: tx}
		current, err := nested.Validate(ctx, currentToken, now)
		if err != nil {
			return err
		}
		if current.UserID != userID {
			return ErrInvalidRefreshSession
		}
		replacement, err = nested.Create(ctx, userID, next)
		if err != nil {
			return err
		}
		updates := map[string]any{
			"revoked_at":     now,
			"last_used_at":   now,
			"replaced_by_id": replacement.ID,
		}
		return tx.Model(&models.RefreshSession{}).
			Where("id = ? AND revoked_at IS NULL", current.ID).
			Updates(updates).Error
	}); err != nil {
		if errors.Is(err, ErrInvalidRefreshSession) {
			if recordErr := s.RecordInvalidUse(ctx, currentToken, now); recordErr != nil {
				return nil, recordErr
			}
		}
		return nil, err
	}
	return replacement, nil
}

func (s *RefreshSessionService) RevokeByToken(ctx context.Context, token string, now time.Time) error {
	hash := refreshTokenHash(token)
	if hash == "" {
		return nil
	}
	return s.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("token_hash = ? AND revoked_at IS NULL", hash).
		Updates(map[string]any{
			"revoked_at":   now,
			"last_used_at": now,
		}).Error
}

func (s *RefreshSessionService) RevokeAllForUser(ctx context.Context, userID uint64, now time.Time) (int, error) {
	if userID == 0 {
		return 0, ErrInvalidRefreshSession
	}
	result := s.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]any{
			"revoked_at":   now,
			"last_used_at": now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

func (s *RefreshSessionService) RevokeForUserByID(ctx context.Context, userID uint64, sessionID uint64, currentToken string, now time.Time) error {
	if userID == 0 || sessionID == 0 {
		return ErrInvalidRefreshSession
	}

	var session models.RefreshSession
	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", sessionID, userID).
		Take(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidRefreshSession
		}
		return err
	}
	currentHash := refreshTokenHash(currentToken)
	if currentHash != "" && session.TokenHash == currentHash {
		return ErrCannotRevokeCurrentSession
	}
	if session.RevokedAt != nil {
		return nil
	}

	return s.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).
		Updates(map[string]any{
			"revoked_at":   now,
			"last_used_at": now,
		}).Error
}

func (s *RefreshSessionService) ListForUser(ctx context.Context, userID uint64, currentToken string, now time.Time, limit int) ([]RefreshSessionView, error) {
	if userID == 0 {
		return nil, ErrInvalidRefreshSession
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	var sessions []models.RefreshSession
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&sessions).Error; err != nil {
		return nil, err
	}

	currentHash := refreshTokenHash(currentToken)
	views := make([]RefreshSessionView, 0, len(sessions))
	for _, session := range sessions {
		views = append(views, RefreshSessionView{
			ID:               session.ID,
			Status:           refreshSessionStatus(session, now),
			Current:          currentHash != "" && session.TokenHash == currentHash,
			UserAgent:        session.UserAgent,
			IPAddress:        session.IPAddress,
			ExpiresAt:        session.ExpiresAt,
			LastUsedAt:       session.LastUsedAt,
			RevokedAt:        session.RevokedAt,
			InvalidUseCount:  session.InvalidUseCount,
			LastInvalidUseAt: session.LastInvalidUseAt,
			CreatedAt:        session.CreatedAt,
			UpdatedAt:        session.UpdatedAt,
		})
	}
	return views, nil
}

func (s *RefreshSessionService) RecordInvalidUse(ctx context.Context, token string, now time.Time) error {
	hash := refreshTokenHash(token)
	if hash == "" {
		return nil
	}
	result := s.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("token_hash = ?", hash).
		Updates(map[string]any{
			"invalid_use_count":   gorm.Expr("invalid_use_count + ?", 1),
			"last_invalid_use_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 && s.metrics != nil {
		s.metrics.Inc("refresh_session_invalid_use_total")
	}
	return nil
}

func (s *RefreshSessionService) CleanupExpired(ctx context.Context, now time.Time, revokedRetention time.Duration, limit int) (*RefreshSessionCleanupResult, error) {
	if limit <= 0 {
		limit = 500
	}
	if revokedRetention < 0 {
		revokedRetention = 0
	}

	revokedBefore := now.Add(-revokedRetention)
	var ids []uint64
	if err := s.db.WithContext(ctx).
		Model(&models.RefreshSession{}).
		Where("expires_at <= ? OR (revoked_at IS NOT NULL AND revoked_at <= ?)", now, revokedBefore).
		Order("id ASC").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &RefreshSessionCleanupResult{}, nil
	}

	result := s.db.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&models.RefreshSession{})
	if result.Error != nil {
		return nil, result.Error
	}
	return &RefreshSessionCleanupResult{Deleted: int(result.RowsAffected)}, nil
}

func refreshSessionStatus(session models.RefreshSession, now time.Time) string {
	if session.RevokedAt != nil {
		return "revoked"
	}
	if !session.ExpiresAt.After(now) {
		return "expired"
	}
	return "active"
}

func refreshTokenHash(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func trimMax(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}
