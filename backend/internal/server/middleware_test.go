package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
