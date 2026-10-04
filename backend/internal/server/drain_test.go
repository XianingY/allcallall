package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The point of failing /ready during shutdown is that it pulls the Pod out of
// the endpoint slice. The point of leaving /health alone is that liveness must
// not restart it: a Pod shutting down on purpose would otherwise be killed
// mid-drain and the connections it is trying to hand over would be cut.
func TestReadinessFailsWhileDrainingButHealthStaysUp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// The flag is package-level, so restore it to "not draining" afterwards.
	// Leaving it true would fail every later readiness assertion in this package.
	draining.Store(false)
	t.Cleanup(func() { draining.Store(false) })

	router := gin.New()
	api := router.Group("/api/v1")
	registerHealthRoutes(api, RouteDependencies{})

	get := func(path string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}

	if code, _ := get("/api/v1/ready"); code != http.StatusOK {
		t.Fatalf("readiness should pass before shutdown, got %d", code)
	}

	BeginDrain()

	code, body := get("/api/v1/ready")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("readiness must fail once draining so traffic moves away, got %d", code)
	}
	if body["status"] != "draining" {
		t.Errorf("the failure should say why, got %v", body["status"])
	}

	if code, body := get("/api/v1/health"); code != http.StatusOK {
		t.Fatalf("health must stay green during drain or liveness will restart us mid-handover, got %d", code)
	} else if body["status"] != "ok" {
		t.Errorf("health body changed during drain: %v", body)
	}
}
