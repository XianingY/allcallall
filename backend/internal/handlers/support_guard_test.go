package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireSupportNetwork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUPPORT_INTERNAL_ONLY", "true")

	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		allowed    bool
	}{
		{name: "private direct", remoteAddr: "10.0.0.2:1234", allowed: true},
		{name: "public direct", remoteAddr: "203.0.113.9:1234", allowed: false},
		{name: "private proxy public client", remoteAddr: "172.18.0.2:1234", forwarded: "203.0.113.9", allowed: false},
		{name: "private proxy private client", remoteAddr: "172.18.0.2:1234", forwarded: "10.1.2.3", allowed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/support", nil)
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", test.forwarded)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			if got := requireSupportNetwork(context); got != test.allowed {
				t.Fatalf("expected allowed=%v, got %v", test.allowed, got)
			}
		})
	}
}

// TestRequireSupportNetworkDefaultRestricted verifies the secure-by-default
// behavior: when SUPPORT_INTERNAL_ONLY is unset the support API is restricted
// to the internal network (fail closed).
func TestRequireSupportNetworkDefaultRestricted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUPPORT_INTERNAL_ONLY", "")

	tests := []struct {
		name       string
		remoteAddr string
		allowed    bool
	}{
		{name: "private direct allowed", remoteAddr: "10.0.0.2:1234", allowed: true},
		{name: "public direct blocked", remoteAddr: "203.0.113.9:1234", allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/support", nil)
			request.RemoteAddr = test.remoteAddr
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			if got := requireSupportNetwork(context); got != test.allowed {
				t.Fatalf("expected allowed=%v, got %v", test.allowed, got)
			}
		})
	}
}

// TestRequireSupportNetworkOptOut confirms an explicit opt-out opens the API.
func TestRequireSupportNetworkOptOut(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SUPPORT_INTERNAL_ONLY", "false")

	request := httptest.NewRequest(http.MethodGet, "/support", nil)
	request.RemoteAddr = "203.0.113.9:1234"
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	if !requireSupportNetwork(context) {
		t.Fatal("expected opt-out to allow external access")
	}
}
