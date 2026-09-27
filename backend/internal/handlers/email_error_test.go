package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func postVerifyCode(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/email/verify-code", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// RED: site 6 — the public verify-code endpoint must never echo raw DB/SQL
// errors. Unknown failures become a generic 500 message.
func TestHandleVerifyCodeDBFailureDoesNotLeakInternals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newHandlerTestEnv(t)
	handler := NewEmailHandler(zerolog.Nop(), env.verifySvc)
	router := newRouterWithClaims(nil, handler.RegisterRoutes)

	// No sqlmock expectations: the verification lookup fails like a DB outage.
	rec := postVerifyCode(t, router, `{"email":"user@example.com","code":"123456"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for verification DB failure, got %d body=%s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	for _, fragment := range []string{"SELECT", "email_verification_codes", "was not expected", "sqlmock"} {
		if strings.Contains(body, fragment) {
			t.Fatalf("response leaked internal error fragment %q: %s", fragment, body)
		}
	}
	var resp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, body)
	}
	if resp.Error != "failed to verify code" {
		t.Fatalf("expected generic message, got %q", resp.Error)
	}
}

// Contract guard: an unknown/used verification code stays a typed 401 with its
// user-facing business message (typed sentinel error).
func TestHandleVerifyCodeNotFoundKeepsBusinessMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newHandlerTestEnv(t)
	handler := NewEmailHandler(zerolog.Nop(), env.verifySvc)
	router := newRouterWithClaims(nil, handler.RegisterRoutes)

	// Empty result set -> gorm.ErrRecordNotFound -> mail.ErrVerificationCodeNotFoundOrUsed.
	env.mock.ExpectQuery("email_verification_codes").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	rec := postVerifyCode(t, router, `{"email":"user@example.com","code":"123456"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unknown verification code, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	if resp.Error != "verification code not found or already used" {
		t.Fatalf("expected business message kept, got %q", resp.Error)
	}
}
