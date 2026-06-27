package runtime

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
	case "openai_compatible", "openai":
		timeout, err := transcriptionDurationFromEnv("TRANSCRIPTION_OPENAI_TIMEOUT_SEC", 120)
		if err != nil {
			return nil, false, err
		}
		chunkDuration, err := transcriptionDurationFromEnv("TRANSCRIPTION_CHUNK_SECONDS", 600)
		if err != nil {
			return nil, false, err
		}
		maxUploadBytes, err := transcriptionInt64FromEnv("TRANSCRIPTION_MAX_UPLOAD_BYTES", 24*1024*1024)
		if err != nil {
			return nil, false, err
		}
		result, err := transcription.NewOpenAICompatibleProvider(transcription.OpenAICompatibleConfig{
			BaseURL:        os.Getenv("TRANSCRIPTION_OPENAI_BASE_URL"),
			APIKey:         os.Getenv("TRANSCRIPTION_OPENAI_API_KEY"),
			Model:          os.Getenv("TRANSCRIPTION_OPENAI_MODEL"),
			Language:       os.Getenv("TRANSCRIPTION_OPENAI_LANGUAGE"),
			Timeout:        timeout,
			ChunkDuration:  chunkDuration,
			MaxUploadBytes: maxUploadBytes,
			FFmpegPath:     os.Getenv("TRANSCRIPTION_FFMPEG_PATH"),
		})
		if err != nil {
			return nil, false, err
		}
		return result, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported transcription provider %q", provider)
	}
}

func transcriptionDurationFromEnv(key string, fallbackSeconds int64) (time.Duration, error) {
	value, err := transcriptionInt64FromEnv(key, fallbackSeconds)
	if err != nil {
		return 0, err
	}
	return time.Duration(value) * time.Second, nil
}

func transcriptionInt64FromEnv(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}
