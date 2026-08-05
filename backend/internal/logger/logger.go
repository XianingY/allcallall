package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// New 创建新的 Zerolog 日志记录器
// New returns a zerolog.Logger configured with given level.
func New(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	var output io.Writer = os.Stdout

	// Use pretty-printing console writer only in non-release mode for development convenience.
	// In production (GIN_MODE=release), it defaults to structured JSON for performance and observability.
	if os.Getenv("GIN_MODE") != "release" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	logger := zerolog.New(output).With().Timestamp().Logger().Level(lvl)
	return logger
}
