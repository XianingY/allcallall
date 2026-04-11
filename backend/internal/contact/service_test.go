package contact

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/allcallall/backend/internal/user"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newContactTestEnv(t *testing.T) (*Service, sqlmock.Sqlmock, func()) {
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

	uSvc := user.NewService(user.NewRepository(gdb))
	svc := NewService(NewRepository(gdb), uSvc)
	return svc, mock, func() { _ = db.Close() }
}

func TestRepository(t *testing.T) {
	svc, mock, cleanup := newContactTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO .*contacts.*").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := svc.repo.AddContact(ctx, 1, 2); err != nil {
		t.Fatalf("add contact failed: %v", err)
	}

	mock.ExpectExec("DELETE FROM .*contacts.*owner_id = .*contact_id = .*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := svc.repo.RemoveContact(ctx, 1, 2); err != nil {
		t.Fatalf("remove contact failed: %v", err)
	}

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM .*contacts.*owner_id = .*contact_id = .*").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	exists, err := svc.repo.ContactExists(ctx, 1, 2)
	if err != nil || !exists {
		t.Fatalf("unexpected exists result: exists=%v err=%v", exists, err)
	}

	mock.ExpectQuery("SELECT .*FROM .*contacts.*JOIN users ON contacts.contact_id = users.id.*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
			AddRow(2, "bob@example.com", "hash", "Bob", "", time.Now(), time.Now(), nil))
	users, err := svc.repo.ListContacts(ctx, 1)
	if err != nil || len(users) != 1 {
		t.Fatalf("unexpected list result: users=%d err=%v", len(users), err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestService(t *testing.T) {
	svc, mock, cleanup := newContactTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
			AddRow(2, "bob@example.com", "hash", "Bob", "", time.Now(), time.Now(), nil))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM .*contacts.*owner_id = .*contact_id = .*").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO .*contacts.*").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := svc.AddByEmail(ctx, 1, "alice@example.com", "bob@example.com"); err != nil {
		t.Fatalf("add by email failed: %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
			AddRow(1, "alice@example.com", "hash", "Alice", "", time.Now(), time.Now(), nil))
	if err := svc.AddByEmail(ctx, 1, "alice@example.com", "alice@example.com"); !errors.Is(err, ErrSelfContact) {
		t.Fatalf("expected self contact error, got %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
			AddRow(2, "bob@example.com", "hash", "Bob", "", time.Now(), time.Now(), nil))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM .*contacts.*owner_id = .*contact_id = .*").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	if err := svc.AddByEmail(ctx, 1, "alice@example.com", "bob@example.com"); !errors.Is(err, ErrContactExists) {
		t.Fatalf("expected duplicate contact error, got %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnError(user.ErrNotFound)
	if err := svc.AddByEmail(ctx, 1, "alice@example.com", "missing@example.com"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("expected user not found error, got %v", err)
	}

	mock.ExpectExec("DELETE FROM .*contacts.*owner_id = .*contact_id = .*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := svc.Remove(ctx, 1, 2); err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	mock.ExpectQuery("SELECT .*FROM .*contacts.*JOIN users ON contacts.contact_id = users.id.*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
			AddRow(2, "bob@example.com", "hash", "Bob", "", time.Now(), time.Now(), nil))
	users, err := svc.List(ctx, 1)
	if err != nil || len(users) != 1 {
		t.Fatalf("unexpected list result: users=%d err=%v", len(users), err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
