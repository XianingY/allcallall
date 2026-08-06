package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/trace"
)

func TestRequireTLSRejectsPlaintextWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTLS(true))
	router.GET("/api/v1/secret", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for plaintext request, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "HTTPS_REQUIRED") {
		t.Fatalf("expected HTTPS_REQUIRED error code, got %q", rec.Body.String())
	}
}

func TestRequireTLSAcceptsDirectTLSAndForwardedProto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }

	// 直接 TLS：请求带 c.Request.TLS 状态。
	// Direct TLS: the request carries a TLS state.
	routerTLS := gin.New()
	routerTLS.Use(RequireTLS(true))
	routerTLS.GET("/ok", handler)
	reqTLS := httptest.NewRequest(http.MethodGet, "/ok", nil)
	reqTLS.TLS = &tls.ConnectionState{}
	recTLS := httptest.NewRecorder()
	routerTLS.ServeHTTP(recTLS, reqTLS)
	if recTLS.Code != http.StatusOK {
		t.Fatalf("direct TLS request should pass, got %d", recTLS.Code)
	}

	// 经反代终结 TLS：X-Forwarded-Proto: https。
	// Behind a TLS-terminating proxy: X-Forwarded-Proto: https.
	routerProxy := gin.New()
	routerProxy.Use(RequireTLS(true))
	routerProxy.GET("/ok", handler)
	reqProxy := httptest.NewRequest(http.MethodGet, "/ok", nil)
	reqProxy.Header.Set("X-Forwarded-Proto", "https")
	recProxy := httptest.NewRecorder()
	routerProxy.ServeHTTP(recProxy, reqProxy)
	if recProxy.Code != http.StatusOK {
		t.Fatalf("forwarded-proto https should pass, got %d", recProxy.Code)
	}
}

func TestRequireTLSPassthroughWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireTLS(false))
	router.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disabled RequireTLS must not block, got %d", rec.Code)
	}
}

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
