package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReadinessEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("ready", func(t *testing.T) {
		router := gin.New()
		registerHealthRoutesForTest(router, map[string]ReadinessCheck{
			"mysql": func(context.Context) error { return nil },
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", response.Code)
		}
	})

	t.Run("dependency failure", func(t *testing.T) {
		router := gin.New()
		registerHealthRoutesForTest(router, map[string]ReadinessCheck{
			"redis": func(context.Context) error { return errors.New("unavailable") },
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", response.Code)
		}
	})
}

func registerHealthRoutesForTest(router *gin.Engine, checks map[string]ReadinessCheck) {
	api := router.Group("/api/v1")
	registerHealthRoutes(api, RouteDependencies{ReadinessChecks: checks})
}
