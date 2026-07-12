package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func newExecutionLeaseToken() (string, error) {
	var entropy [24]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate execution lease token: %w", err)
	}
	return "lease:" + hex.EncodeToString(entropy[:]), nil
}
