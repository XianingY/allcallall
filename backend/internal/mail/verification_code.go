package mail

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

var (
	ErrEmailTemporarilyBlocked         = errors.New("email is temporarily blocked, please try again later")
	ErrVerificationCodeNotFoundOrUsed  = errors.New("verification code not found or already used")
	ErrVerificationCodeExpired         = errors.New("verification code has expired")
	ErrTooManyVerificationAttempts     = errors.New("too many attempts, please try again later")
	ErrVerificationCodeIncorrect       = errors.New("verification code is incorrect")
	ErrEmailNotVerifiedForRegistration = errors.New("email must be verified before registration")
	ErrVerificationAlreadyConsumed     = errors.New("verified email state has already been consumed")
	ErrEmailNotVerifiedForPurpose      = errors.New("email must be verified for the requested purpose")
)

const (
	PurposeRegister        = "register"
	PurposePasswordReset   = "password_reset"
	PurposeAccountDeletion = "account_deletion"
)

// VerificationCodeService 验证码业务逻辑
// VerificationCodeService handles email verification code operations
type VerificationCodeService struct {
	db            *gorm.DB
	mailService   verificationCodeSender
	codeLength    int
	validityTime  time.Duration
	maxRetries    int
	blockDuration time.Duration
}

// verificationCodeSender 是验证码发送的最小接口。生产实现是 *Service；
// 同包测试注入捕获明文的 stub，验证 DB 中只落哈希。
type verificationCodeSender interface {
	SendVerificationCode(email, code string) error
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

	// 4. 创建新验证码记录（DB 只落 SHA-256，明文只进邮件正文）
	now := time.Now()
	verification := &models.EmailVerificationCode{
		Email:        email,
		Code:         verificationCodeDigest(code),
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

	// 验证码匹配：恒时比较，泄露前缀进度的短路比较不可接受。
	if !verificationCodeMatches(verification.Code, inputCode) {
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
	for i := 0; i < length; i++ {
		// 256 % 10 = 6：直接取模会让 0-5 比 6-9 更常出现。拒绝采样
		// 消除分布偏置——验证码是安全凭据，随机性不应有可预测偏差。
		for {
			if _, err := rand.Read(b[i : i+1]); err != nil {
				return "", err
			}
			if int(b[i]) < 250 {
				b[i] = digits[int(b[i])%len(digits)]
				break
			}
		}
	}
	return string(b), nil
}

// verificationCodeDigest 返回验证码的 SHA-256 hex。
func verificationCodeDigest(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// verificationCodeMatches 恒时比较输入验证码与存储值。
//
// 存储值有两种形态：64 字符 hex（新写入）与 6 位明文（历史行，
// AutoMigrate 扩列后仍留在表里）。旧行做兼容明文比较，避免部署窗口
// 内已发出的验证码被升级作废；长度是唯一区分依据。
func verificationCodeMatches(stored, input string) bool {
	if len(stored) == sha256.Size*2 {
		return subtle.ConstantTimeCompare([]byte(stored), []byte(verificationCodeDigest(input))) == 1
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(input)) == 1
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
