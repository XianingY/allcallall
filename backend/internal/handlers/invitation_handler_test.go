package handlers

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/contact"
	"github.com/allcallall/backend/internal/invitation"
)

func TestInvitationHandlerGetInvitationNotFound(t *testing.T) {
	env := newHandlerTestEnv(t)
	handler := NewInvitationHandler(
		env.logger,
		invitation.NewService(env.db, env.userSvc, env.contactSvc),
		env.contactSvc,
		env.userSvc,
	)
	router := newRouterWithClaims(nil, handler.RegisterPublicRoutes)

	env.mock.ExpectQuery("SELECT .*FROM .*invitations.*code = .*").
		WillReturnError(sqlmock.ErrCancelled)

	rec := performRequest(t, router, "GET", "/api/v1/invitations/missing", nil)
	expectHandlerStatus(t, rec, 500)
}

func TestInvitationHandlerAcceptInvitationGuards(t *testing.T) {
	t.Run("email mismatch", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		commercialSvc := commerce.NewService(env.db)
		contactSvc := contact.NewService(contact.NewRepository(env.db), env.userSvc, commercialSvc)
		handler := NewInvitationHandler(
			env.logger,
			invitation.NewService(env.db, env.userSvc, contactSvc, commercialSvc),
			contactSvc,
			env.userSvc,
		)
		router := newRouterWithClaims(&auth.Claims{UserID: 9, Email: "other@example.com"}, handler.RegisterProtectedRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*invitations.*code = .*").
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "code", "inviter_id", "inviter_email", "inviter_display_name", "target_email",
				"default_source_lang", "default_target_lang", "note", "status", "accepted_user_id",
				"accepted_at", "expires_at", "created_at", "updated_at",
			}).AddRow(
				1, "invite-code", 2, "owner@example.com", "Owner", "target@example.com",
				"en", "zh", "hello", "pending", nil, nil, time.Now().Add(24*time.Hour), time.Now(), time.Now(),
			))

		rec := performRequest(t, router, "POST", "/api/v1/invitations/invite-code/accept", nil)
		expectHandlerStatus(t, rec, 403)
	})

	t.Run("blocked users", func(t *testing.T) {
		env := newHandlerTestEnv(t)
		commercialSvc := commerce.NewService(env.db)
		contactSvc := contact.NewService(contact.NewRepository(env.db), env.userSvc, commercialSvc)
		handler := NewInvitationHandler(
			env.logger,
			invitation.NewService(env.db, env.userSvc, contactSvc, commercialSvc),
			contactSvc,
			env.userSvc,
		)
		router := newRouterWithClaims(&auth.Claims{UserID: 9, Email: "target@example.com"}, handler.RegisterProtectedRoutes)

		env.mock.ExpectQuery("SELECT .*FROM .*invitations.*code = .*").
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "code", "inviter_id", "inviter_email", "inviter_display_name", "target_email",
				"default_source_lang", "default_target_lang", "note", "status", "accepted_user_id",
				"accepted_at", "expires_at", "created_at", "updated_at",
			}).AddRow(
				1, "invite-code", 2, "owner@example.com", "Owner", "target@example.com",
				"en", "zh", "hello", "pending", nil, nil, time.Now().Add(24*time.Hour), time.Now(), time.Now(),
			))
		env.mock.ExpectQuery("SELECT count\\(\\*\\) FROM .*user_blocks.*").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		rec := performRequest(t, router, "POST", "/api/v1/invitations/invite-code/accept", nil)
		expectHandlerStatus(t, rec, 403)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if got["code"] != "USER_BLOCKED" {
			t.Fatalf("expected USER_BLOCKED, got=%v body=%s", got["code"], rec.Body.String())
		}
	})
}
