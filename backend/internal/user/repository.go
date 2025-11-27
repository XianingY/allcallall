package user

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// Repository 用户数据访问层
// Repository encapsulates database operations for users.
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造函数
// NewRepository creates a new user repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ErrNotFound 用户不存在错误
// ErrNotFound signals that the user record was not located.
var ErrNotFound = gorm.ErrRecordNotFound

// Create 保存用户
// Create persists a new user record.
func (r *Repository) Create(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// FindByEmail 根据邮箱查找用户
// FindByEmail fetches a user by email address.
func (r *Repository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("LOWER(email) = ?", strings.ToLower(email)).Take(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByID 根据 ID 查找用户
// FindByID returns user by primary key.
func (r *Repository) FindByID(ctx context.Context, id uint64) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// SearchByEmail 查询匹配邮箱的用户（模糊搜索）
// SearchByEmail performs case-insensitive search by email substring.
func (r *Repository) SearchByEmail(ctx context.Context, query string, limit int) ([]models.User, error) {
	if limit <= 0 {
		limit = 10
	}

	q := strings.ToLower(query)
	var users []models.User
	err := r.db.WithContext(ctx).
		Where("LOWER(email) LIKE ?", "%"+q+"%").
		Order("created_at DESC").
		Limit(limit).
		Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

// UpdateLastSeen 更新用户最后在线时间
// UpdateLastSeen updates the last_seen timestamp.
func (r *Repository) UpdateLastSeen(ctx context.Context, userID uint64, t *time.Time) error {
	return r.db.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", userID).
		Update("last_seen", t).Error
}

// UpdatePassword 更新用户密码
// UpdatePassword updates user password hash.
func (r *Repository) UpdatePassword(ctx context.Context, userID uint64, passwordHash string) error {
	return r.db.WithContext(ctx).Model(&models.User{}).
		Where("id = ?", userID).
		Update("password_hash", passwordHash).Error
}
