package handlers

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/allcallall/backend/internal/user"
)

func TestAuthHandlerRegisterAndLogin(t *testing.T) {
	t.Run("register invalid payload", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/register", []byte(`{"email":"bad"}`))
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("register success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnError(user.ErrNotFound)
		env.mock.ExpectExec("INSERT INTO .*users.*").
			WillReturnResult(sqlmock.NewResult(1, 1))

		rec := performRequest(t, router, "POST", "/api/v1/register", []byte(`{"email":"Alice@Example.com","password":"Abcd1234","display_name":"Alice"}`))
		expectHandlerStatus(t, rec, 201)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["access_token"] == "" {
			t.Fatalf("expected access token, got=%v", got)
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
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

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
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

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
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/login", []byte(`{"email":"bad"}`))
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("login success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr)
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
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("login invalid credentials", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr)
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
		handler := NewAuthHandler(env.logger, env.userSvc, env.jwtMgr)
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
