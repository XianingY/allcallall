package user

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestAuthenticateEqualizesUnknownEmailTiming(t *testing.T) {
	svc, mock := newMockService(t)

	// 邮箱不存在时也必须执行一次 bcrypt，否则攻击者可通过响应时间
	// 差异枚举已注册邮箱。用计数桩锁定该行为，而不是断言墙钟时间。
	calls := 0
	original := bcryptCompareHashAndPassword
	bcryptCompareHashAndPassword = func(storedHash, password []byte) error {
		calls++
		if len(storedHash) != len(dummyPasswordHash) {
			t.Fatalf("dummy hash length = %d, want %d", len(storedHash), len(dummyPasswordHash))
		}
		_ = password
		return errors.New("stop: dummy compare result is discarded")
	}
	t.Cleanup(func() { bcryptCompareHashAndPassword = original })

	mock.ExpectQuery("SELECT .*FROM `users`.*LOWER\\(email\\) = .*").
		WillReturnError(gorm.ErrRecordNotFound)

	if _, err := svc.Authenticate(context.Background(), LoginInput{Email: "ghost@example.com", Password: "whatever1"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("unknown-email path must run the bcrypt workload exactly once, got %d calls", calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
