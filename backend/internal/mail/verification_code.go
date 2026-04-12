package mail

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

var (
	ErrEmailTemporarilyBlocked          = errors.New("email is temporarily blocked, please try again later")
	ErrVerificationCodeNotFoundOrUsed   = errors.New("verification code not found or already used")
	ErrVerificationCodeExpired          = errors.New("verification code has expired")
	ErrTooManyVerificationAttempts      = errors.New("too many attempts, please try again later")
	ErrVerificationCodeIncorrect        = errors.New("verification code is incorrect")
	ErrEmailNotVerifiedForRegistration  = errors.New("email must be verified before registration")
	ErrVerificationAlreadyConsumed      = errors.New("verified email state has already been consumed")
	ErrEmailNotVerifiedForPurpose       = errors.New("email must be verified for the requested purpose")
)

const (
	PurposeRegister       = "register"
	PurposePasswordReset  = "password_reset"
	PurposeAccountDeletion = "account_deletion"
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
	return s.GenerateAndSendForPurpose(email, PurposeRegister)
}

// GenerateAndSendForPurpose creates a verification code scoped to a specific purpose.
func (s *VerificationCodeService) GenerateAndSendForPurpose(email string, purpose string) error {
	// 1. 检查防刷限制
	email = strings.TrimSpace(strings.ToLower(email))
	purpose = normalizePurpose(purpose)
	blocked, err := s.isEmailBlocked(email, purpose)
	if err != nil {
		return err
	}
	if blocked {
		return ErrEmailTemporarilyBlocked
	}

	// 2. 生成验证码
	code, err := s.generateRandomCode(s.codeLength)
	if err != nil {
		return err
	}

	// 3. 删除旧验证码
	if err := s.db.
		Where("email = ? AND purpose = ? AND consumed_at IS NULL", email, purpose).
		Delete(&models.EmailVerificationCode{}).Error; err != nil {
		return fmt.Errorf("delete old codes: %w", err)
	}

	// 4. 创建新验证码记录
	now := time.Now()
	verification := &models.EmailVerificationCode{
		Email:        email,
		Code:         code,
		Purpose:      purpose,
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
	return s.VerifyForPurpose(email, inputCode, PurposeRegister)
}

// VerifyForPurpose checks if the provided code matches the stored code for the email+purpose pair.
func (s *VerificationCodeService) VerifyForPurpose(email, inputCode string, purpose string) error {
	var verification models.EmailVerificationCode
	email = strings.TrimSpace(strings.ToLower(email))
	purpose = normalizePurpose(purpose)

	// 查询验证码记录
	if err := s.db.
		Where("email = ? AND purpose = ? AND is_verified = ?", email, purpose, false).
		Where("consumed_at IS NULL").
		Order("created_at DESC").
		First(&verification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVerificationCodeNotFoundOrUsed
		}
		return err
	}

	// 检查过期
	if time.Now().After(verification.ExpiresAt) {
		return ErrVerificationCodeExpired
	}

	// 检查尝试次数和封禁状态
	if verification.AttemptCount >= verification.MaxAttempts {
		if verification.BlockedUntil != nil && time.Now().Before(*verification.BlockedUntil) {
			return ErrTooManyVerificationAttempts
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
		return ErrVerificationCodeIncorrect
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

// EnsureVerifiedForRegistration 检查邮箱是否存在可消费的已验证状态
// EnsureVerifiedForRegistration ensures the email has a verified state ready for registration.
func (s *VerificationCodeService) EnsureVerifiedForRegistration(email string) error {
	return s.EnsureVerifiedForPurpose(email, PurposeRegister)
}

func (s *VerificationCodeService) EnsureVerifiedForPurpose(email string, purpose string) error {
	var verification models.EmailVerificationCode
	email = strings.TrimSpace(strings.ToLower(email))
	purpose = normalizePurpose(purpose)

	if err := s.db.
		Where("email = ? AND purpose = ? AND is_verified = ? AND consumed_at IS NULL", email, purpose, true).
		Order("verified_at DESC").
		First(&verification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if purpose == PurposeRegister {
				return ErrEmailNotVerifiedForRegistration
			}
			return ErrEmailNotVerifiedForPurpose
		}
		return err
	}

	return nil
}

// ConsumeVerifiedRegistration 将已验证状态标记为已消费，避免重复注册复用
// ConsumeVerifiedRegistration marks a verified email state as consumed after registration succeeds.
func (s *VerificationCodeService) ConsumeVerifiedRegistration(email string) error {
	return s.ConsumeVerifiedPurpose(email, PurposeRegister)
}

func (s *VerificationCodeService) ConsumeVerifiedPurpose(email string, purpose string) error {
	var verification models.EmailVerificationCode
	email = strings.TrimSpace(strings.ToLower(email))
	purpose = normalizePurpose(purpose)

	if err := s.db.
		Where("email = ? AND purpose = ? AND is_verified = ? AND consumed_at IS NULL", email, purpose, true).
		Order("verified_at DESC").
		First(&verification).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVerificationAlreadyConsumed
		}
		return err
	}

	now := time.Now()
	verification.ConsumedAt = &now
	if err := s.db.Save(&verification).Error; err != nil {
		return fmt.Errorf("consume verified registration: %w", err)
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
func (s *VerificationCodeService) isEmailBlocked(email string, purpose ...string) (bool, error) {
	currentPurpose := PurposeRegister
	if len(purpose) > 0 {
		currentPurpose = normalizePurpose(purpose[0])
	}
	var count int64
	result := s.db.
		Model(&models.EmailVerificationCode{}).
		Where("email = ? AND purpose = ? AND blocked_until > ?", email, currentPurpose, time.Now()).
		Count(&count)

	return count > 0, result.Error
}

func normalizePurpose(purpose string) string {
	switch strings.TrimSpace(strings.ToLower(purpose)) {
	case PurposePasswordReset:
		return PurposePasswordReset
	case PurposeAccountDeletion:
		return PurposeAccountDeletion
	default:
		return PurposeRegister
	}
}
