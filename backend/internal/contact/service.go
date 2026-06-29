package contact

import (
	"context"
	"errors"
	"strings"

	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
	"gorm.io/gorm"
)

// Service 联系人业务逻辑
// Service coordinates contact operations with user service.
type Service struct {
	repo       *Repository
	users      *user.Service
	commercial *commerce.Service
}

// NewService 构造函数
func NewService(repo *Repository, users *user.Service, commercial ...*commerce.Service) *Service {
	var commercialSvc *commerce.Service
	if len(commercial) > 0 {
		commercialSvc = commercial[0]
	}
	return &Service{
		repo:       repo,
		users:      users,
		commercial: commercialSvc,
	}
}

// ErrContactExists 联系人已存在
var ErrContactExists = errors.New("contact already exists")

// ErrSelfContact 不能添加自己为联系人
var ErrSelfContact = errors.New("cannot add yourself as contact")

// ErrContactNotFound indicates the directional contact relation does not exist.
var ErrContactNotFound = errors.New("contact not found")

type ContactRecord struct {
	models.User
	Company               string `json:"company,omitempty"`
	Role                  string `json:"role,omitempty"`
	Timezone              string `json:"timezone,omitempty"`
	DefaultSourceLang     string `json:"default_source_lang,omitempty"`
	DefaultTargetLang     string `json:"default_target_lang,omitempty"`
	RelationshipStatus    string `json:"relationship_status,omitempty"`
	PreferredContactStart string `json:"preferred_contact_start,omitempty"`
	PreferredContactEnd   string `json:"preferred_contact_end,omitempty"`
	PreferredContactDays  string `json:"preferred_contact_days,omitempty"`
	LastFollowupState     string `json:"last_followup_state,omitempty"`
	Note                  string `json:"note,omitempty"`
}

type ContactProfileInput struct {
	Company               string
	Role                  string
	Timezone              string
	DefaultSourceLang     string
	DefaultTargetLang     string
	RelationshipStatus    string
	PreferredContactStart string
	PreferredContactEnd   string
	PreferredContactDays  string
	LastFollowupState     string
	Note                  string
}

// AddByEmail 通过邮箱添加联系人
func (s *Service) AddByEmail(ctx context.Context, ownerID uint64, ownerEmail, targetEmail string) error {
	target, err := s.users.GetByEmail(ctx, targetEmail)
	if err != nil {
		return err
	}
	if target.ID == ownerID {
		return ErrSelfContact
	}
	if s.commercial != nil {
		blocked, err := s.commercial.AreUsersBlocked(ctx, ownerID, target.ID)
		if err != nil {
			return err
		}
		if blocked {
			return commerce.ErrUserBlocked
		}
	}

	exists, err := s.repo.ContactExists(ctx, ownerID, target.ID)
	if err != nil {
		return err
	}
	if exists {
		return ErrContactExists
	}

	return s.repo.AddContact(ctx, ownerID, target.ID)
}

// Remove 删除联系人
func (s *Service) Remove(ctx context.Context, ownerID, contactID uint64) error {
	return s.repo.RemoveContact(ctx, ownerID, contactID)
}

// List 列出所有联系人
func (s *Service) List(ctx context.Context, ownerID uint64) ([]models.User, error) {
	return s.repo.ListContacts(ctx, ownerID)
}

func (s *Service) ListWithProfiles(ctx context.Context, ownerID uint64) ([]ContactRecord, error) {
	rows, err := s.repo.ListContactsWithProfiles(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	result := make([]ContactRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, ContactRecord{
			User:                  row.User,
			Company:               row.Company,
			Role:                  row.Role,
			Timezone:              row.Timezone,
			DefaultSourceLang:     row.DefaultSourceLang,
			DefaultTargetLang:     row.DefaultTargetLang,
			RelationshipStatus:    row.RelationshipStatus,
			PreferredContactStart: row.PreferredContactStart,
			PreferredContactEnd:   row.PreferredContactEnd,
			PreferredContactDays:  row.PreferredContactDays,
			LastFollowupState:     row.LastFollowupState,
			Note:                  row.Note,
		})
	}
	return result, nil
}

func (s *Service) EnsureBidirectional(ctx context.Context, userA, userB uint64) error {
	if userA == userB {
		return ErrSelfContact
	}
	if s.commercial != nil {
		blocked, err := s.commercial.AreUsersBlocked(ctx, userA, userB)
		if err != nil {
			return err
		}
		if blocked {
			return commerce.ErrUserBlocked
		}
	}
	return s.repo.AddContactsBidirectional(ctx, userA, userB)
}

func (s *Service) GetProfile(ctx context.Context, ownerID, contactID uint64) (*models.ContactProfile, error) {
	exists, err := s.repo.ContactExists(ctx, ownerID, contactID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrContactNotFound
	}
	profile, err := s.repo.GetContactProfile(ctx, ownerID, contactID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &models.ContactProfile{
				OwnerID:       ownerID,
				ContactUserID: contactID,
			}, nil
		}
		return nil, err
	}
	return profile, nil
}

func (s *Service) SaveProfile(ctx context.Context, ownerID, contactID uint64, in ContactProfileInput) (*models.ContactProfile, error) {
	exists, err := s.repo.ContactExists(ctx, ownerID, contactID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrContactNotFound
	}

	profile := &models.ContactProfile{
		OwnerID:               ownerID,
		ContactUserID:         contactID,
		Company:               strings.TrimSpace(in.Company),
		Role:                  strings.TrimSpace(in.Role),
		Timezone:              strings.TrimSpace(in.Timezone),
		DefaultSourceLang:     strings.TrimSpace(strings.ToLower(in.DefaultSourceLang)),
		DefaultTargetLang:     strings.TrimSpace(strings.ToLower(in.DefaultTargetLang)),
		RelationshipStatus:    strings.TrimSpace(strings.ToLower(in.RelationshipStatus)),
		PreferredContactStart: strings.TrimSpace(in.PreferredContactStart),
		PreferredContactEnd:   strings.TrimSpace(in.PreferredContactEnd),
		PreferredContactDays:  strings.TrimSpace(in.PreferredContactDays),
		LastFollowupState:     strings.TrimSpace(strings.ToLower(in.LastFollowupState)),
		Note:                  strings.TrimSpace(in.Note),
	}
	if profile.RelationshipStatus == "" {
		profile.RelationshipStatus = "new"
	}
	if err := s.repo.UpsertContactProfile(ctx, profile); err != nil {
		return nil, err
	}
	return s.repo.GetContactProfile(ctx, ownerID, contactID)
}
