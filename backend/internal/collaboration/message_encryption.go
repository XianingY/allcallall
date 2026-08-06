package collaboration

import (
	"github.com/allcallall/backend/internal/messagecrypto"
	"github.com/allcallall/backend/internal/models"
)

// WithMessageCipher 注入消息正文加密器（由 runtime 依据配置装配）。
// 未注入时回落到进程级默认加密器；默认加密器未配置时等价于不加密。
// WithMessageCipher injects the body cipher; falls back to the process default.
func (s *Service) WithMessageCipher(cipher messagecrypto.Cipher) *Service {
	s.messageCipher = cipher
	return s
}

// cipher 返回当前生效的加密器，保证任何调用点都不会拿到 nil。
// cipher resolves the effective cipher, never nil.
func (s *Service) cipher() messagecrypto.Cipher {
	if s.messageCipher != nil {
		return s.messageCipher
	}
	return messagecrypto.Default()
}

// EncryptionEnabled 报告是否启用了真实加密（供透明度接口对外披露）。
// EncryptionEnabled reports whether real encryption is active.
func (s *Service) EncryptionEnabled() bool {
	return s.cipher().KeyID() != ""
}

// encryptMessageBody 在写入前加密正文。加密关闭时原样返回，元数据为空。
// encryptMessageBody encrypts a body before persisting it.
func (s *Service) encryptMessageBody(plaintext string) (string, string, error) {
	return s.cipher().Encrypt(plaintext)
}

// decryptMessageBody 在读取后解密正文。
// 解密失败不返回错误而是返回空串并告警：单条消息密钥损坏不应导致整个会话拉取失败。
// decryptMessageBody decrypts a body after loading; a single bad row must not break the page.
func (s *Service) decryptMessageBody(ciphertext, metadata string) string {
	plaintext, err := s.cipher().Decrypt(ciphertext, metadata)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to decrypt message body")
		s.metrics.Inc("message_decrypt_fail_total")
		return ""
	}
	return plaintext
}

// decryptMessageInPlace 就地解密一个 models.Message 的正文。
// decryptMessageInPlace decrypts a models.Message body in place.
func (s *Service) decryptMessageInPlace(message *models.Message) {
	if message == nil || message.EncryptionMetadata == "" {
		return
	}
	message.Body = s.decryptMessageBody(message.Body, message.EncryptionMetadata)
}
