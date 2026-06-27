package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/trace"
)

func TestCORSMiddlewareAllowsCredentialedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware(CORSConfig{AllowedOrigins: []string{"http://localhost:8081"}}))
	router.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("Origin", "http://localhost:8081")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8081" {
		t.Fatalf("unexpected allow origin: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials header, got %q", got)
	}
}

func TestCORSMiddlewareRejectsUnknownPreflightOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware(CORSConfig{AllowedOrigins: []string{"http://localhost:8081"}}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/refresh", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow origin for denied preflight: %q", got)
	}
}

func TestCORSMiddlewareIgnoresRequestsWithoutOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware(CORSConfig{AllowedOrigins: []string{"http://localhost:8081"}}))
	router.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS headers without Origin, got %q", got)
	}
}

func TestDefaultCORSOriginsUsesExplicitEnvList(t *testing.T) {
	got := DefaultCORSOrigins("https://app.example.com, https://desktop.example.com/")
	if len(got) != 2 {
		t.Fatalf("expected 2 origins, got %d", len(got))
	}
	if got[0] != "https://app.example.com" || got[1] != "https://desktop.example.com/" {
		t.Fatalf("unexpected origins: %+v", got)
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SecurityHeadersMiddleware())
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("expected content security policy")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("unexpected content type policy: %q", got)
	}
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("expected HSTS for HTTPS request")
	}
}

func TestRequestLoggerPropagatesRequestIDToContextAndHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestLogger(ginLoggerForTest(), nil))
	router.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"request_id": trace.RequestID(c.Request.Context())})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set(requestIDHeader, "req-middleware-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get(requestIDHeader); got != "req-middleware-1" {
		t.Fatalf("unexpected request id header: %q", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "req-middleware-1") {
		t.Fatalf("response missing propagated request id: %s", body)
	}
}

func TestRequestLoggerReplacesInvalidRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestLogger(ginLoggerForTest(), nil))
	router.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"request_id": trace.RequestID(c.Request.Context())})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set(requestIDHeader, "bad\nrequest")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	got := rec.Header().Get(requestIDHeader)
	if got == "" || got == "bad\nrequest" {
		t.Fatalf("expected generated request id, got %q", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, got) {
		t.Fatalf("response missing generated request id %q: %s", got, body)
	}
}

func ginLoggerForTest() zerolog.Logger {
	return zerolog.Nop()
}
