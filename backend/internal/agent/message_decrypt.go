package agent

import (
	"github.com/allcallall/backend/internal/messagecrypto"
	"github.com/allcallall/backend/internal/models"
)

// decryptMessageBodies 就地解密一批消息正文。
//
// 为什么在 agent 侧也要做：会话消息是 Agent 上下文的主要来源，而 messages.body 在库中
// 已是应用层密文。若不解密，密文会直接进入 LLM prompt——既污染推理，也可能把密钥材料
// 外传给第三方模型服务。因此这里采取 fail-closed：解密失败一律置空。
//
// decryptMessageBodies decrypts message bodies in place, failing closed on error.
func decryptMessageBodies(messages []models.Message) {
	cipher := messagecrypto.Default()
	for i := range messages {
		if messages[i].EncryptionMetadata == "" {
			continue
		}
		plaintext, err := cipher.Decrypt(messages[i].Body, messages[i].EncryptionMetadata)
		if err != nil {
			messages[i].Body = ""
			continue
		}
		messages[i].Body = plaintext
	}
}
