package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT 声明
// Claims extends RegisteredClaims with user information.
type Claims struct {
	UserID    uint64 `json:"user_id"`
	Email     string `json:"email"`
	TokenType string `json:"token_type,omitempty"`
	jwt.RegisteredClaims
}

// Manager JWT 管理器
// Manager issues and validates JWT tokens.
type Manager struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// Config JWT 配置
// Config holds JWT related configuration.
type Config struct {
	Secret          string
	Issuer          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// NewManager 创建管理器
// NewManager instantiates a Manager from configuration.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Secret == "" {
		return nil, errors.New("jwt secret must not be empty")
	}
	if cfg.AccessTokenTTL <= 0 {
		cfg.AccessTokenTTL = time.Hour
	}
	if cfg.RefreshTokenTTL <= 0 {
		cfg.RefreshTokenTTL = 24 * time.Hour * 7
	}
	return &Manager{
		secret:     []byte(cfg.Secret),
		issuer:     cfg.Issuer,
		accessTTL:  cfg.AccessTokenTTL,
		refreshTTL: cfg.RefreshTokenTTL,
	}, nil
}

// GenerateAccessToken 生成访问令牌
// GenerateAccessToken issues a signed JWT for the user.
func (m *Manager) GenerateAccessToken(userID uint64, email string) (string, error) {
	return m.generateToken(userID, email, TokenTypeAccess, m.accessTTL)
}

// GenerateRefreshToken issues a signed JWT intended only for refresh-cookie exchange.
func (m *Manager) GenerateRefreshToken(userID uint64, email string) (string, error) {
	return m.generateToken(userID, email, TokenTypeRefresh, m.refreshTTL)
}

func (m *Manager) generateToken(userID uint64, email string, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Email:     email,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Subject:   email,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ParseToken 解析令牌
// ParseToken validates JWT and returns claims.
func (m *Manager) ParseToken(tokenString string) (*Claims, error) {
	claims, err := m.parseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != "" && claims.TokenType != TokenTypeAccess {
		return nil, errors.New("invalid access token type")
	}
	return claims, nil
}

// ParseRefreshToken validates refresh-cookie JWTs only.
func (m *Manager) ParseRefreshToken(tokenString string) (*Claims, error) {
	claims, err := m.parseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeRefresh {
		return nil, errors.New("invalid refresh token type")
	}
	return claims, nil
}

func (m *Manager) RefreshTokenTTL() time.Duration {
	return m.refreshTTL
}

func (m *Manager) parseToken(tokenString string) (*Claims, error) {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
	}
	if m.issuer != "" {
		opts = append(opts, jwt.WithIssuer(m.issuer))
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return m.secret, nil
	}, opts...)

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token claims")
}
