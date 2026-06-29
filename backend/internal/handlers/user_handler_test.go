package handlers

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/user"
)

func testPassword() string {
	return "Abcd" + "1234"
}

func TestUserHandlerEndpoints(t *testing.T) {
	claims := &auth.Claims{UserID: 1, Email: "alice@example.com"}

	t.Run("me unauthorized", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		rec := performRequest(t, router, "GET", "/api/v1/me", nil)
		expectHandlerStatus(t, rec, 401)
	})

	t.Run("me success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "GET", "/api/v1/me", nil)
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		userPayload, ok := got["user"].(map[string]any)
		if !ok {
			t.Fatalf("expected user payload, got=%v", got)
		}
		if userPayload["email"] != "alice@example.com" {
			t.Fatalf("unexpected user payload: %v", userPayload)
		}
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("me service error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnError(errors.New("db failure"))

		rec := performRequest(t, router, "GET", "/api/v1/me", nil)
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("search empty query", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "GET", "/api/v1/search?q=", nil)
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		results, ok := got["results"].([]any)
		if !ok || len(results) != 0 {
			t.Fatalf("expected empty results, got=%v", got)
		}
	})

	t.Run("search filters self", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LIKE.*ORDER BY .*created_at.*LIMIT").
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
				AddRow(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil).
				AddRow(2, "bob@example.com", mustHashPassword(t, testPassword()), "Bob", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "GET", "/api/v1/search?q=example", nil)
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		results, ok := got["results"].([]any)
		if !ok || len(results) != 1 {
			t.Fatalf("expected one result after filtering self, got=%v", got)
		}
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("search error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LIKE.*ORDER BY .*created_at.*LIMIT").
			WillReturnError(errors.New("query failed"))

		rec := performRequest(t, router, "GET", "/api/v1/search?q=example", nil)
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("presence default", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "GET", "/api/v1/presence", nil)
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		presencePayload, ok := got["presence"].([]any)
		if !ok || len(presencePayload) != 1 {
			t.Fatalf("expected single presence item, got=%v", got)
		}
	})

	t.Run("presence custom list", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "GET", "/api/v1/presence?emails=bob@example.com,carol@example.com", nil)
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		presencePayload, ok := got["presence"].([]any)
		if !ok || len(presencePayload) != 2 {
			t.Fatalf("expected two presence items, got=%v", got)
		}
	})

	t.Run("add contact success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnRows(userRows(2, "bob@example.com", mustHashPassword(t, testPassword()), "Bob", "", time.Now(), time.Now(), nil))
		env.mock.ExpectQuery("SELECT count\\(\\*\\) FROM .*contacts.*owner_id = .*contact_id = .*").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		env.mock.ExpectExec("INSERT INTO .*contacts.*").
			WillReturnResult(sqlmock.NewResult(1, 1))

		rec := performRequest(t, router, "POST", "/api/v1/contacts", []byte(`{"email":"bob@example.com"}`))
		expectHandlerStatus(t, rec, 201)

		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("add contact self", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "POST", "/api/v1/contacts", []byte(`{"email":"alice@example.com"}`))
		expectHandlerStatus(t, rec, 400)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("add contact duplicate", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnRows(userRows(2, "bob@example.com", mustHashPassword(t, testPassword()), "Bob", "", time.Now(), time.Now(), nil))
		env.mock.ExpectQuery("SELECT count\\(\\*\\) FROM .*contacts.*owner_id = .*contact_id = .*").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		rec := performRequest(t, router, "POST", "/api/v1/contacts", []byte(`{"email":"bob@example.com"}`))
		expectHandlerStatus(t, rec, 409)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("add contact service error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
			WillReturnError(errors.New("user lookup failed"))

		rec := performRequest(t, router, "POST", "/api/v1/contacts", []byte(`{"email":"bob@example.com"}`))
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("list contacts success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*contacts.*JOIN users ON contacts.contact_id = users.id.*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
				AddRow(2, "bob@example.com", mustHashPassword(t, testPassword()), "Bob", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "GET", "/api/v1/contacts", nil)
		expectHandlerStatus(t, rec, 200)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("list contacts error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*contacts.*JOIN users ON contacts.contact_id = users.id.*").
			WillReturnError(errors.New("query failed"))

		rec := performRequest(t, router, "GET", "/api/v1/contacts", nil)
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("remove contact invalid id", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "DELETE", "/api/v1/contacts/not-a-number", nil)
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("remove contact success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectExec("DELETE FROM .*contacts.*owner_id = .*contact_id = .*").
			WillReturnResult(sqlmock.NewResult(0, 1))

		rec := performRequest(t, router, "DELETE", "/api/v1/contacts/2", nil)
		expectHandlerStatus(t, rec, 200)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("remove contact error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectExec("DELETE FROM .*contacts.*owner_id = .*contact_id = .*").
			WillReturnError(errors.New("delete failed"))

		rec := performRequest(t, router, "DELETE", "/api/v1/contacts/2", nil)
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))
		env.mock.ExpectExec("UPDATE .*users.*password_hash.*").
			WillReturnResult(sqlmock.NewResult(0, 1))

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"`+testPassword()+`","new_password":"Newpass1","confirm_password":"Newpass1"}`))
		expectHandlerStatus(t, rec, 200)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password invalid credentials", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"wrong","new_password":"Newpass1","confirm_password":"Newpass1"}`))
		expectHandlerStatus(t, rec, 401)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password too short", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"`+testPassword()+`","new_password":"Abc123","confirm_password":"Abc123"}`))
		expectHandlerStatus(t, rec, 400)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password mismatch", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"`+testPassword()+`","new_password":"Newpass1","confirm_password":"Newpass2"}`))
		expectHandlerStatus(t, rec, 400)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password weak", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"`+testPassword()+`","new_password":"abcdefgh","confirm_password":"abcdefgh"}`))
		expectHandlerStatus(t, rec, 400)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password special chars", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"`+testPassword()+`","new_password":"Newpass!","confirm_password":"Newpass!"}`))
		expectHandlerStatus(t, rec, 400)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password unchanged", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"`+testPassword()+`","new_password":"`+testPassword()+`","confirm_password":"`+testPassword()+`"}`))
		expectHandlerStatus(t, rec, 400)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password not found", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnError(user.ErrNotFound)

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"`+testPassword()+`","new_password":"Newpass1","confirm_password":"Newpass1"}`))
		expectHandlerStatus(t, rec, 404)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("change password update error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
			WillReturnRows(userRows(1, "alice@example.com", mustHashPassword(t, testPassword()), "Alice", "", time.Now(), time.Now(), nil))
		env.mock.ExpectExec("UPDATE .*users.*password_hash.*").
			WillReturnError(errors.New("update failed"))

		rec := performRequest(t, router, "POST", "/api/v1/change-password", []byte(`{"old_password":"`+testPassword()+`","new_password":"Newpass1","confirm_password":"Newpass1"}`))
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("save fcm token invalid payload", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/fcm-token", []byte(`{"fcm_token":""}`))
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("save fcm token success", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectExec("UPDATE .*users.*fcm_token.*").
			WillReturnResult(sqlmock.NewResult(0, 1))

		rec := performRequest(t, router, "POST", "/api/v1/fcm-token", []byte(`{"fcm_token":"fcm-token-1234567890abcdef"}`))
		expectHandlerStatus(t, rec, 200)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("save fcm token error", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		handler := NewUserHandler(env.logger, env.userSvc, env.presence, env.contactSvc)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		env.mock.ExpectExec("UPDATE .*users.*fcm_token.*").
			WillReturnError(errors.New("update failed"))

		rec := performRequest(t, router, "POST", "/api/v1/fcm-token", []byte(`{"fcm_token":"fcm-token-1234567890abcdef"}`))
		expectHandlerStatus(t, rec, 500)
		if err := env.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}
