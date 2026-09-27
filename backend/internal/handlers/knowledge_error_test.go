package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// RED: site 1 — writeKnowledgeError must not echo the raw internal error
// string for unknown (non-sentinel) errors, only a generic message with the
// stable KNOWLEDGE_REQUEST_FAILED code and a 500 status.
func TestWriteKnowledgeErrorUnknownErrorDoesNotLeakInternals(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewKnowledgeHandler(zerolog.Nop(), nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/sources", nil)

	internal := "driver: connection reset by peer: db=knowledge_sources"
	h.writeKnowledgeError(c, errors.New(internal))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unknown knowledge error, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	if resp.Code != "KNOWLEDGE_REQUEST_FAILED" {
		t.Fatalf("expected code KNOWLEDGE_REQUEST_FAILED, got %q", resp.Code)
	}
	if resp.Error == "" {
		t.Fatal("expected a generic error message, got empty string")
	}
	body := rec.Body.String()
	if resp.Error == internal || strings.Contains(body, internal) {
		t.Fatalf("response leaked internal error string: %s", body)
	}
}
