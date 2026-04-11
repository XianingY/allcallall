package mail

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestServiceFailurePaths(t *testing.T) {
	svc := NewService(Config{
		Host:     "127.0.0.1",
		Port:     1,
		From:     "noreply@example.com",
		FromName: "AllCallAll",
	}, zerolog.Nop())

	if err := svc.SendVerificationCode("alice@example.com", "123456"); err == nil {
		t.Fatal("expected send error")
	}

	if err := svc.HealthCheck(); err == nil {
		t.Fatal("expected health check error")
	}
}

func TestNewService(t *testing.T) {
	svc := NewService(Config{Host: "localhost"}, zerolog.Nop())
	if svc == nil {
		t.Fatal("expected service")
	}
}

func TestSendInternalErrorPath(t *testing.T) {
	svc := NewService(Config{
		Host:     "127.0.0.1",
		Port:     1,
		From:     "noreply@example.com",
		FromName: "AllCallAll",
	}, zerolog.Nop())

	if err := svc.send("alice@example.com", "subject", "body"); err == nil {
		t.Fatal("expected send error")
	}
}
