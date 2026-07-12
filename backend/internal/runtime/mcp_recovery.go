package runtime

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/mcpplatform"
)

// StartMCPReconciliationWorker resolves durable Sandbox receipts without replaying tools.
func StartMCPReconciliationWorker(ctx context.Context, log zerolog.Logger, service *mcpplatform.Service) {
	if service == nil {
		return
	}
	intervalSeconds := intFromEnv("MCP_RECONCILE_INTERVAL_SEC", 15)
	batchSize := intFromEnv("MCP_RECONCILE_BATCH_SIZE", 100)
	interval := time.Duration(intervalSeconds) * time.Second
	log.Info().
		Int("interval_sec", intervalSeconds).
		Int("batch_size", batchSize).
		Msg("MCP execution reconciliation enabled")
	go runTicker(ctx, interval, func() {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		reconciled, err := service.ReconcilePendingExecutions(runCtx, batchSize)
		if err != nil {
			log.Error().Err(err).Msg("MCP execution reconciliation failed")
			return
		}
		if reconciled > 0 {
			log.Info().Int("executions", reconciled).Msg("MCP executions reconciled")
		}
	})
}
