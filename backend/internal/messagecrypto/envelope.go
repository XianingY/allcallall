// Package messagecrypto 提供聊天正文的应用层信封加密（envelope encryption）。
//
// 威胁模型（必须明确，避免过度承诺）：
//   - 防护：数据库落盘泄露、备份文件外泄、DBA 直接查库、只读从库/慢日志泄露。
//   - 不防护：应用进程被攻破（进程持有主密钥）。因此这不是端到端加密，
//     与微信的信任模型一致——服务端具备解密能力，隐私靠「不长期留存 + 不用于挖掘 + 严格访问控制」保障。
//
// 结构：每条消息随机生成一个数据密钥（DEK），用 DEK 以 AES-256-GCM 加密正文；
// DEK 再用主密钥（KEK）加密后随消息一起落库。主密钥可由环境变量注入，
// 后续替换为 KMS 只需实现 KeyProvider 接口，无需改动调用方。
package messagecrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
)

const (
	// AlgorithmAESGCM 是当前唯一支持的算法标识。
	AlgorithmAESGCM = "AES-256-GCM"
	// envelopeVersion 用于未来算法/格式升级时的兼容分支。
	envelopeVersion = 1
	// masterKeyBytes 主密钥长度，固定 32 字节（AES-256）。
	masterKeyBytes = 32
	// dekBytes 数据密钥长度。
	dekBytes = 32
)

var (
	// ErrMasterKeyRequired 主密钥缺失或长度不合法。
	ErrMasterKeyRequired = errors.New("messagecrypto: master key must be 32 bytes")
	// ErrUnsupportedAlgorithm 元数据里的算法不被支持。
	ErrUnsupportedAlgorithm = errors.New("messagecrypto: unsupported algorithm")
	// ErrMalformedEnvelope 元数据结构损坏。
	ErrMalformedEnvelope = errors.New("messagecrypto: malformed envelope metadata")
)

// Cipher 是正文加解密的抽象，便于测试替身与未来切换 KMS。
// Cipher abstracts body encryption so KMS backends can be swapped in later.
type Cipher interface {
	// Encrypt 返回密文与随行元数据；元数据为空字符串表示未加密。
	Encrypt(plaintext string) (ciphertext string, metadata string, err error)
	// Decrypt 在元数据为空时原样返回入参，以兼容加密上线前的历史明文数据。
	Decrypt(ciphertext string, metadata string) (string, error)
	// KeyID 返回当前主密钥标识，便于轮转审计。
	KeyID() string
}

// envelopeMetadata 是随消息落库的信封元数据（JSON）。
type envelopeMetadata struct {
	Version   int    `json:"v"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	// WrappedDEK 是被主密钥加密后的数据密钥（base64，nonce 前置）。
	WrappedDEK string `json:"dek"`
}

// EnvelopeCipher 基于本地主密钥的信封加密实现。
type EnvelopeCipher struct {
	keyID string
	kek   cipher.AEAD
}

// NewEnvelopeCipher 用 32 字节主密钥构造加密器。
func NewEnvelopeCipher(masterKey []byte, keyID string) (*EnvelopeCipher, error) {
	if len(masterKey) != masterKeyBytes {
		return nil, ErrMasterKeyRequired
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(keyID) == "" {
		keyID = "local-v1"
	}
	return &EnvelopeCipher{keyID: keyID, kek: aead}, nil
}

// NewEnvelopeCipherFromBase64 接受 base64 编码的主密钥（标准或 URL 变体）。
func NewEnvelopeCipherFromBase64(encoded, keyID string) (*EnvelopeCipher, error) {
	raw, err := decodeBase64(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("messagecrypto: decode master key: %w", err)
	}
	return NewEnvelopeCipher(raw, keyID)
}

func (c *EnvelopeCipher) KeyID() string { return c.keyID }

// Encrypt 生成一次性 DEK 加密正文，并用主密钥包裹该 DEK。
func (c *EnvelopeCipher) Encrypt(plaintext string) (string, string, error) {
	if plaintext == "" {
		// 空正文（例如已删除/已撤回消息）无需加密，保持空值语义。
		return "", "", nil
	}
	dek := make([]byte, dekBytes)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return "", "", err
	}
	dataAEAD, err := newAEAD(dek)
	if err != nil {
		return "", "", err
	}
	sealed, err := seal(dataAEAD, []byte(plaintext))
	if err != nil {
		return "", "", err
	}
	wrapped, err := seal(c.kek, dek)
	if err != nil {
		return "", "", err
	}
	metadata, err := json.Marshal(envelopeMetadata{
		Version:    envelopeVersion,
		Algorithm:  AlgorithmAESGCM,
		KeyID:      c.keyID,
		WrappedDEK: base64.StdEncoding.EncodeToString(wrapped),
	})
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), string(metadata), nil
}

// Decrypt 解开信封；metadata 为空视为历史明文，原样返回。
func (c *EnvelopeCipher) Decrypt(ciphertext, metadata string) (string, error) {
	if strings.TrimSpace(metadata) == "" {
		return ciphertext, nil
	}
	if ciphertext == "" {
		return "", nil
	}
	var envelope envelopeMetadata
	if err := json.Unmarshal([]byte(metadata), &envelope); err != nil {
		return "", ErrMalformedEnvelope
	}
	if envelope.Algorithm != AlgorithmAESGCM {
		return "", ErrUnsupportedAlgorithm
	}
	wrapped, err := base64.StdEncoding.DecodeString(envelope.WrappedDEK)
	if err != nil {
		return "", ErrMalformedEnvelope
	}
	dek, err := open(c.kek, wrapped)
	if err != nil {
		return "", err
	}
	dataAEAD, err := newAEAD(dek)
	if err != nil {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", ErrMalformedEnvelope
	}
	plaintext, err := open(dataAEAD, sealed)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// NoopCipher 表示「未启用加密」，写入原文、读取原样返回。
// NoopCipher keeps plaintext behaviour when encryption is disabled.
type NoopCipher struct{}

func (NoopCipher) Encrypt(plaintext string) (string, string, error) { return plaintext, "", nil }
func (NoopCipher) Decrypt(ciphertext, metadata string) (string, error) {
	if strings.TrimSpace(metadata) != "" {
		// 已加密的历史数据在关闭加密后仍必须可读，否则会造成数据不可用。
		return "", errors.New("messagecrypto: encrypted payload requires a configured cipher")
	}
	return ciphertext, nil
}
func (NoopCipher) KeyID() string { return "" }

// defaultCipher 是进程级默认加密器，供无法注入依赖的调用方（例如 agent 上下文装载）使用。
var defaultCipher atomic.Value

// SetDefault 在进程启动时装配默认加密器。
func SetDefault(c Cipher) {
	if c == nil {
		return
	}
	defaultCipher.Store(&c)
}

// Default 返回默认加密器，未配置时退化为 NoopCipher。
func Default() Cipher {
	if stored, ok := defaultCipher.Load().(*Cipher); ok && stored != nil {
		return *stored
	}
	return NoopCipher{}
}

// DecryptWithDefault 是给非依赖注入场景的便捷函数。
func DecryptWithDefault(ciphertext, metadata string) string {
	plaintext, err := Default().Decrypt(ciphertext, metadata)
	if err != nil {
		return ""
	}
	return plaintext
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal 输出 nonce||ciphertext，避免额外字段存储 nonce。
func seal(aead cipher.AEAD, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

func open(aead cipher.AEAD, payload []byte) ([]byte, error) {
	if len(payload) < aead.NonceSize() {
		return nil, ErrMalformedEnvelope
	}
	nonce := payload[:aead.NonceSize()]
	return aead.Open(nil, nonce, payload[aead.NonceSize():], nil)
}

func decodeBase64(value string) ([]byte, error) {
	if raw, err := base64.StdEncoding.DecodeString(value); err == nil {
		return raw, nil
	}
	return base64.URLEncoding.DecodeString(value)
}
