package apperror

import (
	"errors"
	"net/http"
	"testing"
)

func TestNew(t *testing.T) {
	e := New("CUSTOM_CODE", "something failed", http.StatusTeapot)
	if e.Code != "CUSTOM_CODE" {
		t.Fatalf("unexpected code: %q", e.Code)
	}
	if e.Message != "something failed" {
		t.Fatalf("unexpected message: %q", e.Message)
	}
	if e.HTTPStatus != http.StatusTeapot {
		t.Fatalf("unexpected http status: %d", e.HTTPStatus)
	}
	if e.Internal != nil {
		t.Fatal("expected no wrapped error")
	}
}

func TestErrorWithoutInternal(t *testing.T) {
	e := New(ErrCodeNotFound, "missing", http.StatusNotFound)
	want := "NOT_FOUND: missing"
	if got := e.Error(); got != want {
		t.Fatalf("unexpected error string: %q want %q", got, want)
	}
}

func TestErrorWithInternal(t *testing.T) {
	inner := errors.New("boom")
	e := Wrap(inner, ErrCodeInternalServerError, "wrapped", http.StatusInternalServerError)
	want := "INTERNAL_SERVER_ERROR: wrapped (internal: boom)"
	if got := e.Error(); got != want {
		t.Fatalf("unexpected error string: %q want %q", got, want)
	}
}

func TestUnwrap(t *testing.T) {
	inner := errors.New("root cause")
	e := Wrap(inner, ErrCodeInternalServerError, "wrapped", http.StatusInternalServerError)
	if !errors.Is(e, inner) {
		t.Fatal("errors.Is should find the wrapped internal error")
	}
	if e.Unwrap() != inner {
		t.Fatal("Unwrap should return the internal error")
	}
}

func TestWrap(t *testing.T) {
	inner := errors.New("inner")
	e := Wrap(inner, "CODE", "msg", http.StatusBadGateway)
	if e.Internal != inner {
		t.Fatal("Wrap should store the internal error")
	}
	if e.Code != "CODE" || e.Message != "msg" || e.HTTPStatus != http.StatusBadGateway {
		t.Fatalf("unexpected fields: %+v", e)
	}
}

func TestConstructorsSetExpectedStatus(t *testing.T) {
	cases := []struct {
		name     string
		err      *AppError
		wantCode string
		wantHTTP int
	}{
		{"not_found", NewNotFound("n"), ErrCodeNotFound, http.StatusNotFound},
		{"invalid_request", NewInvalidRequest("i"), ErrCodeInvalidRequest, http.StatusBadRequest},
		{"internal", NewInternal(errors.New("x"), "i"), ErrCodeInternalServerError, http.StatusInternalServerError},
		{"unauthorized", NewUnauthorized("u"), ErrCodeUnauthorized, http.StatusUnauthorized},
		{"forbidden", NewForbidden("f"), ErrCodeForbidden, http.StatusForbidden},
		{"insufficient_balance", NewInsufficientBalance("b"), ErrCodeInsufficientBalance, http.StatusPaymentRequired},
		{"rate_limited", NewRateLimited("r"), ErrCodeRateLimited, http.StatusTooManyRequests},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Code != tc.wantCode {
				t.Fatalf("unexpected code: %q want %q", tc.err.Code, tc.wantCode)
			}
			if tc.err.HTTPStatus != tc.wantHTTP {
				t.Fatalf("unexpected http status: %d want %d", tc.err.HTTPStatus, tc.wantHTTP)
			}
			if tc.err.Message == "" {
				t.Fatal("expected a non-empty message")
			}
		})
	}
}

func TestNewInternalWrapsError(t *testing.T) {
	inner := errors.New("db down")
	e := NewInternal(inner, "query failed")
	if !errors.Is(e, inner) {
		t.Fatal("NewInternal should wrap the provided error")
	}
}
