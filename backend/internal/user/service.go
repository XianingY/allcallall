package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/allcallall/backend/internal/models"
)

// Service 用户业务逻辑
// Service handles high-level user operations.
type Service struct {
	repo               *Repository
	pushDevicesEnabled bool
}

type ServiceOption func(*Service)

func WithPushDeviceSupport() ServiceOption {
	return func(s *Service) {
		s.pushDevicesEnabled = true
	}
}

// NewService 构造函数
// NewService constructs a Service.
func NewService(repo *Repository, options ...ServiceOption) *Service {
	svc := &Service{repo: repo}
	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}
	return svc
}

// RegisterInput 注册输入
// RegisterInput captures registration parameters.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

// LoginInput 登录输入
// LoginInput represents login credentials.
type LoginInput struct {
	Email    string
	Password string
}

// ErrEmailAlreadyUsed 邮箱已存在
// ErrEmailAlreadyUsed indicates the email is taken.
var ErrEmailAlreadyUsed = errors.New("email already registered")

// ErrInvalidCredentials 凭证无效
// ErrInvalidCredentials indicates wrong password or email.
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserDeleted = errors.New("user account deleted")

// Register 注册用户
// Register creates a new user with hashed password.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*models.User, error) {
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)

	if _, err := s.repo.FindByEmail(ctx, in.Email); err == nil {
		return nil, ErrEmailAlreadyUsed
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Email:        in.Email,
		PasswordHash: string(hash),
		DisplayName:  in.DisplayName,
		Status:       models.UserStatusActive,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Authenticate 验证用户
// Authenticate verifies email/password and returns user.
func (s *Service) Authenticate(ctx context.Context, in LoginInput) (*models.User, error) {
	user, err := s.repo.FindByEmail(ctx, strings.TrimSpace(strings.ToLower(in.Email)))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user.Status == models.UserStatusDeleted {
		return nil, ErrUserDeleted
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// SearchByEmail 搜索用户
// SearchByEmail searches users by email substring.
func (s *Service) SearchByEmail(ctx context.Context, query string, limit int) ([]models.User, error) {
	return s.repo.SearchByEmail(ctx, query, limit)
}

// GetByID 根据 ID 获取用户
// GetByID fetches user by ID.
func (s *Service) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	return s.repo.FindByID(ctx, id)
}

// GetByEmail 根据邮箱获取用户
// GetByEmail fetches user by email.
func (s *Service) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	return s.repo.FindByEmail(ctx, strings.TrimSpace(strings.ToLower(email)))
}

// UpdateLastSeen 更新最后在线时间
// UpdateLastSeen updates last seen timestamp.
func (s *Service) UpdateLastSeen(ctx context.Context, userID uint64, t *time.Time) error {
	return s.repo.UpdateLastSeen(ctx, userID, t)
}

// ChangePasswordInput 密码修改输入
// ChangePasswordInput represents password change parameters.
type ChangePasswordInput struct {
	OldPassword     string
	NewPassword     string
	ConfirmPassword string
}

// ChangePassword 修改用户密码
// ChangePassword updates user password after verifying old password.
func (s *Service) ChangePassword(ctx context.Context, userID uint64, in ChangePasswordInput) error {
	// 获取用户信息
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.OldPassword)); err != nil {
		return ErrInvalidCredentials
	}

	// 验证新密码的完整性（包括与旧密码的比较）
	if err := ValidatePasswordChange(in.OldPassword, in.NewPassword, in.ConfirmPassword); err != nil {
		return err
	}

	// 生成新的密码哈希
	newHash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// 更新数据库
	return s.repo.UpdatePassword(ctx, userID, string(newHash))
}

// SaveFCMToken 保存或更新用户的 FCM Token
// SaveFCMToken saves or updates user FCM token for push notifications.
func (s *Service) SaveFCMToken(ctx context.Context, userID uint64, fcmToken string) error {
	if strings.TrimSpace(fcmToken) == "" {
		return errors.New("fcm token cannot be empty")
	}
	return s.repo.UpdateFCMToken(ctx, userID, strings.TrimSpace(fcmToken))
}

type SavePushRegistrationInput struct {
	Token      string
	Provider   string
	Platform   string
	DeviceName string
	AppVersion string
}

func (s *Service) SavePushRegistration(ctx context.Context, userID uint64, in SavePushRegistrationInput) error {
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return errors.New("fcm token cannot be empty")
	}
	if err := s.repo.UpdateFCMToken(ctx, userID, token); err != nil {
		return err
	}
	if !s.pushDevicesEnabled {
		return nil
	}
	return s.repo.SavePushDevice(ctx, &models.PushDevice{
		UserID:         userID,
		Provider:       defaultPushValue(in.Provider, "fcm"),
		Platform:       defaultPushValue(in.Platform, "android"),
		DeviceName:     strings.TrimSpace(in.DeviceName),
		AppVersion:     strings.TrimSpace(in.AppVersion),
		Token:          token,
		LastRegistered: time.Now(),
	})
}

func defaultPushValue(value string, fallback string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

// GetFCMToken 获取用户的 FCM Token
// GetFCMToken retrieves user FCM token.
func (s *Service) GetFCMToken(ctx context.Context, userID uint64) (string, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.FCMToken, nil
}

// ResetPassword updates password hash after purpose-scoped verification succeeded.
func (s *Service) ResetPassword(ctx context.Context, userID uint64, newPassword string) error {
	if err := ValidatePasswordStrength(newPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.ResetPasswordHash(ctx, userID, string(hash))
}
