package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/metrics"
	appserver "github.com/allcallall/backend/internal/server"
	"github.com/allcallall/backend/internal/signaling"
)

func serveUntilShutdown(
	ctx context.Context,
	httpServer *http.Server,
	metricsServer *http.Server,
	signalingHub *signaling.Hub,
	log zerolog.Logger,
) error {
	serveErr := make(chan error, 1)
	go func() {
		log.Info().Str("addr", httpServer.Addr).Msg("http server starting")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("http server failed: %w", err)
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace())
			defer cancel()
			metrics.Shutdown(shutdownCtx, metricsServer, log)
			return err
		}
		return nil
	}

	// Fail readiness before closing listeners so load balancers can move traffic.
	appserver.BeginDrain()
	if delay := shutdownDrainDelay(); delay > 0 {
		log.Info().Dur("delay", delay).Msg("draining: waiting for traffic to move to another replica")
		time.Sleep(delay)
	}

	// WebSocket connections are hijacked and must be closed explicitly.
	if n := signalingHub.Drain("server shutting down"); n > 0 {
		log.Info().Int("connections", n).Msg("realtime connections drained")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace())
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("http server shutdown error")
	} else {
		log.Info().Msg("http server gracefully stopped")
	}
	metrics.Shutdown(shutdownCtx, metricsServer, log)
	return nil
}

// shutdownDrainDelay is how long to wait after failing readiness before closing
// connections, giving load balancers time to stop sending work here.
func shutdownDrainDelay() time.Duration {
	raw := strings.TrimSpace(os.Getenv("SHUTDOWN_DRAIN_DELAY"))
	if raw == "" {
		return 5 * time.Second
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed < 0 {
		return 5 * time.Second
	}
	return parsed
}

// shutdownGrace is the HTTP shutdown budget. Keep it aligned with the chart's
// terminationGracePeriodSeconds setting.
func shutdownGrace() time.Duration {
	raw := strings.TrimSpace(os.Getenv("SHUTDOWN_GRACE_PERIOD"))
	if raw == "" {
		return 20 * time.Second
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return 20 * time.Second
	}
	return parsed
}
