package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestForceHTTPSRedirectRedirectsPlaintext(t *testing.T) {
	router := gin.New()
	router.Use(ForceHTTPSRedirect())
	router.GET("/api/v1/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	req.Host = "app.example.com"
	router.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://app.example.com/api/v1/ping" {
		t.Fatalf("unexpected redirect target: %q", loc)
	}
}

func TestForceHTTPSRedirectPassesHTTPS(t *testing.T) {
	router := gin.New()
	router.Use(ForceHTTPSRedirect())
	router.GET("/api/v1/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	req.TLS = &tls.ConnectionState{}
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for https, got %d", w.Code)
	}
}
