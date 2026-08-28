package server

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/version"
)

// serverStartTime approximates process start. The server package is imported by
// the binary at init time, so this is effectively the service start time used
// for uptime reporting on the SLA status page.
var serverStartTime = time.Now()

// ComponentStatus describes the health of a single dependency/component.
type ComponentStatus struct {
	Name      string `json:"name"`
	Healthy   bool   `json:"healthy"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

// StatusResponse is the SLA status page payload. It is intentionally
// unauthenticated (like /health and /ready) so external monitors and status
// pages can scrape it. The overall health is conveyed via the "status" field
// rather than the HTTP code, matching common status-page conventions.
type StatusResponse struct {
	Status      string            `json:"status"` // "ok" | "degraded"
	Version     version.Info      `json:"version"`
	Environment string            `json:"environment,omitempty"`
	Region      string            `json:"region,omitempty"`
	UptimeSec   int64             `json:"uptime_seconds"`
	Timestamp   time.Time         `json:"timestamp"`
	Components  []ComponentStatus `json:"components"`
	Metrics     map[string]int64  `json:"metrics,omitempty"`
}

// buildStatusResponse evaluates readiness checks and assembles the SLA payload.
func buildStatusResponse(deps RouteDependencies) StatusResponse {
	components := make([]ComponentStatus, 0, len(deps.ReadinessChecks))
	allHealthy := true
	for name, check := range deps.ReadinessChecks {
		cs := ComponentStatus{Name: name, Healthy: true}
		if check == nil {
			components = append(components, cs)
			continue
		}
		start := time.Now()
		if err := check(context.Background()); err != nil {
			cs.Healthy = false
			cs.Error = err.Error()
			allHealthy = false
		}
		cs.LatencyMs = time.Since(start).Milliseconds()
		components = append(components, cs)
	}

	resp := StatusResponse{
		Status:      "ok",
		Version:     version.Get(),
		Environment: os.Getenv("DEPLOY_ENVIRONMENT"),
		Region:      os.Getenv("DEPLOY_REGION"),
		UptimeSec:   int64(time.Since(serverStartTime).Seconds()),
		Timestamp:   time.Now().UTC(),
		Components:  components,
	}
	if !allHealthy {
		resp.Status = "degraded"
	}
	if deps.Metrics != nil {
		resp.Metrics = deps.Metrics.Snapshot()
	}
	return resp
}

// RegisterStatusRoutes adds the public SLA status page under the API group.
// It always returns HTTP 200; the body's "status" field reports overall health
// so downstream dashboards can decide alerting independently of the code.
func RegisterStatusRoutes(api *gin.RouterGroup, deps RouteDependencies) {
	api.GET("/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, buildStatusResponse(deps))
	})
}
