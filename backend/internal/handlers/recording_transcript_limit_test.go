package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/pagination"
)

// getRecordingTranscript issues an authenticated transcript GET against the
// wave-1 error-test router (owner + organization on sqlite).
func getRecordingTranscript(t *testing.T, router http.Handler, orgID uint64, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", orgID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type transcriptErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func decodeTranscriptError(t *testing.T, rec *httptest.ResponseRecorder) transcriptErrorResponse {
	t.Helper()
	var resp transcriptErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	return resp
}

// RED: wave-2 (b) — a non-sentinel service error (missing table / SQL detail)
// must not echo err.Error() to the wire. The 404 status and the
// RECORDING_TRANSCRIPT_NOT_FOUND code stay exactly as before.
func TestHandleGetRecordingTranscriptServiceErrorIsSanitized(t *testing.T) {
	// recordingSessions=false → recording_sessions table is absent, so the
	// service returns a raw database error the way an outage would.
	router, _, org := newCollaborationErrorTestEnv(t, false)

	rec := getRecordingTranscript(t, router, org.ID, "/api/v1/recordings/1/transcript")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for transcript load failure, got %d body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeTranscriptError(t, rec)
	if resp.Code != "RECORDING_TRANSCRIPT_NOT_FOUND" {
		t.Fatalf("expected code RECORDING_TRANSCRIPT_NOT_FOUND, got %q", resp.Code)
	}
	if resp.Error != "recording transcript not found" {
		t.Fatalf("expected generic message, got %q", resp.Error)
	}
	body := rec.Body.String()
	if strings.Contains(body, "no such table") || strings.Contains(body, "recording_sessions") {
		t.Fatalf("response leaked internal DB error: %s", body)
	}
}

// RED: wave-2 (a) — limit=10^9 must be clamped to pagination.MaxLimit at the
// handler instead of flowing into service.GetRecordingTranscript unclamped.
func TestParseTranscriptLimitClampsHugeValue(t *testing.T) {
	limit, err := parseTranscriptLimit("100000000")
	if err != nil {
		t.Fatalf("expected huge limit to clamp, got error %v", err)
	}
	if limit != pagination.MaxLimit {
		t.Fatalf("expected limit to clamp to %d, got %d", pagination.MaxLimit, limit)
	}
}

// Wave-2 (a) contract: default stays 100, values at/under the max pass
// through untouched, and `<= 0` / non-numeric stay rejected (400 invalid
// limit).
func TestParseTranscriptLimitDefaultsAndRejects(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 100},
		{"  ", 100},
		{"1", 1},
		{"250", 250},
		{"500", 500},
	}
	for _, tc := range cases {
		got, err := parseTranscriptLimit(tc.raw)
		if err != nil {
			t.Fatalf("parseTranscriptLimit(%q) unexpected error: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("parseTranscriptLimit(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
	for _, raw := range []string{"0", "-1", "abc"} {
		if _, err := parseTranscriptLimit(raw); err == nil {
			t.Fatalf("parseTranscriptLimit(%q) expected error, got none", raw)
		}
	}
}

// Invariant guard: a whitelisted sentinel (gorm.ErrRecordNotFound) keeps its
// own message, the 404 status and the stable code.
func TestHandleGetRecordingTranscriptNotFoundKeepsWhitelistedMessage(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, true)

	rec := getRecordingTranscript(t, router, org.ID, "/api/v1/recordings/999999/transcript")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing recording, got %d body=%s", rec.Code, rec.Body.String())
	}
	resp := decodeTranscriptError(t, rec)
	if resp.Code != "RECORDING_TRANSCRIPT_NOT_FOUND" {
		t.Fatalf("expected code RECORDING_TRANSCRIPT_NOT_FOUND, got %q", resp.Code)
	}
	if resp.Error != "record not found" {
		t.Fatalf("expected whitelisted sentinel message, got %q", resp.Error)
	}
}

// Invariant guard: a huge limit is clamped, never rejected — existing
// pagination clients keep receiving a normal 200 page.
func TestHandleGetRecordingTranscriptHugeLimitReturnsPageNot400(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, true)
	if err := db.AutoMigrate(&models.RecordingTranscription{}, &models.MeetingTranscriptSegment{}); err != nil {
		t.Fatalf("auto migrate transcript tables failed: %v", err)
	}
	var owner models.User
	if err := db.Where("email = ?", "owner@example.com").Take(&owner).Error; err != nil {
		t.Fatalf("load owner failed: %v", err)
	}
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         42,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}

	rec := getRecordingTranscript(t, router, org.ID,
		fmt.Sprintf("/api/v1/recordings/%d/transcript?limit=100000000", session.ID))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for clamped huge limit, got %d body=%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Segments []models.MeetingTranscriptSegment `json:"segments"`
		Error    string                            `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode page failed: %v body=%s", err, rec.Body.String())
	}
	if page.Error != "" || page.Segments == nil {
		t.Fatalf("expected a segment page, got body=%s", rec.Body.String())
	}
}
