package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// RED: 0.4 — a genuine database failure on the conversation list must answer a
// generic 500 message, never the raw SQL error (table names, driver text).
func TestHandleListConversationsDBFailureDoesNotLeakInternals(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, true)
	if err := db.Exec("DROP TABLE conversations").Error; err != nil {
		t.Fatalf("drop conversations table failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations", nil)
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", org.ID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for conversation list DB failure, got %d body=%s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, body)
	}
	if resp.Error != "failed to list conversations" {
		t.Fatalf("expected generic message, got %q", resp.Error)
	}
	for _, fragment := range []string{"no such table", "SELECT", "sqlite", "DROP TABLE"} {
		if strings.Contains(body, fragment) {
			t.Fatalf("response leaked internal fragment %q: %s", fragment, body)
		}
	}
}

// RED: 0.4 — a malformed create-conversation body must answer a fixed
// "invalid request body" 400 without echoing validator internals.
func TestHandleCreateConversationBindingErrorIsSanitized(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", org.ID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed body, got %d body=%s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, body)
	}
	if resp.Error != "invalid request body" {
		t.Fatalf("expected fixed binding message, got %q", resp.Error)
	}
	if strings.Contains(body, "invalid character") || strings.Contains(body, "Code(") {
		t.Fatalf("response leaked binding internals: %s", body)
	}
}
