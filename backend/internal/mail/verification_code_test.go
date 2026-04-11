package mail

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/allcallall/backend/internal/models"
	"github.com/rs/zerolog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newVerificationTestEnv(t *testing.T) (*VerificationCodeService, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock failed: %v", err)
	}

	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = db.Close()
		t.Fatalf("open gorm db failed: %v", err)
	}

	mailSvc := NewService(Config{
		Host:     "127.0.0.1",
		Port:     1,
		From:     "noreply@example.com",
		FromName: "AllCallAll",
	}, zerolog.Nop())

	svc := NewVerificationCodeService(gdb, mailSvc)
	return svc, mock, func() { _ = db.Close() }
}

func TestVerificationCodeServicePaths(t *testing.T) {
	svc, mock, cleanup := newVerificationTestEnv(t)
	defer cleanup()

	if got, err := svc.generateRandomCode(6); err != nil || len(got) != 6 {
		t.Fatalf("unexpected random code result: %q err=%v", got, err)
	}

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM .*email_verification_codes.*blocked_until > .*").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	if blocked, err := svc.isEmailBlocked("alice@example.com"); err != nil || !blocked {
		t.Fatalf("expected blocked email, got blocked=%v err=%v", blocked, err)
	}

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM .*email_verification_codes.*blocked_until > .*").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("DELETE FROM .*email_verification_codes.*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO .*email_verification_codes.*").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM .*email_verification_codes.*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := svc.GenerateAndSend("alice@example.com"); err == nil {
		t.Fatal("expected send failure")
	}

	mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*email = .*").
		WillReturnError(gorm.ErrRecordNotFound)
	if err := svc.Verify("alice@example.com", "123456"); err == nil || err.Error() != "verification code not found or already used" {
		t.Fatalf("expected not found error, got %v", err)
	}

	expired := models.EmailVerificationCode{
		Email:        "alice@example.com",
		Code:         "123456",
		ExpiresAt:    time.Now().Add(-time.Minute),
		MaxAttempts:  3,
		AttemptCount: 0,
	}
	mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*email = .*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "code", "expires_at", "is_verified", "attempt_count", "max_attempts", "blocked_until", "last_attempt_at", "verified_at", "created_at", "updated_at"}).
			AddRow(1, expired.Email, expired.Code, expired.ExpiresAt, false, 0, 3, nil, nil, nil, time.Now(), time.Now()))
	if err := svc.Verify("alice@example.com", "123456"); err == nil {
		t.Fatal("expected expired code error")
	}

	mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*email = .*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "code", "expires_at", "is_verified", "attempt_count", "max_attempts", "blocked_until", "last_attempt_at", "verified_at", "created_at", "updated_at"}).
			AddRow(1, "alice@example.com", "123456", time.Now().Add(time.Minute), false, 3, 3, time.Now().Add(time.Hour), nil, nil, time.Now(), time.Now()))
	if err := svc.Verify("alice@example.com", "000000"); err == nil {
		t.Fatal("expected blocked attempt error")
	}

	mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*email = .*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "code", "expires_at", "is_verified", "attempt_count", "max_attempts", "blocked_until", "last_attempt_at", "verified_at", "created_at", "updated_at"}).
			AddRow(1, "alice@example.com", "123456", time.Now().Add(time.Minute), false, 0, 3, nil, nil, nil, time.Now(), time.Now()))
	if err := svc.Verify("alice@example.com", "000000"); err == nil {
		t.Fatal("expected incorrect code error")
	}
}
