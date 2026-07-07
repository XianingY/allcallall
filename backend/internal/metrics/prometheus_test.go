package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(PrometheusMiddleware())

	router.GET("/test", func(c *gin.Context) {
		c.String(200, "OK")
	})
	router.GET("/error", func(c *gin.Context) {
		c.Status(500)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/error", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != 500 {
		t.Errorf("Expected status 500, got %d", w2.Code)
	}

	// Verify metrics were collected
	count := testutil.CollectAndCount(HttpRequestsTotal)
	if count == 0 {
		t.Error("Expected HttpRequestsTotal to be collected")
	}

	// Verify format
	err := testutil.CollectAndCompare(HttpRequestsTotal, strings.NewReader(`
		# HELP http_requests_total Total number of HTTP requests
		# TYPE http_requests_total counter
		http_requests_total{method="GET",path="/error",status="500"} 1
		http_requests_total{method="GET",path="/test",status="200"} 1
	`), "http_requests_total")

	if err != nil {
		t.Errorf("Metrics comparison failed: %v", err)
	}
}
