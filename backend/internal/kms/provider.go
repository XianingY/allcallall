// Package kms provides a pluggable master-key (KEK) provider for the existing
// envelope encryption in internal/messagecrypto.
//
// The original implementation read the master key from an environment variable.
// This package generalizes that into a MasterKeyProvider interface so the key
// can later be sourced from AWS KMS / GCP KMS / Alibaba KMS without touching any
// call site: swap the provider, keep NewCipher. Rotation is supported via the
// RotatingProvider cache.
package kms

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/allcallall/backend/internal/messagecrypto"
)

// MasterKeyProvider yields the plaintext master key for a given key id.
// Implementations must return a 32-byte (AES-256) key.
type MasterKeyProvider interface {
	GetMasterKey(ctx context.Context, keyID string) ([]byte, error)
}

// StaticProvider reads the master key from an environment variable (base64).
// This is the production default and keeps secrets out of code/config files.
type StaticProvider struct {
	// EnvVar is the environment variable holding the base64 master key.
	// Defaults to MESSAGE_ENCRYPTION_MASTER_KEY.
	EnvVar string
	// Value, when non-empty, is used directly instead of reading EnvVar. It lets
	// callers that already resolved the key (e.g. from the config layer) reuse
	// this provider's decoding and validation instead of duplicating it.
	Value string
}

// GetMasterKey implements MasterKeyProvider.
func (p StaticProvider) GetMasterKey(_ context.Context, _ string) ([]byte, error) {
	raw := strings.TrimSpace(p.Value)
	source := "value"
	if raw == "" {
		env := p.EnvVar
		if env == "" {
			env = "MESSAGE_ENCRYPTION_MASTER_KEY"
		}
		source = env
		raw = strings.TrimSpace(os.Getenv(env))
	}
	if raw == "" {
		return nil, fmt.Errorf("kms: %s is not set", source)
	}
	key, err := decodeBase64(raw)
	if err != nil {
		return nil, fmt.Errorf("kms: decode master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("kms: master key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

// CloudKMSAdapter adapts an external KMS decrypt call into MasterKeyProvider.
// The concrete cloud SDK call is injected (Decrypt) so this package stays free
// of heavyweight cloud dependencies. The injected function should call the
// cloud KMS Decrypt API and return the plaintext DEK-wrapping key.
type CloudKMSAdapter struct {
	Decrypt func(ctx context.Context, keyID string) ([]byte, error)
}

// GetMasterKey implements MasterKeyProvider.
func (a CloudKMSAdapter) GetMasterKey(ctx context.Context, keyID string) ([]byte, error) {
	if a.Decrypt == nil {
		return nil, errors.New("kms: CloudKMSAdapter.Decrypt is nil")
	}
	key, err := a.Decrypt(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("kms: cloud decrypt: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("kms: cloud master key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

type cacheEntry struct {
	key []byte
	exp time.Time
}

// RotatingProvider caches keys for TTL and refreshes them via Fetcher. It is
// the bridge to cloud KMS: point Fetcher at your cloud client and keys rotate
// transparently without process restarts.
type RotatingProvider struct {
	Fetcher func(ctx context.Context, keyID string) ([]byte, error)
	TTL     time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// GetMasterKey implements MasterKeyProvider with TTL caching.
func (p *RotatingProvider) GetMasterKey(ctx context.Context, keyID string) ([]byte, error) {
	if p.Fetcher == nil {
		return nil, errors.New("kms: RotatingProvider.Fetcher is nil")
	}
	if p.TTL <= 0 {
		p.TTL = time.Hour
	}
	now := time.Now().UTC()

	p.mu.Lock()
	if p.cache == nil {
		p.cache = make(map[string]cacheEntry)
	}
	if e, ok := p.cache[keyID]; ok && now.Before(e.exp) {
		cp := append([]byte(nil), e.key...)
		p.mu.Unlock()
		return cp, nil
	}
	p.mu.Unlock()

	key, err := p.Fetcher(ctx, keyID)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("kms: fetched master key must be 32 bytes, got %d", len(key))
	}

	p.mu.Lock()
	p.cache[keyID] = cacheEntry{key: append([]byte(nil), key...), exp: now.Add(p.TTL)}
	p.mu.Unlock()
	return append([]byte(nil), key...), nil
}

// NewCipher builds a messagecrypto.EnvelopeCipher from a provider and key id.
func NewCipher(ctx context.Context, p MasterKeyProvider, keyID string) (messagecrypto.Cipher, error) {
	if p == nil {
		return nil, errors.New("kms: provider is nil")
	}
	key, err := p.GetMasterKey(ctx, keyID)
	if err != nil {
		return nil, err
	}
	return messagecrypto.NewEnvelopeCipher(key, keyID)
}

// ResolveFromEnv selects a provider: cloud when KMS_PROVIDER=cloud (and a
// fetcher is injected via WithCloudFetcher), otherwise the env StaticProvider.
func ResolveFromEnv(opts ...Option) MasterKeyProvider {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.cloudFetcher != nil {
		return &RotatingProvider{Fetcher: cfg.cloudFetcher, TTL: cfg.ttl}
	}
	return StaticProvider{EnvVar: cfg.envVar}
}

type config struct {
	envVar       string
	cloudFetcher func(ctx context.Context, keyID string) ([]byte, error)
	ttl          time.Duration
}

// Option configures ResolveFromEnv.
type Option func(*config)

// WithEnvVar overrides the master-key environment variable.
func WithEnvVar(v string) Option { return func(c *config) { c.envVar = v } }

// WithCloudFetcher enables the rotating cloud-KMS provider.
func WithCloudFetcher(f func(ctx context.Context, keyID string) ([]byte, error), ttl time.Duration) Option {
	return func(c *config) { c.cloudFetcher = f; c.ttl = ttl }
}

func decodeBase64(value string) ([]byte, error) {
	if raw, err := base64.StdEncoding.DecodeString(value); err == nil {
		return raw, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

// KeyIDHash returns a stable, non-reversible reference to a key id for logging.
func KeyIDHash(keyID string) string {
	sum := sha256.Sum256([]byte(keyID))
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}
