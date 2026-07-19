package mcpplatform

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSandboxCapabilityBindsOperationAndDigest(t *testing.T) {
	_, signer, verifier := sandboxCapabilityTestPair(t)
	digest, err := SandboxLookupRequestDigest("mcp:receipt-1")
	if err != nil {
		t.Fatal(err)
	}
	token, err := signer.Issue(http.MethodGet, "/internal/v1/executions/mcp:receipt-1", digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(token, http.MethodGet, "/internal/v1/executions/mcp:receipt-1", digest); err != nil {
		t.Fatalf("verify matching capability: %v", err)
	}

	otherDigest, err := SandboxLookupRequestDigest("mcp:receipt-2")
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name, method, path, digest string
	}{
		{name: "method", method: http.MethodPost, path: "/internal/v1/executions/mcp:receipt-1", digest: digest},
		{name: "path", method: http.MethodGet, path: "/internal/v1/executions/mcp:receipt-2", digest: digest},
		{name: "digest", method: http.MethodGet, path: "/internal/v1/executions/mcp:receipt-1", digest: otherDigest},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := verifier.Verify(token, check.method, check.path, check.digest); !errors.Is(err, ErrInvalidSandboxCapability) {
				t.Fatalf("expected operation-bound rejection, got %v", err)
			}
		})
	}
}

func TestSandboxCapabilityRejectsExpiredToken(t *testing.T) {
	privateKey, _, verifier := sandboxCapabilityTestPair(t)
	digest, err := SandboxLookupRequestDigest("mcp:expired")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claims := SandboxCapabilityClaims{
		HTTPMethod:    http.MethodGet,
		HTTPPath:      "/internal/v1/executions/mcp:expired",
		RequestDigest: digest,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sandboxCapabilityIssuer,
			Subject:   http.MethodGet + " /internal/v1/executions/mcp:expired",
			Audience:  jwt.ClaimStrings{sandboxCapabilityAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Minute)),
			ID:        "expired-token",
		},
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(raw, claims.HTTPMethod, claims.HTTPPath, digest); !errors.Is(err, ErrInvalidSandboxCapability) {
		t.Fatalf("expected expired token rejection, got %v", err)
	}
}

func TestSandboxCapabilityAudienceIsDistinctFromToolCapability(t *testing.T) {
	privateKey, signer, verifier := sandboxCapabilityTestPair(t)
	toolManager, err := NewCapabilityManager(privateKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := SandboxLookupRequestDigest("mcp:audience")
	if err != nil {
		t.Fatal(err)
	}
	sandboxToken, err := signer.Issue(http.MethodGet, "/internal/v1/executions/mcp:audience", digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := toolManager.Verify(sandboxToken); !errors.Is(err, ErrInvalidCapability) {
		t.Fatalf("tool verifier accepted sandbox audience: %v", err)
	}

	toolToken, err := toolManager.Issue(CapabilityClaims{
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunRef:         "agent:99",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(toolToken, http.MethodGet, "/internal/v1/executions/mcp:audience", digest); !errors.Is(err, ErrInvalidSandboxCapability) {
		t.Fatalf("sandbox verifier accepted tool audience: %v", err)
	}
}

func TestSandboxCapabilityPublicKeyConfigurationIsStrictAndMatches(t *testing.T) {
	privateKey, _, _ := sandboxCapabilityTestPair(t)
	manager, err := NewCapabilityManager(privateKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	encoded := base64.StdEncoding.EncodeToString(publicKey)
	if err := manager.ValidateSandboxCapabilityPublicKey(encoded); err != nil {
		t.Fatalf("validate matching public key: %v", err)
	}

	_, otherPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPublic := base64.StdEncoding.EncodeToString(otherPrivateKey.Public().(ed25519.PublicKey))
	if err := manager.ValidateSandboxCapabilityPublicKey(otherPublic); err == nil {
		t.Fatal("expected mismatched public key rejection")
	}
	if err := manager.ValidateSandboxCapabilityPublicKey(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("expected non-32-byte public key rejection")
	}
}

func sandboxCapabilityTestPair(t *testing.T) (ed25519.PrivateKey, *SandboxCapabilitySigner, *SandboxCapabilityVerifier) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSandboxCapabilitySigner(privateKey, sandboxCapabilityTTL)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewSandboxCapabilityVerifier(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	return privateKey, signer, verifier
}
