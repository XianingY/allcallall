package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/allcallall/backend/internal/user"
)

func TestAuthHandlerRegisterAndLogin(t *testing.T) {
	t.Run("register invalid payload", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/register", []byte(`{"email":"bad"}`))
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("register requires verified email", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*is_verified.*consumed_at IS NULL.*").
			WillReturnError(user.ErrNotFound)

		rec := performRequest(t, router, "POST", "/api/v1/register", []byte(`{"email":"alice@example.com","password":"Abcd1234","display_name":"Alice"}`))
		expectHandlerStatus(t, rec, 403)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("register success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		now := time.Now()
		env.mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*is_verified.*consumed_at IS NULL.*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "code", "is_verified", "verified_at", "consumed_at", "attempt_count", "max_attempts", "last_attempt_at", "blocked_until", "expires_at", "created_at", "updated_at"}).
				AddRow(1, "Alice@Example.com", "123456", true, now, nil, 0, 3, nil, nil, now.Add(time.Minute), now, now))
		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnError(user.ErrNotFound)
		env.mock.ExpectExec("INSERT INTO .*users.*").
			WillReturnResult(sqlmock.NewResult(1, 1))
		env.mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*is_verified.*consumed_at IS NULL.*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "code", "is_verified", "verified_at", "consumed_at", "attempt_count", "max_attempts", "last_attempt_at", "blocked_until", "expires_at", "created_at", "updated_at"}).
				AddRow(1, "Alice@Example.com", "123456", true, now, nil, 0, 3, nil, nil, now.Add(time.Minute), now, now))
		env.mock.ExpectExec("UPDATE .*email_verification_codes.*").
			WillReturnResult(sqlmock.NewResult(0, 1))

		rec := performRequest(t, router, "POST", "/api/v1/register", []byte(`{"email":"Alice@Example.com","password":"Abcd1234","display_name":"Alice"}`))
		expectHandlerStatus(t, rec, 201)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["access_token"] == "" {
			t.Fatalf("expected access token, got=%v", got)
		}
		cookie := findCookie(t, rec, refreshCookieName)
		if !cookie.HttpOnly {
			t.Fatal("expected refresh cookie to be HttpOnly")
		}
		if cookie.Path != "/api/v1/auth" {
			t.Fatalf("unexpected refresh cookie path: %s", cookie.Path)
		}
		if _, ok := got["user"].(map[string]any); !ok {
			t.Fatalf("expected user payload, got=%v", got)
		}
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("register duplicate", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		now := time.Now()
		env.mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*is_verified.*consumed_at IS NULL.*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "code", "is_verified", "verified_at", "consumed_at", "attempt_count", "max_attempts", "last_attempt_at", "blocked_until", "expires_at", "created_at", "updated_at"}).
				AddRow(1, "alice@example.com", "123456", true, now, nil, 0, 3, nil, nil, now.Add(time.Minute), now, now))
		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, "Abcd1234"), "Alice", "", nil, nil, nil))

		rec := performRequest(t, router, "POST", "/api/v1/register", []byte(`{"email":"alice@example.com","password":"Abcd1234","display_name":"Alice"}`))
		expectHandlerStatus(t, rec, 409)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("register service error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		now := time.Now()
		env.mock.ExpectQuery("SELECT .*FROM .*email_verification_codes.*is_verified.*consumed_at IS NULL.*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "code", "is_verified", "verified_at", "consumed_at", "attempt_count", "max_attempts", "last_attempt_at", "blocked_until", "expires_at", "created_at", "updated_at"}).
				AddRow(1, "new@example.com", "123456", true, now, nil, 0, 3, nil, nil, now.Add(time.Minute), now, now))
		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnError(errors.New("db failure"))

		rec := performRequest(t, router, "POST", "/api/v1/register", []byte(`{"email":"new@example.com","password":"Abcd1234","display_name":"New"}`))
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("login invalid payload", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/login", []byte(`{"email":"bad"}`))
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("login success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, "Abcd1234"), "Alice", "", nil, nil, nil))

		rec := performRequest(t, router, "POST", "/api/v1/login", []byte(`{"email":"alice@example.com","password":"Abcd1234"}`))
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["access_token"] == "" {
			t.Fatalf("expected access token, got=%v", got)
		}
		if cookie := findCookie(t, rec, refreshCookieName); !cookie.HttpOnly || cookie.MaxAge <= 0 {
			t.Fatalf("expected persistent HttpOnly refresh cookie, got=%+v", cookie)
		}
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("refresh success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		refreshToken, err := env.jwtMgr.GenerateRefreshToken(1, "alice@example.com")
		if err != nil {
			t.Fatalf("generate refresh token failed: %v", err)
		}
		env.mock.ExpectQuery("SELECT .*FROM .*users.*id.*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, "Abcd1234"), "Alice", "", nil, nil, nil))

		req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
		req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: refreshToken})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["access_token"] == "" {
			t.Fatalf("expected access token, got=%v", got)
		}
		if cookie := findCookie(t, rec, refreshCookieName); !cookie.HttpOnly || cookie.MaxAge <= 0 {
			t.Fatalf("expected rotated HttpOnly refresh cookie, got=%+v", cookie)
		}
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("refresh rejects access token cookie", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		accessToken, err := env.jwtMgr.GenerateAccessToken(1, "alice@example.com")
		if err != nil {
			t.Fatalf("generate access token failed: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
		req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: accessToken})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		expectHandlerStatus(t, rec, 401)
		if cookie := findCookie(t, rec, refreshCookieName); cookie.MaxAge >= 0 {
			t.Fatalf("expected refresh cookie clear, got=%+v", cookie)
		}
	})

	t.Run("logout clears refresh cookie", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/logout", nil)
		expectHandlerStatus(t, rec, 200)
		if cookie := findCookie(t, rec, refreshCookieName); cookie.MaxAge >= 0 {
			t.Fatalf("expected refresh cookie clear, got=%+v", cookie)
		}
	})

	t.Run("login invalid credentials", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnError(user.ErrNotFound)

		rec := performRequest(t, router, "POST", "/api/v1/login", []byte(`{"email":"missing@example.com","password":"Abcd1234"}`))
		expectHandlerStatus(t, rec, 401)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("login service error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr, env.verifySvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnError(errors.New("db failure"))

		rec := performRequest(t, router, "POST", "/api/v1/login", []byte(`{"email":"alice@example.com","password":"Abcd1234"}`))
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}

func findCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("expected cookie %s in response", name)
	return nil
}
