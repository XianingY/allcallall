package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The reason the key is not the IP: mobile carriers put thousands of
// subscribers behind one NAT address, so a per-IP limit punishes innocent users
// for a neighbour's traffic and the symptom reads as "the app randomly rejects
// me". These tests pin the identity-based key that fixes it.
func TestRateLimitKeySeparatesUsersSharingAnAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newCtx := func(token, ip string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/api/v1/conversations", nil)
		c.Request.RemoteAddr = ip + ":51000"
		if token != "" {
			c.Request.Header.Set("Authorization", token)
		}
		return c
	}

	const sharedIP = "203.0.113.7" // one carrier NAT

	alice := rateLimitKey(newCtx("Bearer alice-token", sharedIP))
	bob := rateLimitKey(newCtx("Bearer bob-token", sharedIP))
	anonymous := rateLimitKey(newCtx("", sharedIP))

	if alice == bob {
		t.Fatalf("two users behind %s share a bucket (%s); one can throttle the other", sharedIP, alice)
	}
	if alice == anonymous {
		t.Fatal("an authenticated caller and an anonymous one must not share a bucket")
	}
	if !strings.HasPrefix(alice, "global:token:") {
		t.Errorf("authenticated callers should key on identity, got %q", alice)
	}
	if !strings.HasPrefix(anonymous, "global:ip:") {
		t.Errorf("unauthenticated traffic should fall back to IP, got %q", anonymous)
	}
}

func TestRateLimitKeyIsStablePerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	make1 := func() string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/api/v1/conversations", nil)
		c.Request.RemoteAddr = "198.51.100.4:51000"
		c.Request.Header.Set("Authorization", "Bearer stable-token")
		return rateLimitKey(c)
	}
	if make1() != make1() {
		t.Fatal("the same token must map to the same bucket, or the limit is per-request")
	}
}

func TestRateLimitKeyDoesNotLeakTheBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/conversations", nil)
	c.Request.RemoteAddr = "198.51.100.9:51000"
	c.Request.Header.Set("Authorization", "Bearer super-secret-jwt-value")

	key := rateLimitKey(c)
	if strings.Contains(key, "super-secret-jwt-value") {
		t.Fatalf("the rate limit key must not contain the credential itself: %q", key)
	}
}
