package bootstrap

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/config"
)

func TestRunServerStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RunServer(ctx, &config.Config{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("RunServer() error = %v, want nil", err)
	}
}
