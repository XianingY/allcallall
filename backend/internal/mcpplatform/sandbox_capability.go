package mcpplatform

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	sandboxCapabilityAudience = "allcallall-sandbox-control-plane"
	sandboxCapabilityIssuer   = "allcallall-api"
	sandboxCapabilityTTL      = 30 * time.Second
	sandboxCapabilityMaxTTL   = time.Minute
)

var ErrInvalidSandboxCapability = errors.New("invalid sandbox capability")

// SandboxCapabilityClaims binds a short-lived capability to one HTTP operation.
type SandboxCapabilityClaims struct {
	HTTPMethod    string `json:"http_method"`
	HTTPPath      string `json:"http_path"`
	RequestDigest string `json:"request_digest"`
	jwt.RegisteredClaims
}

// SandboxCapabilitySigner is held only by trusted Go API and worker processes.
type SandboxCapabilitySigner struct {
	privateKey ed25519.PrivateKey
	ttl        time.Duration
}

// SandboxCapabilityVerifier is held by the Sandbox Control Plane and contains no private key.
type SandboxCapabilityVerifier struct {
	publicKey ed25519.PublicKey
}

func NewSandboxCapabilitySigner(privateKey ed25519.PrivateKey, ttl time.Duration) (*SandboxCapabilitySigner, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid Ed25519 private key")
	}
	if ttl <= 0 || ttl > sandboxCapabilityMaxTTL {
		ttl = sandboxCapabilityTTL
	}
	return &SandboxCapabilitySigner{privateKey: privateKey, ttl: ttl}, nil
}

// SandboxCapabilitySigner returns a signer backed by the existing MCP private key.
// Its distinct audience prevents Sandbox tokens from being accepted as tool tokens.
func (m *CapabilityManager) SandboxCapabilitySigner() (*SandboxCapabilitySigner, error) {
	if m == nil {
		return nil, ErrInvalidSandboxCapability
	}
	return NewSandboxCapabilitySigner(m.privateKey, sandboxCapabilityTTL)
}

func NewSandboxCapabilityVerifier(publicKey ed25519.PublicKey) (*SandboxCapabilityVerifier, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key")
	}
	keyCopy := append(ed25519.PublicKey(nil), publicKey...)
	return &SandboxCapabilityVerifier{publicKey: keyCopy}, nil
}

func NewSandboxCapabilityVerifierFromEnv() (*SandboxCapabilityVerifier, error) {
	const name = "SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY"
	encoded := strings.TrimSpace(os.Getenv(name))
	if encoded == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	raw, err := decodeSandboxCapabilityPublicKey(encoded)
	if err != nil {
		return nil, err
	}
	return NewSandboxCapabilityVerifier(ed25519.PublicKey(raw))
}

// ValidateSandboxCapabilityPublicKey catches a mismatched deployment key before
// API and worker processes begin sending requests that the Control Plane rejects.
func (m *CapabilityManager) ValidateSandboxCapabilityPublicKey(encoded string) error {
	if m == nil || len(m.publicKey) != ed25519.PublicKeySize {
		return ErrInvalidSandboxCapability
	}
	raw, err := decodeSandboxCapabilityPublicKey(encoded)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(m.publicKey, raw) != 1 {
		return fmt.Errorf("SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY does not match MCP_CAPABILITY_ED25519_PRIVATE_KEY")
	}
	return nil
}

func decodeSandboxCapabilityPublicKey(encoded string) ([]byte, error) {
	const name = "SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY"
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%s must contain a base64-encoded 32-byte public key", name)
	}
	return raw, nil
}

func (s *SandboxCapabilitySigner) Issue(method, path, requestDigest string) (string, error) {
	if s == nil || len(s.privateKey) != ed25519.PrivateKeySize {
		return "", ErrInvalidSandboxCapability
	}
	method, path, requestDigest, err := normalizeSandboxCapabilityOperation(method, path, requestDigest)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	claims := SandboxCapabilityClaims{
		HTTPMethod:    method,
		HTTPPath:      path,
		RequestDigest: requestDigest,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sandboxCapabilityIssuer,
			Subject:   method + " " + path,
			Audience:  jwt.ClaimStrings{sandboxCapabilityAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
			NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(s.privateKey)
}

func (v *SandboxCapabilityVerifier) Verify(raw, method, path, requestDigest string) error {
	if v == nil || len(v.publicKey) != ed25519.PublicKeySize {
		return ErrInvalidSandboxCapability
	}
	method, path, requestDigest, err := normalizeSandboxCapabilityOperation(method, path, requestDigest)
	if err != nil {
		return err
	}
	claims := &SandboxCapabilityClaims{}
	token, err := jwt.ParseWithClaims(strings.TrimSpace(raw), claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
			return nil, ErrInvalidSandboxCapability
		}
		return v.publicKey, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithAudience(sandboxCapabilityAudience),
		jwt.WithIssuer(sandboxCapabilityIssuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || token == nil || !token.Valid {
		return fmt.Errorf("%w: token validation failed", ErrInvalidSandboxCapability)
	}
	if claims.ExpiresAt == nil || claims.NotBefore == nil || claims.IssuedAt == nil || strings.TrimSpace(claims.ID) == "" {
		return fmt.Errorf("%w: incomplete registered claims", ErrInvalidSandboxCapability)
	}
	validity := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if validity <= 0 || validity > sandboxCapabilityMaxTTL {
		return fmt.Errorf("%w: invalid token lifetime", ErrInvalidSandboxCapability)
	}
	if claims.HTTPMethod != method || claims.HTTPPath != path || claims.Subject != method+" "+path {
		return fmt.Errorf("%w: operation mismatch", ErrInvalidSandboxCapability)
	}
	if subtle.ConstantTimeCompare([]byte(claims.RequestDigest), []byte(requestDigest)) != 1 {
		return fmt.Errorf("%w: request digest mismatch", ErrInvalidSandboxCapability)
	}
	return nil
}

func normalizeSandboxCapabilityOperation(method, path, requestDigest string) (string, string, string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	requestDigest = strings.ToLower(strings.TrimSpace(requestDigest))
	if method == "" || path == "" || !strings.HasPrefix(path, "/") {
		return "", "", "", fmt.Errorf("%w: incomplete operation", ErrInvalidSandboxCapability)
	}
	digestBytes, err := hex.DecodeString(requestDigest)
	if err != nil || len(digestBytes) != sha256.Size {
		return "", "", "", fmt.Errorf("%w: invalid request digest", ErrInvalidSandboxCapability)
	}
	return method, path, requestDigest, nil
}

// ValidationAuthorizationRequestDigest binds authorization to the complete
// request, including the one-time secret wrapping token used by this call.
func ValidationAuthorizationRequestDigest(request ValidationRequest) (string, error) {
	request.SourceType = strings.ToLower(strings.TrimSpace(request.SourceType))
	normalizeInstallationDefinition(&request.Definition)
	return sandboxSemanticDigest(request)
}

// ExecutionAuthorizationRequestDigest is intentionally separate from the
// durable receipt digest: authorization must bind the one-time secret token.
func ExecutionAuthorizationRequestDigest(request ExecutionRequest) (string, error) {
	return sandboxSemanticDigest(normalizeExecutionRequest(request))
}

// SandboxLookupRequestDigest binds a lookup capability to one execution ID.
func SandboxLookupRequestDigest(executionID string) (string, error) {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return "", fmt.Errorf("%w: execution id is required", ErrInvalidInput)
	}
	return sandboxSemanticDigest(struct {
		ExecutionID string `json:"execution_id"`
	}{ExecutionID: executionID})
}

func sandboxSemanticDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode sandbox request digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func normalizeInstallationDefinition(definition *InstallationDefinition) {
	if definition.Command == nil {
		definition.Command = []string{}
	}
	if definition.Args == nil {
		definition.Args = []string{}
	}
	if definition.Config == nil {
		definition.Config = map[string]any{}
	}
	if definition.NetworkAllowlist == nil {
		definition.NetworkAllowlist = []string{}
	}
}

func normalizeExecutionRequest(request ExecutionRequest) ExecutionRequest {
	request.ExecutionID = strings.TrimSpace(request.ExecutionID)
	request.RunRef = strings.TrimSpace(request.RunRef)
	request.ToolCallID = strings.TrimSpace(request.ToolCallID)
	request.ToolName = strings.TrimSpace(request.ToolName)
	request.SourceType = strings.ToLower(strings.TrimSpace(request.SourceType))
	if request.TimeoutMS <= 0 || request.TimeoutMS > DefaultExecutionTimeout.Milliseconds() {
		request.TimeoutMS = DefaultExecutionTimeout.Milliseconds()
	}
	if request.OutputLimit <= 0 || request.OutputLimit > DefaultOutputLimit {
		request.OutputLimit = DefaultOutputLimit
	}
	if request.Arguments == nil {
		request.Arguments = map[string]any{}
	}
	normalizeInstallationDefinition(&request.Definition)
	return request
}

// SandboxAuthorizationToken extracts one strict Bearer token from a request.
func SandboxAuthorizationToken(request *http.Request) (string, error) {
	if request == nil {
		return "", ErrInvalidSandboxCapability
	}
	parts := strings.Fields(request.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", ErrInvalidSandboxCapability
	}
	return parts[1], nil
}
