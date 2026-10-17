package mail

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

type capturingCodeSender struct {
	email string
	code  string
	err   error
}

func (s *capturingCodeSender) SendVerificationCode(email, code string) error {
	s.email, s.code = email, code
	return s.err
}

func newVerificationSQLiteService(t *testing.T) (*VerificationCodeService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "verification-codes.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.EmailVerificationCode{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return NewVerificationCodeService(db, nil), db
}

func TestGenerateAndSendStoresOnlyCodeHash(t *testing.T) {
	svc, db := newVerificationSQLiteService(t)
	sender := &capturingCodeSender{}
	svc.mailService = sender

	if err := svc.GenerateAndSendForPurpose("alice@example.com", PurposePasswordReset); err != nil {
		t.Fatalf("generate and send failed: %v", err)
	}
	if sender.code == "" || len(sender.code) != 6 {
		t.Fatalf("expected the plaintext code to reach the mail sender, got %q", sender.code)
	}

	var row models.EmailVerificationCode
	if err := db.Where("email = ?", "alice@example.com").Take(&row).Error; err != nil {
		t.Fatalf("load verification row failed: %v", err)
	}
	if row.Code == sender.code {
		t.Fatalf("verification code must not be stored in plaintext: %q", row.Code)
	}
	if want := verificationCodeDigest(sender.code); row.Code != want {
		t.Fatalf("stored code = %q, want digest %q", row.Code, want)
	}

	// 明文只进邮件；验证端用同一个输入走哈希比较完成闭环。
	if err := svc.VerifyForPurpose("alice@example.com", sender.code, PurposePasswordReset); err != nil {
		t.Fatalf("verify with the issued code failed: %v", err)
	}
}

func TestVerifyAcceptsLegacyPlaintextRows(t *testing.T) {
	svc, db := newVerificationSQLiteService(t)
	legacy := models.EmailVerificationCode{
		Email:       "legacy@example.com",
		Code:        "123456",
		Purpose:     PurposeRegister,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		MaxAttempts: 3,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy row failed: %v", err)
	}

	// AutoMigrate 扩列后未消费的 6 位明文行仍要可验证：升级不能作废
	// 已发出的验证码。
	if err := svc.VerifyForPurpose("legacy@example.com", "123456", PurposeRegister); err != nil {
		t.Fatalf("legacy plaintext verify failed: %v", err)
	}
}

func TestVerificationCodeMatchesForms(t *testing.T) {
	digest := verificationCodeDigest("123456")
	if !verificationCodeMatches(digest, "123456") {
		t.Fatal("hashed form must match the issued code")
	}
	if verificationCodeMatches(digest, "654321") {
		t.Fatal("hashed form must reject a different code")
	}
	if !verificationCodeMatches("123456", "123456") {
		t.Fatal("legacy plaintext form must still match")
	}
	if verificationCodeMatches("123456", "000123") {
		t.Fatal("legacy plaintext form must reject a different code")
	}
}

func TestGenerateRandomCodeDigitsOnly(t *testing.T) {
	svc, _ := newVerificationSQLiteService(t)
	seen := make(map[rune]int)
	for i := 0; i < 200; i++ {
		code, err := svc.generateRandomCode(6)
		if err != nil {
			t.Fatalf("generate random code failed: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("unexpected code length: %q", code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("non-digit character in code: %q", code)
			}
			seen[r]++
		}
	}
	// 200×6=1200 个数字，10 个桶；拒绝采样下任何数字都不应完全缺席
	// （这只是防回归的粗断言，不做精确分布检验以免测试抖动）。
	if len(seen) != 10 {
		t.Fatalf("expected every digit to appear, got %d distinct digits", len(seen))
	}
}
