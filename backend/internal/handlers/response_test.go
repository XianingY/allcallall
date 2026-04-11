package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestJSONError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	JSONError(c, http.StatusBadRequest, "bad request")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got["error"] != "bad request" {
		t.Fatalf("unexpected error payload: %+v", got)
	}
	if got["success"] != false {
		t.Fatalf("unexpected success payload: %+v", got)
	}
}

func TestJSONSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("default body and status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		JSONSuccess(c, 0, nil)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}

		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if got["success"] != true {
			t.Fatalf("unexpected payload: %+v", got)
		}
	})

	t.Run("custom body and status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		payload := gin.H{"ok": true}
		JSONSuccess(c, http.StatusCreated, payload)

		if rec.Code != http.StatusCreated {
			t.Fatalf("unexpected status: %d", rec.Code)
		}

		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if got["ok"] != true {
			t.Fatalf("unexpected payload: %+v", got)
		}
	})
}
