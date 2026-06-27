package knowledge

import (
	"net/http"
	"testing"
)

func TestSecureRedirectPolicyRejectsPrivateTargets(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/internal", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if err := secureRedirectPolicy(nil)(request, nil); err == nil {
		t.Fatal("expected private redirect target to be rejected")
	}
}

func TestSecureRedirectPolicyPreservesCallerPolicy(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://93.184.216.34/resource", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	called := false
	policy := secureRedirectPolicy(func(*http.Request, []*http.Request) error {
		called = true
		return nil
	})
	if err := policy(request, nil); err != nil {
		t.Fatalf("expected public redirect target to pass: %v", err)
	}
	if !called {
		t.Fatal("expected caller redirect policy to run")
	}
}
