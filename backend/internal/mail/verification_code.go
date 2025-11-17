package mail

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// VerificationCodeService 验证码业务逻辑
// VerificationCodeService handles email verification code operations
type VerificationCodeService struct {
	db            *gorm.DB
	mailService   *Service
	codeLength    int
	validityTime  time.Duration
	maxRetries    int
	blockDuration time.Duration
}

// NewVerificationCodeService 创建验证码服务
// NewVerificationCodeService creates a new verification code service
func NewVerificationCodeService(
	db *gorm.DB,
	mailService *Service,
) *VerificationCodeService {
	return &VerificationCodeService{
		db:            db,
		mailService:   mailService,
		codeLength:    6,
		validityTime:  10 * time.Minute,
		maxRetries:    3,
		blockDuration: 30 * time.Minute,
	}
}

// GenerateAndSend 生成并发送验证码
// GenerateAndSend creates a verification code and sends it via email
func (s *VerificationCodeService) GenerateAndSend(email string) error {
	// 1. 检查防刷限制
	blocked, err := s.isEmailBlocked(email)
	if err != nil {
		return err
	}
	if blocked {
		return errors.New("email is temporarily blocked, please try again later")
	}

	// 2. 生成验证码
	code, err := s.generateRandomCode(s.codeLength)
	if err != nil {
		return err
	}

	// 3. 删除旧验证码
	if err := s.db.
		Where("email = ? AND is_verified = ?", email, false).
		Delete(&models.EmailVerificationCode{}).Error; err != nil {
		return fmt.Errorf("delete old codes: %w", err)
	}

	// 4. 创建新验证码记录
	now := time.Now()
	verification := &models.EmailVerificationCode{
		Email:        email,
		Code:         code,
		ExpiresAt:    now.Add(s.validityTime),
		MaxAttempts:  s.maxRetries,
		AttemptCount: 0,
	}

	if err := s.db.Create(verification).Error; err != nil {
		return fmt.Errorf("create verification code: %w", err)
	}

	// 5. 发送邮件
	if err := s.mailService.SendVerificationCode(email, code); err != nil {
		// 发送失败时删除验证码记录
		s.db.Delete(verification)
		return fmt.Errorf("send verification email: %w", err)
	}

	return nil
}

// Verify 验证码校验
// Verify checks if the provided code matches the stored code for the email
func (s *VerificationCodeService) Verify(email, inputCode string) error {
	var verification models.EmailVerificationCode

	// 查询验证码记录
	if err := s.db.
		Where("email = ? AND is_verified = ?", email, false).
		First(&verification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("verification code not found or already used")
		}
		return err
	}

	// 检查过期
	if time.Now().After(verification.ExpiresAt) {
		return errors.New("verification code has expired")
	}

	// 检查尝试次数和封禁状态
	if verification.AttemptCount >= verification.MaxAttempts {
		if verification.BlockedUntil != nil && time.Now().Before(*verification.BlockedUntil) {
			return errors.New("too many attempts, please try again later")
		}
	}

	// 验证码匹配
	if verification.Code != inputCode {
		verification.AttemptCount++
		now := time.Now()
		verification.LastAttemptAt = &now

		// 超过最大尝试次数时触发封禁
		if verification.AttemptCount >= verification.MaxAttempts {
			blockedUntil := time.Now().Add(s.blockDuration)
			verification.BlockedUntil = &blockedUntil
		}

		s.db.Save(&verification)
		return errors.New("verification code is incorrect")
	}

	// 标记为已验证
	now := time.Now()
	verification.IsVerified = true
	verification.VerifiedAt = &now

	if err := s.db.Save(&verification).Error; err != nil {
		return fmt.Errorf("mark verification as verified: %w", err)
	}

	return nil
}

// 生成随机验证码
func (s *VerificationCodeService) generateRandomCode(length int) (string, error) {
	const digits = "0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := 0; i < length; i++ {
		b[i] = digits[int(b[i])%len(digits)]
	}
	return string(b), nil
}

// 检查邮箱是否被封禁
func (s *VerificationCodeService) isEmailBlocked(email string) (bool, error) {
	var count int64
	result := s.db.
		Model(&models.EmailVerificationCode{}).
		Where("email = ? AND blocked_until > ?", email, time.Now()).
		Count(&count)

	return count > 0, result.Error
}
