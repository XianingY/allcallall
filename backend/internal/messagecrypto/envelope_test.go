package messagecrypto

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func newTestCipher(t *testing.T) *EnvelopeCipher {
	t.Helper()
	key := make([]byte, masterKeyBytes)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := NewEnvelopeCipher(key, "test-key")
	if err != nil {
		t.Fatalf("new envelope cipher failed: %v", err)
	}
	return c
}

func TestEnvelopeCipherRoundTrip(t *testing.T) {
	c := newTestCipher(t)
	plaintext := "今晚八点老地方见，带上合同扫描件"

	ciphertext, metadata, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if ciphertext == plaintext {
		t.Fatal("ciphertext must differ from plaintext")
	}
	if strings.Contains(ciphertext, "老地方") {
		t.Fatal("ciphertext leaked plaintext content")
	}
	if metadata == "" {
		t.Fatal("expected envelope metadata")
	}

	decrypted, err := c.Decrypt(ciphertext, metadata)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("round trip mismatch: got %q want %q", decrypted, plaintext)
	}
}

func TestEnvelopeCipherUsesFreshDEKPerMessage(t *testing.T) {
	c := newTestCipher(t)
	_, metaA, err := c.Encrypt("same text")
	if err != nil {
		t.Fatalf("encrypt A failed: %v", err)
	}
	_, metaB, err := c.Encrypt("same text")
	if err != nil {
		t.Fatalf("encrypt B failed: %v", err)
	}
	var a, b envelopeMetadata
	if err := json.Unmarshal([]byte(metaA), &a); err != nil {
		t.Fatalf("unmarshal A failed: %v", err)
	}
	if err := json.Unmarshal([]byte(metaB), &b); err != nil {
		t.Fatalf("unmarshal B failed: %v", err)
	}
	if a.WrappedDEK == b.WrappedDEK {
		t.Fatal("每条消息必须使用独立数据密钥，避免一次泄露波及全量")
	}
	if a.Algorithm != AlgorithmAESGCM || a.Version != envelopeVersion {
		t.Fatalf("unexpected envelope header: %+v", a)
	}
}

func TestEnvelopeCipherIdenticalPlaintextProducesDifferentCiphertext(t *testing.T) {
	c := newTestCipher(t)
	first, _, err := c.Encrypt("duplicate")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	second, _, err := c.Encrypt("duplicate")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if first == second {
		t.Fatal("相同明文必须产生不同密文，否则可被频率分析")
	}
}

func TestEnvelopeCipherPassesThroughLegacyPlaintext(t *testing.T) {
	c := newTestCipher(t)
	// 加密上线前落库的历史消息没有元数据，必须原样可读。
	got, err := c.Decrypt("legacy plaintext", "")
	if err != nil {
		t.Fatalf("legacy decrypt failed: %v", err)
	}
	if got != "legacy plaintext" {
		t.Fatalf("legacy passthrough mismatch: %q", got)
	}
}

func TestEnvelopeCipherRejectsTamperedCiphertext(t *testing.T) {
	c := newTestCipher(t)
	ciphertext, metadata, err := c.Encrypt("integrity matters")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		t.Fatalf("decode ciphertext failed: %v", err)
	}
	raw[len(raw)-1] ^= 0xFF
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := c.Decrypt(tampered, metadata); err == nil {
		t.Fatal("GCM 必须检出密文篡改")
	}
}

func TestEnvelopeCipherRejectsWrongMasterKey(t *testing.T) {
	c := newTestCipher(t)
	ciphertext, metadata, err := c.Encrypt("cross-key")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	otherKey := make([]byte, masterKeyBytes)
	for i := range otherKey {
		otherKey[i] = byte(200 - i)
	}
	other, err := NewEnvelopeCipher(otherKey, "other")
	if err != nil {
		t.Fatalf("new other cipher failed: %v", err)
	}
	if _, err := other.Decrypt(ciphertext, metadata); err == nil {
		t.Fatal("换主密钥必须解密失败")
	}
}

func TestNewEnvelopeCipherRejectsShortKey(t *testing.T) {
	if _, err := NewEnvelopeCipher([]byte("too-short"), "kid"); err != ErrMasterKeyRequired {
		t.Fatalf("expected ErrMasterKeyRequired, got %v", err)
	}
}

func TestNoopCipherRefusesToSilentlyDropEncryptedData(t *testing.T) {
	if _, err := (NoopCipher{}).Decrypt("cipher", `{"v":1}`); err == nil {
		t.Fatal("关闭加密后遇到密文必须报错，而不是返回乱码")
	}
	got, err := (NoopCipher{}).Decrypt("plain", "")
	if err != nil || got != "plain" {
		t.Fatalf("noop passthrough failed: %q %v", got, err)
	}
}

func TestEmptyPlaintextSkipsEncryption(t *testing.T) {
	c := newTestCipher(t)
	ciphertext, metadata, err := c.Encrypt("")
	if err != nil {
		t.Fatalf("encrypt empty failed: %v", err)
	}
	if ciphertext != "" || metadata != "" {
		t.Fatalf("空正文不应产生信封，got ct=%q md=%q", ciphertext, metadata)
	}
}
