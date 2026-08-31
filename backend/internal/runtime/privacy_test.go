package runtime

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/kms"
	"github.com/allcallall/backend/internal/messagecrypto"
)

func TestMessageRetentionPolicyFromConfigMapsHours(t *testing.T) {
	cfg := &config.Config{}
	cfg.Privacy.MessageRetention.Enabled = true
	cfg.Privacy.MessageRetention.TextTTLHours = 72
	cfg.Privacy.MessageRetention.MediaTTLHours = 120
	cfg.Privacy.MessageRetention.PurgeSystemMessages = true

	policy := MessageRetentionPolicyFromConfig(cfg)
	if !policy.Enabled {
		t.Fatal("policy should be enabled")
	}
	if policy.TextTTL != 72*time.Hour {
		t.Fatalf("text ttl=%s want=72h", policy.TextTTL)
	}
	if policy.MediaTTL != 120*time.Hour {
		t.Fatalf("media ttl=%s want=120h", policy.MediaTTL)
	}
	if !policy.PurgeSystemMessages {
		t.Fatal("system message purge flag lost in translation")
	}
}

func TestMessageRetentionPolicyFromConfigHandlesNil(t *testing.T) {
	policy := MessageRetentionPolicyFromConfig(nil)
	if policy.Enabled {
		t.Fatal("nil config must not enable retention purging")
	}
}

func TestMessageCipherFromConfigDisabledReturnsNoop(t *testing.T) {
	cfg := &config.Config{}
	cfg.Privacy.Encryption.Enabled = false

	cipher, err := MessageCipherFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("disabled encryption should not error: %v", err)
	}
	if _, ok := cipher.(messagecrypto.NoopCipher); !ok {
		t.Fatalf("cipher=%T want NoopCipher", cipher)
	}
}

func TestMessageCipherFromConfigRejectsInvalidKey(t *testing.T) {
	// 关键回归点：开启加密但密钥非法时必须报错。
	// 一旦这里静默返回 NoopCipher，运维会在「以为加密了」的状态下裸奔。
	cases := map[string]string{
		"empty key":      "",
		"not base64":     "!!!not-base64!!!",
		"key too short":  base64.StdEncoding.EncodeToString([]byte("short")),
		"wrong key size": base64.StdEncoding.EncodeToString(make([]byte, 16)),
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Privacy.Encryption.Enabled = true
			cfg.Privacy.Encryption.MasterKeyBase64 = key
			cfg.Privacy.Encryption.KeyID = "primary"

			cipher, err := MessageCipherFromConfig(context.Background(), cfg)
			if err == nil {
				t.Fatalf("expected error for %s, got cipher=%T", name, cipher)
			}
			if cipher != nil {
				t.Fatalf("cipher must be nil on failure, got %T", cipher)
			}
			if !strings.Contains(err.Error(), "message encryption") {
				t.Fatalf("error should be attributable to encryption wiring: %v", err)
			}
		})
	}
}

func TestApplyPrivacyPoliciesFailsClosedOnBadKey(t *testing.T) {
	previous := messagecrypto.Default()
	t.Cleanup(func() { messagecrypto.SetDefault(previous) })

	cfg := &config.Config{}
	cfg.Privacy.Encryption.Enabled = true
	cfg.Privacy.Encryption.MasterKeyBase64 = "definitely-not-a-valid-key"

	if err := ApplyPrivacyPolicies(context.Background(), cfg, nil); err == nil {
		t.Fatal("ApplyPrivacyPolicies must fail when the master key is unusable")
	}
}

// 关键回归点：主密钥必须经 KMS provider 解析，而非绕过 KMS 直接读配置。
// 此前 MessageCipherFromConfig 直接读 cfg.MasterKeyBase64，导致 internal/kms
// 的云 KMS 适配与轮转能力完全不生效。
func TestMessageCipherFromConfigUsesKMSProvider(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate master key: %v", err)
	}

	cfg := &config.Config{}
	cfg.Privacy.Encryption.Enabled = true
	// 故意不设置 MasterKeyBase64：主密钥只能来自注入的 KMS provider。
	cfg.Privacy.Encryption.KeyID = "cloud-key-1"

	var gotKeyID string
	calls := 0
	fetcher := func(_ context.Context, keyID string) ([]byte, error) {
		calls++
		gotKeyID = keyID
		return key, nil
	}

	cipher, err := MessageCipherFromConfig(context.Background(), cfg, kms.WithCloudFetcher(fetcher, time.Minute))
	if err != nil {
		t.Fatalf("cipher via cloud KMS: %v", err)
	}
	if calls == 0 {
		t.Fatal("expected the KMS fetcher to be invoked")
	}
	if gotKeyID != "cloud-key-1" {
		t.Fatalf("fetcher received key id=%q want=cloud-key-1", gotKeyID)
	}
	if cipher.KeyID() != "cloud-key-1" {
		t.Fatalf("cipher key id=%q want=cloud-key-1", cipher.KeyID())
	}

	ciphertext, metadata, err := cipher.Encrypt("云 KMS 回归正文")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	plaintext, err := cipher.Decrypt(ciphertext, metadata)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plaintext != "云 KMS 回归正文" {
		t.Fatalf("round trip=%q want=云 KMS 回归正文", plaintext)
	}
}

func TestApplyPrivacyPoliciesInstallsProcessDefaultCipher(t *testing.T) {
	previous := messagecrypto.Default()
	t.Cleanup(func() { messagecrypto.SetDefault(previous) })

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate master key: %v", err)
	}
	cfg := &config.Config{}
	cfg.Privacy.Encryption.Enabled = true
	cfg.Privacy.Encryption.MasterKeyBase64 = base64.StdEncoding.EncodeToString(key)
	cfg.Privacy.Encryption.KeyID = "primary-2026"

	// svc 传 nil：进程级默认加密器仍必须装配，
	// 否则 agent 侧上下文装载会读到密文当明文用。
	if err := ApplyPrivacyPolicies(context.Background(), cfg, nil); err != nil {
		t.Fatalf("ApplyPrivacyPolicies: %v", err)
	}
	installed := messagecrypto.Default()
	if installed.KeyID() != "primary-2026" {
		t.Fatalf("default cipher key id=%q want=primary-2026", installed.KeyID())
	}
	ciphertext, metadata, err := installed.Encrypt("回归测试正文")
	if err != nil {
		t.Fatalf("encrypt through default cipher: %v", err)
	}
	if ciphertext == "回归测试正文" || metadata == "" {
		t.Fatalf("default cipher did not actually encrypt: ciphertext=%q metadata=%q", ciphertext, metadata)
	}
	// DecryptWithDefault 是 fail-closed 的：失败返回空串而非乱码/密文。
	plaintext := messagecrypto.DecryptWithDefault(ciphertext, metadata)
	if plaintext != "回归测试正文" {
		t.Fatalf("round trip=%q want=回归测试正文", plaintext)
	}
}
