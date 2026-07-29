package invitation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

// log 是该包内的包级最低限度日志器，用于记录关键路径上被吞掉的错误。
// log is the package-level fallback logger for swallowed errors on critical paths.
var log = zerolog.New(os.Stderr).With().Timestamp().Logger()

var (
	ErrInvitationNotFound      = errors.New("invitation not found")
	ErrInvitationExpired       = errors.New("invitation expired")
	ErrInvitationAlreadyUsed   = errors.New("invitation already accepted")
	ErrInvitationEmailMismatch = errors.New("invitation email mismatch")
)

type Service struct {
	db         *gorm.DB
	users      *user.Service
	contacts   *contact.Service
	commercial *commerce.Service
}

func NewService(db *gorm.DB, users *user.Service, contacts *contact.Service, commercial ...*commerce.Service) *Service {
	var commercialSvc *commerce.Service
	if len(commercial) > 0 {
		commercialSvc = commercial[0]
	}
	return &Service{db: db, users: users, contacts: contacts, commercial: commercialSvc}
}

type CreateInvitationInput struct {
	InviterID          uint64
	InviterEmail       string
	InviterDisplayName string
	TargetEmail        string
	DefaultSourceLang  string
	DefaultTargetLang  string
	Note               string
	ExpiresAt          time.Time
}

func (s *Service) Create(ctx context.Context, in CreateInvitationInput) (*models.Invitation, error) {
	code, err := randomCode()
	if err != nil {
		return nil, err
	}
	invitation := &models.Invitation{
		Code:               code,
		InviterID:          in.InviterID,
		InviterEmail:       strings.TrimSpace(strings.ToLower(in.InviterEmail)),
		InviterDisplayName: strings.TrimSpace(in.InviterDisplayName),
		TargetEmail:        strings.TrimSpace(strings.ToLower(in.TargetEmail)),
		DefaultSourceLang:  normalizeLang(in.DefaultSourceLang),
		DefaultTargetLang:  normalizeLang(in.DefaultTargetLang),
		Note:               strings.TrimSpace(in.Note),
		Status:             models.InvitationStatusPending,
		ExpiresAt:          in.ExpiresAt.UTC(),
	}
	if invitation.ExpiresAt.IsZero() {
		invitation.ExpiresAt = time.Now().UTC().Add(7 * 24 * time.Hour)
	}
	if err := s.db.WithContext(ctx).Create(invitation).Error; err != nil {
		return nil, err
	}
	return invitation, nil
}

func (s *Service) GetByCode(ctx context.Context, code string) (*models.Invitation, error) {
	var invitation models.Invitation
	if err := s.db.WithContext(ctx).Where("code = ?", strings.TrimSpace(code)).Take(&invitation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvitationNotFound
		}
		return nil, err
	}
	if invitation.Status == models.InvitationStatusPending && invitation.ExpiresAt.Before(time.Now().UTC()) {
		if err := s.db.WithContext(ctx).Model(&models.Invitation{}).
			Where("id = ?", invitation.ID).
			Update("status", models.InvitationStatusExpired).Error; err != nil {
			log.Warn().Err(err).Uint64("invitation_id", invitation.ID).Msg("failed to mark invitation as expired")
		}
		invitation.Status = models.InvitationStatusExpired
	}
	return &invitation, nil
}

func (s *Service) Accept(ctx context.Context, code string, acceptUserID uint64, acceptEmail string) (*models.Invitation, error) {
	invitation, err := s.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if invitation.Status == models.InvitationStatusAccepted {
		return nil, ErrInvitationAlreadyUsed
	}
	if invitation.ExpiresAt.Before(time.Now().UTC()) || invitation.Status == models.InvitationStatusExpired {
		return nil, ErrInvitationExpired
	}
	normalizedEmail := strings.TrimSpace(strings.ToLower(acceptEmail))
	if invitation.TargetEmail != normalizedEmail {
		return nil, ErrInvitationEmailMismatch
	}
	if acceptUserID == invitation.InviterID {
		return nil, contact.ErrSelfContact
	}
	if s.commercial != nil {
		blocked, err := s.commercial.AreUsersBlocked(ctx, invitation.InviterID, acceptUserID)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, commerce.ErrUserBlocked
		}
	}

	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Invitation{}).
			Where("id = ? AND status = ?", invitation.ID, models.InvitationStatusPending).
			Updates(map[string]any{
				"status":           models.InvitationStatusAccepted,
				"accepted_user_id": acceptUserID,
				"accepted_at":      &now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrInvitationAlreadyUsed
		}
		for _, pair := range [][2]uint64{{invitation.InviterID, acceptUserID}, {acceptUserID, invitation.InviterID}} {
			entry := &models.Contact{
				OwnerID:   pair[0],
				ContactID: pair[1],
			}
			if err := tx.Where("owner_id = ? AND contact_id = ?", pair[0], pair[1]).FirstOrCreate(entry).Error; err != nil {
				return err
			}
		}
		if invitation.DefaultSourceLang != "" || invitation.DefaultTargetLang != "" || invitation.Note != "" {
			if err := upsertProfile(tx, invitation.InviterID, acceptUserID, invitation.DefaultSourceLang, invitation.DefaultTargetLang, invitation.Note); err != nil {
				return err
			}
		}
		if err := upsertProfile(tx, acceptUserID, invitation.InviterID, invitation.DefaultTargetLang, invitation.DefaultSourceLang, invitation.Note); err != nil {
			return err
		}
		return nil
	}); err != nil {
		if errors.Is(err, commerce.ErrUserBlocked) {
			return nil, err
		}
		return nil, err
	}
	return s.GetByCode(ctx, code)
}

func upsertProfile(tx *gorm.DB, ownerID, contactID uint64, sourceLang, targetLang, note string) error {
	var existing models.ContactProfile
	err := tx.Where("owner_id = ? AND contact_user_id = ?", ownerID, contactID).Take(&existing).Error
	if err == nil {
		existing.DefaultSourceLang = normalizeLang(sourceLang)
		existing.DefaultTargetLang = normalizeLang(targetLang)
		if strings.TrimSpace(note) != "" {
			existing.Note = strings.TrimSpace(note)
		}
		return tx.Save(&existing).Error
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&models.ContactProfile{
		OwnerID:           ownerID,
		ContactUserID:     contactID,
		DefaultSourceLang: normalizeLang(sourceLang),
		DefaultTargetLang: normalizeLang(targetLang),
		Note:              strings.TrimSpace(note),
	}).Error
}

func normalizeLang(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return ""
	}
	return normalized
}

func randomCode() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
