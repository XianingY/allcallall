package mcpplatform

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// ExecutionRequestDigest binds a durable receipt to the semantic request.
// One-time secret wrapping tokens are transport credentials, not semantics.
func ExecutionRequestDigest(request ExecutionRequest) (string, error) {
	request = normalizeExecutionRequest(request)
	request.SecretWrapToken = ""
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode sandbox execution fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest), nil
}
