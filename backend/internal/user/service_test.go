package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/allcallall/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMockService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock failed: %v", err)
	}

	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = db.Close()
		t.Fatalf("open gorm db failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return NewService(NewRepository(gdb)), mock
}

func userRows(u models.User) *sqlmock.Rows {
	cols := []string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}
	return sqlmock.NewRows(cols).AddRow(u.ID, u.Email, u.PasswordHash, u.DisplayName, u.FCMToken, u.CreatedAt, u.UpdatedAt, u.LastSeen)
}

func mustPasswordHash(t *testing.T, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generate password hash failed: %v", err)
	}
	return string(hash)
}

func TestServiceHappyPath(t *testing.T) {
	svc, mock := newMockService(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnError(ErrNotFound)
	mock.ExpectExec("INSERT INTO .*users.*").
		WillReturnResult(sqlmock.NewResult(1, 1))

	created, err := svc.Register(ctx, RegisterInput{
		Email:       " Alice@Example.com ",
		Password:    "Abcd1234",
		DisplayName: "  Alice  ",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if created.Email != "alice@example.com" || created.DisplayName != "Alice" {
		t.Fatalf("unexpected created user: %+v", created)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(userRows(models.User{
			ID:           1,
			Email:        created.Email,
			PasswordHash: created.PasswordHash,
			DisplayName:  created.DisplayName,
		}))

	authenticated, err := svc.Authenticate(ctx, LoginInput{
		Email:    "ALICE@example.com",
		Password: "Abcd1234",
	})
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if authenticated.Email != created.Email {
		t.Fatalf("unexpected authenticated user: %+v", authenticated)
	}

	searchRows := sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
		AddRow(1, "alice@example.com", created.PasswordHash, "Alice", "", time.Now(), time.Now(), nil).
		AddRow(2, "bob@example.com", created.PasswordHash, "Bob", "", time.Now(), time.Now(), nil)
	mock.ExpectQuery("SELECT .*FROM .*users.*LIKE.*ORDER BY .*created_at.*LIMIT").
		WillReturnRows(searchRows)

	users, err := svc.SearchByEmail(ctx, "example", 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("unexpected search result count: %d", len(users))
	}

	seenAt := time.Date(2026, time.April, 10, 10, 0, 0, 0, time.Local)
	mock.ExpectExec("UPDATE .*users.*last_seen.*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := svc.UpdateLastSeen(ctx, 1, &seenAt); err != nil {
		t.Fatalf("update last seen failed: %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
		WillReturnRows(userRows(models.User{
			ID:           1,
			Email:        created.Email,
			PasswordHash: created.PasswordHash,
			DisplayName:  created.DisplayName,
		}))
	mock.ExpectExec("UPDATE .*users.*password_hash.*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := svc.ChangePassword(ctx, 1, ChangePasswordInput{
		OldPassword:     "Abcd1234",
		NewPassword:     "Newpass1",
		ConfirmPassword: "Newpass1",
	}); err != nil {
		t.Fatalf("change password failed: %v", err)
	}

	mock.ExpectExec("UPDATE .*users.*fcm_token.*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := svc.SaveFCMToken(ctx, 1, " fcm-token-1 "); err != nil {
		t.Fatalf("save fcm token failed: %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
		WillReturnRows(userRows(models.User{
			ID:           1,
			Email:        created.Email,
			PasswordHash: created.PasswordHash,
			DisplayName:  created.DisplayName,
			FCMToken:     "fcm-token-1",
		}))
	gotToken, err := svc.GetFCMToken(ctx, 1)
	if err != nil {
		t.Fatalf("get fcm token failed: %v", err)
	}
	if gotToken != "fcm-token-1" {
		t.Fatalf("unexpected token: %q", gotToken)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestServiceErrorPaths(t *testing.T) {
	svc, mock := newMockService(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(userRows(models.User{
			ID:           1,
			Email:        "alice@example.com",
			PasswordHash: "existing-hash",
			DisplayName:  "Alice",
		}))

	if _, err := svc.Register(ctx, RegisterInput{
		Email:       "alice@example.com",
		Password:    "Abcd1234",
		DisplayName: "Alice",
	}); !errors.Is(err, ErrEmailAlreadyUsed) {
		t.Fatalf("expected duplicate email error, got %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnError(ErrNotFound)
	if _, err := svc.Authenticate(ctx, LoginInput{
		Email:    "missing@example.com",
		Password: "Abcd1234",
	}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for missing user, got %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(userRows(models.User{
			ID:           1,
			Email:        "alice@example.com",
			PasswordHash: "existing-hash",
			DisplayName:  "Alice",
		}))
	if _, err := svc.Authenticate(ctx, LoginInput{
		Email:    "alice@example.com",
		Password: "wrong-password",
	}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for bad password, got %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnError(errors.New("query failed"))
	if _, err := svc.Register(ctx, RegisterInput{
		Email:       "new@example.com",
		Password:    "Abcd1234",
		DisplayName: "New",
	}); err == nil || errors.Is(err, ErrEmailAlreadyUsed) {
		t.Fatalf("expected repository error on register, got %v", err)
	}

	if err := svc.SaveFCMToken(ctx, 1, " "); err == nil {
		t.Fatal("expected empty token error")
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
		WillReturnRows(userRows(models.User{
			ID:           1,
			Email:        "alice@example.com",
			PasswordHash: "existing-hash",
			DisplayName:  "Alice",
		}))
	if err := svc.ChangePassword(ctx, 1, ChangePasswordInput{
		OldPassword:     "wrong-old",
		NewPassword:     "Newpass1",
		ConfirmPassword: "Newpass1",
	}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for wrong old password, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestServiceAdditionalBranches(t *testing.T) {
	svc, mock := newMockService(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
		WillReturnError(ErrNotFound)
	if _, err := svc.GetByID(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnError(ErrNotFound)
	if _, err := svc.GetByEmail(ctx, "missing@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
			AddRow(1, "alice@example.com", "hash", "Alice", "", time.Now(), time.Now(), nil))
	if users, err := svc.SearchByEmail(ctx, "Alice", 0); err != nil || len(users) != 1 {
		t.Fatalf("unexpected search result: users=%d err=%v", len(users), err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
		WillReturnError(ErrNotFound)
	if err := svc.ChangePassword(ctx, 1, ChangePasswordInput{
		OldPassword:     "Abcd1234",
		NewPassword:     "Abcd1234",
		ConfirmPassword: "Abcd1234",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
		WillReturnRows(userRows(models.User{
			ID:           1,
			Email:        "alice@example.com",
			PasswordHash: mustPasswordHash(t, "Abcd1234"),
			DisplayName:  "Alice",
		}))
	if err := svc.ChangePassword(ctx, 1, ChangePasswordInput{
		OldPassword:     "Abcd1234",
		NewPassword:     "Abcd1234!",
		ConfirmPassword: "Abcd1234!",
	}); err == nil {
		t.Fatal("expected password validation error")
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
		WillReturnRows(userRows(models.User{
			ID:           1,
			Email:        "alice@example.com",
			PasswordHash: mustPasswordHash(t, "Abcd1234"),
			DisplayName:  "Alice",
		}))
	mock.ExpectExec("UPDATE .*users.*password_hash.*").
		WillReturnError(errors.New("update failed"))
	if err := svc.ChangePassword(ctx, 1, ChangePasswordInput{
		OldPassword:     "Abcd1234",
		NewPassword:     "Newpass1",
		ConfirmPassword: "Newpass1",
	}); err == nil {
		t.Fatal("expected password update error")
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*id = .*").
		WillReturnError(ErrNotFound)
	if _, err := svc.GetFCMToken(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
