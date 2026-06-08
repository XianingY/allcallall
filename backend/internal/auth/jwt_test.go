package auth

import (
	"testing"
	"time"
)

func TestNewManagerValidationAndDefaults(t *testing.T) {
	if _, err := NewManager(Config{}); err == nil {
		t.Fatal("expected error for empty secret")
	}

	mgr, err := NewManager(Config{Secret: "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := mgr.accessTTL, time.Hour; got != want {
		t.Fatalf("unexpected default access TTL: got %v want %v", got, want)
	}
	if got, want := mgr.refreshTTL, 7*24*time.Hour; got != want {
		t.Fatalf("unexpected default refresh TTL: got %v want %v", got, want)
	}

	mgr, err = NewManager(Config{
		Secret:          "secret",
		Issuer:          "allcallall",
		AccessTokenTTL:  10 * time.Minute,
		RefreshTokenTTL: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := mgr.issuer, "allcallall"; got != want {
		t.Fatalf("unexpected issuer: got %q want %q", got, want)
	}
	if got, want := mgr.accessTTL, 10*time.Minute; got != want {
		t.Fatalf("unexpected access TTL: got %v want %v", got, want)
	}
	if got, want := mgr.refreshTTL, 2*time.Hour; got != want {
		t.Fatalf("unexpected refresh TTL: got %v want %v", got, want)
	}
}

func TestGenerateAndParseAccessToken(t *testing.T) {
	mgr, err := NewManager(Config{
		Secret:         "top-secret",
		Issuer:         "allcallall",
		AccessTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	before := time.Now()
	token, err := mgr.GenerateAccessToken(42, "user@example.com")
	if err != nil {
		t.Fatalf("generate access token failed: %v", err)
	}

	claims, err := mgr.ParseToken(token)
	if err != nil {
		t.Fatalf("parse token failed: %v", err)
	}

	if claims.UserID != 42 {
		t.Fatalf("unexpected user ID: got %d want %d", claims.UserID, 42)
	}
	if claims.TokenType != TokenTypeAccess {
		t.Fatalf("unexpected token type: got %q", claims.TokenType)
	}
	if claims.Email != "user@example.com" {
		t.Fatalf("unexpected email: got %q", claims.Email)
	}
	if claims.Issuer != "allcallall" {
		t.Fatalf("unexpected issuer: got %q", claims.Issuer)
	}
	if claims.Subject != "user@example.com" {
		t.Fatalf("unexpected subject: got %q", claims.Subject)
	}
	if claims.ExpiresAt == nil || !claims.ExpiresAt.Time.After(before) {
		t.Fatalf("expected future expiration, got %+v", claims.ExpiresAt)
	}
}

func TestGenerateAndParseRefreshToken(t *testing.T) {
	mgr, err := NewManager(Config{
		Secret:          "top-secret",
		Issuer:          "allcallall",
		RefreshTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	token, err := mgr.GenerateRefreshToken(42, "user@example.com")
	if err != nil {
		t.Fatalf("generate refresh token failed: %v", err)
	}

	claims, err := mgr.ParseRefreshToken(token)
	if err != nil {
		t.Fatalf("parse refresh token failed: %v", err)
	}
	if claims.UserID != 42 || claims.TokenType != TokenTypeRefresh {
		t.Fatalf("unexpected refresh claims: %+v", claims)
	}
	if _, err := mgr.ParseToken(token); err == nil {
		t.Fatal("expected refresh token to be rejected as access token")
	}
}

func TestParseTokenRejectsInvalidIssuerOrSecret(t *testing.T) {
	mgr, err := NewManager(Config{
		Secret: "secret-1",
		Issuer: "issuer-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token, err := mgr.GenerateAccessToken(7, "alice@example.com")
	if err != nil {
		t.Fatalf("generate access token failed: %v", err)
	}

	otherIssuerMgr, err := NewManager(Config{
		Secret: "secret-1",
		Issuer: "issuer-2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := otherIssuerMgr.ParseToken(token); err == nil {
		t.Fatal("expected issuer validation error")
	}

	otherSecretMgr, err := NewManager(Config{
		Secret: "secret-2",
		Issuer: "issuer-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := otherSecretMgr.ParseToken(token); err == nil {
		t.Fatal("expected signature validation error")
	}
}
