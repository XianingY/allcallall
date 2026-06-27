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
