package logger

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewLogger(t *testing.T) {
	// Test default behavior (development mode)
	os.Setenv("GIN_MODE", "")
	logger := New("info")
	if logger.GetLevel() != zerolog.InfoLevel {
		t.Errorf("Expected info level, got %v", logger.GetLevel())
	}

	// Test release mode (JSON output)
	os.Setenv("GIN_MODE", "release")
	prodLogger := New("debug")
	if prodLogger.GetLevel() != zerolog.DebugLevel {
		t.Errorf("Expected debug level, got %v", prodLogger.GetLevel())
	}

	// Unset to not affect other tests
	os.Unsetenv("GIN_MODE")
}
