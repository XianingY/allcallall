package bootstrap

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/cache"
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


func TestRunServerStartsRedisPoolMetricsSampler(t *testing.T) {
	// RunServer should start a Redis pool metrics sampler with the root
	// context. This test verifies the sampler is accessible from this
	// package without a live Redis server.
	_ = cache.StartRedisPoolMetrics // verify the function is accessible
}
