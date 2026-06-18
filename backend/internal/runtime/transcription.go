package runtime

import (
	"fmt"
	"os"
	"strings"

	"github.com/allcallall/backend/internal/transcription"
)

func TranscriptionProviderFromEnv() (transcription.Provider, bool, error) {
	enabled := strings.ToLower(strings.TrimSpace(os.Getenv("TRANSCRIPTION_ENABLED")))
	if enabled != "1" && enabled != "true" && enabled != "yes" {
		return nil, false, nil
	}
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("TRANSCRIPTION_PROVIDER")))
	if provider == "" {
		provider = "mock"
	}
	switch provider {
	case "mock":
		return transcription.NewMockProvider(), true, nil
	default:
		return nil, false, fmt.Errorf("unsupported transcription provider %q", provider)
	}
}
