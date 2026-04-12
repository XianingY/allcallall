package contact

import (
	"context"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

type ContactWithProfile struct {
	models.User
	Company           string `gorm:"column:profile_company"`
	Role              string `gorm:"column:profile_role"`
	Timezone          string `gorm:"column:profile_timezone"`
	DefaultSourceLang string `gorm:"column:profile_default_source_lang"`
	DefaultTargetLang string `gorm:"column:profile_default_target_lang"`
	RelationshipStatus string `gorm:"column:profile_relationship_status"`
	PreferredContactStart string `gorm:"column:profile_preferred_contact_start"`
	PreferredContactEnd   string `gorm:"column:profile_preferred_contact_end"`
	PreferredContactDays  string `gorm:"column:profile_preferred_contact_days"`
	LastFollowupState     string `gorm:"column:profile_last_followup_state"`
	Note              string `gorm:"column:profile_note"`
}

// Repository 联系人与关系数据访问
// Repository handles database operations for contacts.
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造函数
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// AddContact 创建联系人关系
func (r *Repository) AddContact(ctx context.Context, ownerID, contactID uint64) error {
	contact := &models.Contact{
		OwnerID:   ownerID,
		ContactID: contactID,
	}
	return r.db.WithContext(ctx).Create(contact).Error
}

// AddContactsBidirectional creates both directional relationships if missing.
func (r *Repository) AddContactsBidirectional(ctx context.Context, userA, userB uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, pair := range [][2]uint64{{userA, userB}, {userB, userA}} {
			entry := &models.Contact{
				OwnerID:   pair[0],
				ContactID: pair[1],
			}
			if err := tx.Where("owner_id = ? AND contact_id = ?", pair[0], pair[1]).FirstOrCreate(entry).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveContact 删除联系人关系
func (r *Repository) RemoveContact(ctx context.Context, ownerID, contactID uint64) error {
	return r.db.WithContext(ctx).
		Where("owner_id = ? AND contact_id = ?", ownerID, contactID).
		Delete(&models.Contact{}).Error
}

// ContactExists 检查联系人是否存在
func (r *Repository) ContactExists(ctx context.Context, ownerID, contactID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.Contact{}).
		Where("owner_id = ? AND contact_id = ?", ownerID, contactID).
		Count(&count).Error
	return count > 0, err
}

// ListContacts 列出联系人（返回用户信息）
func (r *Repository) ListContacts(ctx context.Context, ownerID uint64) ([]models.User, error) {
	var users []models.User
	err := r.db.WithContext(ctx).
		Table("contacts").
		Select("users.*").
		Joins("JOIN users ON contacts.contact_id = users.id").
		Where("contacts.owner_id = ?", ownerID).
		Order("users.display_name ASC").
		Find(&users).Error
	return users, err
}

func (r *Repository) ListContactsWithProfiles(ctx context.Context, ownerID uint64) ([]ContactWithProfile, error) {
	var rows []ContactWithProfile
	err := r.db.WithContext(ctx).
		Table("contacts").
		Select(`users.*,
			contact_profiles.company AS profile_company,
			contact_profiles.role AS profile_role,
			contact_profiles.timezone AS profile_timezone,
			contact_profiles.default_source_lang AS profile_default_source_lang,
			contact_profiles.default_target_lang AS profile_default_target_lang,
			contact_profiles.relationship_status AS profile_relationship_status,
			contact_profiles.preferred_contact_start AS profile_preferred_contact_start,
			contact_profiles.preferred_contact_end AS profile_preferred_contact_end,
			contact_profiles.preferred_contact_days AS profile_preferred_contact_days,
			contact_profiles.last_followup_state AS profile_last_followup_state,
			contact_profiles.note AS profile_note`).
		Joins("JOIN users ON contacts.contact_id = users.id").
		Joins("LEFT JOIN contact_profiles ON contact_profiles.owner_id = contacts.owner_id AND contact_profiles.contact_user_id = contacts.contact_id").
		Where("contacts.owner_id = ?", ownerID).
		Order("users.display_name ASC").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) GetContactProfile(ctx context.Context, ownerID, contactID uint64) (*models.ContactProfile, error) {
	var profile models.ContactProfile
	if err := r.db.WithContext(ctx).
		Where("owner_id = ? AND contact_user_id = ?", ownerID, contactID).
		Take(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *Repository) UpsertContactProfile(ctx context.Context, profile *models.ContactProfile) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.ContactProfile
		err := tx.Where("owner_id = ? AND contact_user_id = ?", profile.OwnerID, profile.ContactUserID).Take(&existing).Error
		if err == nil {
			existing.Company = profile.Company
			existing.Role = profile.Role
			existing.Timezone = profile.Timezone
			existing.DefaultSourceLang = profile.DefaultSourceLang
			existing.DefaultTargetLang = profile.DefaultTargetLang
			existing.RelationshipStatus = profile.RelationshipStatus
			existing.PreferredContactStart = profile.PreferredContactStart
			existing.PreferredContactEnd = profile.PreferredContactEnd
			existing.PreferredContactDays = profile.PreferredContactDays
			existing.LastFollowupState = profile.LastFollowupState
			existing.Note = profile.Note
			return tx.Save(&existing).Error
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		return tx.Create(profile).Error
	})
}
