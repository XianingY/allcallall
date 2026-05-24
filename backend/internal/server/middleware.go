package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/metrics"
)

const requestIDHeader = "X-Request-ID"

func requestLogger(log zerolog.Logger, counters *metrics.CounterStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(requestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(requestIDHeader, requestID)
		c.Writer.Header().Set(requestIDHeader, requestID)

		start := time.Now()
		path := c.Request.URL.Path
		c.Next()

		duration := time.Since(start)
		if counters != nil {
			counters.Inc("http_requests_total")
			if c.Writer.Status() >= http.StatusInternalServerError {
				counters.Inc("http_requests_5xx_total")
			}
		}

		log.Info().
			Str("request_id", requestID).
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", c.Writer.Status()).
			Str("client_ip", c.ClientIP()).
			Dur("duration", duration).
			Msg("http_request_completed")
	}
}
