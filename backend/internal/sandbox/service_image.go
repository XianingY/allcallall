package sandbox

import (
	"encoding/hex"
	"fmt"
	"strings"
)

func validateDigestPinned(imageRef string) error {
	digest := imageDigest(imageRef)
	if len(digest) != 64 {
		return fmt.Errorf("%w: image is not pinned to sha256", ErrImageRejected)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return fmt.Errorf("%w: invalid image digest", ErrImageRejected)
	}
	return nil
}

func imageDigest(imageRef string) string {
	parts := strings.Split(strings.TrimSpace(imageRef), "@sha256:")
	if len(parts) != 2 || parts[0] == "" {
		return ""
	}
	return strings.ToLower(parts[1])
}
