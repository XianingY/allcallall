package mcpplatform

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const capabilityAudience = "allcallall-agent-tools"

var ErrInvalidCapability = errors.New("invalid tool capability")

type CapabilityClaims struct {
	OrganizationID uint64            `json:"organization_id"`
	UserID         uint64            `json:"user_id"`
	ConversationID uint64            `json:"conversation_id"`
	RunRef         string            `json:"run_ref"`
	Revisions      map[string]uint64 `json:"revisions"`
	Tools          []string          `json:"tools"`
	jwt.RegisteredClaims
}

type CapabilityManager struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	ttl        time.Duration
}

func NewCapabilityManager(privateKey ed25519.PrivateKey, ttl time.Duration) (*CapabilityManager, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid Ed25519 private key")
	}
	if ttl <= 0 || ttl > 5*time.Minute {
		ttl = time.Minute
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("derive Ed25519 public key")
	}
	return &CapabilityManager{privateKey: privateKey, publicKey: publicKey, ttl: ttl}, nil
}

func NewCapabilityManagerFromEnv() (*CapabilityManager, error) {
	encoded := strings.TrimSpace(os.Getenv("MCP_CAPABILITY_ED25519_PRIVATE_KEY"))
	var privateKey ed25519.PrivateKey
	if encoded == "" {
		_, generated, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate Ed25519 capability key: %w", err)
		}
		privateKey = generated
	} else {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode MCP_CAPABILITY_ED25519_PRIVATE_KEY: %w", err)
		}
		switch len(raw) {
		case ed25519.SeedSize:
			privateKey = ed25519.NewKeyFromSeed(raw)
		case ed25519.PrivateKeySize:
			privateKey = ed25519.PrivateKey(raw)
		default:
			return nil, fmt.Errorf("MCP_CAPABILITY_ED25519_PRIVATE_KEY must contain a 32-byte seed or 64-byte private key")
		}
	}
	return NewCapabilityManager(privateKey, time.Minute)
}

func (m *CapabilityManager) IssueForRun(ctx context.Context, service *Service, organizationID, userID, conversationID uint64, runRef string) (string, error) {
	tools, err := service.Catalog(ctx, organizationID, userID)
	if err != nil {
		return "", err
	}
	allowedTools := make([]string, 0, len(tools))
	revisions := make(map[string]uint64)
	for _, tool := range tools {
		allowedTools = append(allowedTools, tool.NamespacedName)
		revisions[strconv.FormatUint(tool.InstallationID, 10)] = tool.RevisionID
	}
	return m.Issue(CapabilityClaims{
		OrganizationID: organizationID,
		UserID:         userID,
		ConversationID: conversationID,
		RunRef:         strings.TrimSpace(runRef),
		Revisions:      revisions,
		Tools:          allowedTools,
	})
}

func (m *CapabilityManager) Issue(claims CapabilityClaims) (string, error) {
	if m == nil || len(m.privateKey) != ed25519.PrivateKeySize {
		return "", ErrInvalidCapability
	}
	if claims.OrganizationID == 0 || claims.UserID == 0 || claims.ConversationID == 0 || claims.RunRef == "" {
		return "", fmt.Errorf("%w: incomplete subject", ErrInvalidCapability)
	}
	now := time.Now().UTC()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    "allcallall-api",
		Subject:   claims.RunRef,
		Audience:  jwt.ClaimStrings{capabilityAudience},
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        uuid.NewString(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(m.privateKey)
}

func (m *CapabilityManager) Verify(raw string) (*CapabilityClaims, error) {
	if m == nil || len(m.publicKey) != ed25519.PublicKeySize {
		return nil, ErrInvalidCapability
	}
	claims := &CapabilityClaims{}
	token, err := jwt.ParseWithClaims(strings.TrimSpace(raw), claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
			return nil, ErrInvalidCapability
		}
		return m.publicKey, nil
	}, jwt.WithAudience(capabilityAudience), jwt.WithIssuer("allcallall-api"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCapability, err)
	}
	return claims, nil
}

func (c *CapabilityClaims) Allows(organizationID, userID, conversationID uint64, runRef, toolName string, installationID, revisionID uint64) bool {
	if c == nil || c.OrganizationID != organizationID || c.UserID != userID || c.ConversationID != conversationID || c.RunRef != runRef {
		return false
	}
	allowed := false
	for _, name := range c.Tools {
		if name == toolName {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	return c.Revisions[strconv.FormatUint(installationID, 10)] == revisionID
}

func (m *CapabilityManager) PublicKeyBase64() string {
	if m == nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(m.publicKey)
}
