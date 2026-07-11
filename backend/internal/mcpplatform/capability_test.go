package mcpplatform

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func TestCapabilityRejectsCrossTenantAndRevisionMismatch(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewCapabilityManager(privateKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := manager.Issue(CapabilityClaims{
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunRef:         "agent:99",
		Revisions:      map[string]uint64{"3": 11},
		Tools:          []string{"mcp.3.search"},
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Verify(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !claims.Allows(1, 7, 42, "agent:99", "mcp.3.search", 3, 11) {
		t.Fatal("expected matching capability to authorize tool")
	}
	checks := []struct {
		name                       string
		organizationID, revisionID uint64
	}{
		{name: "cross organization", organizationID: 2, revisionID: 11},
		{name: "revision mismatch", organizationID: 1, revisionID: 12},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if claims.Allows(check.organizationID, 7, 42, "agent:99", "mcp.3.search", 3, check.revisionID) {
				t.Fatal("capability authorized mismatched subject")
			}
		})
	}
}

func TestCapabilityExpires(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewCapabilityManager(privateKey, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := manager.Issue(CapabilityClaims{
		OrganizationID: 1,
		UserID:         7,
		ConversationID: 42,
		RunRef:         "workflow:9",
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := manager.Verify(raw); !errors.Is(err, ErrInvalidCapability) {
		t.Fatalf("expected expired capability error, got %v", err)
	}
}
