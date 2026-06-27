package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/trace"
)

const requestIDHeader = "X-Request-ID"

type CORSConfig struct {
	AllowedOrigins []string
}

func DefaultCORSOrigins(value string) []string {
	if strings.TrimSpace(value) != "" {
		return splitCSV(value)
	}
	return []string{
		"http://localhost:8081",
		"http://127.0.0.1:8081",
		"http://localhost:19006",
		"http://127.0.0.1:19006",
		"http://localhost:3000",
		"http://127.0.0.1:3000",
	}
}

func CORSMiddleware(cfg CORSConfig) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(strings.TrimSpace(c.GetHeader("Origin")), "/")
		if origin == "" {
			c.Next()
			return
		}
		if _, ok := allowed[origin]; !ok {
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}
		headers := c.Writer.Header()
		headers.Set("Access-Control-Allow-Origin", origin)
		headers.Set("Access-Control-Allow-Credentials", "true")
		headers.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		headers.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,Accept,X-Organization-ID,X-Request-ID,X-Support-Token")
		headers.Set("Access-Control-Expose-Headers", "X-Request-ID,Location")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := c.Writer.Header()
		headers.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' https: wss:; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("X-Frame-Options", "DENY")
		if c.Request.TLS != nil || strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https") {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

func requestLogger(log zerolog.Logger, counters *metrics.CounterStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := trace.EnsureRequestID(c.GetHeader(requestIDHeader))
		c.Set(requestIDHeader, requestID)
		c.Writer.Header().Set(requestIDHeader, requestID)
		c.Request = c.Request.WithContext(trace.WithRequestID(c.Request.Context(), requestID))

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

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
