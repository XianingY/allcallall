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

var ErrInvalidRefreshSession = errors.New("invalid refresh session")

type RefreshSessionService struct {
	db *gorm.DB
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

func NewRefreshSessionService(db *gorm.DB) *RefreshSessionService {
	return &RefreshSessionService{db: db}
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
