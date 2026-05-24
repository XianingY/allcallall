package presence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/allcallall/backend/internal/user"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newPresenceTestEnv(t *testing.T) (*Manager, sqlmock.Sqlmock, *miniredis.Miniredis, func()) {
	t.Helper()

	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis failed: %v", err)
	}

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		mini.Close()
		t.Fatalf("create sqlmock failed: %v", err)
	}

	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = db.Close()
		mini.Close()
		t.Fatalf("open gorm db failed: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	svc := user.NewService(user.NewRepository(gdb))
	mgr := NewManager(client, zerolog.Nop(), svc)
	return mgr, mock, mini, func() {
		_ = db.Close()
		mini.Close()
	}
}

func TestManager(t *testing.T) {
	mgr, mock, mini, cleanup := newPresenceTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	if err := mgr.SetOnline(ctx, "alice@example.com"); err != nil {
		t.Fatalf("set online failed: %v", err)
	}
	if got, err := mgr.GetStatus(ctx, "alice@example.com"); err != nil || !got.Online {
		t.Fatalf("unexpected online status: %+v err=%v", got, err)
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
			AddRow(1, "alice@example.com", "hash", "Alice", "", time.Now(), time.Now(), nil))
	mock.ExpectExec("UPDATE .*users.*last_seen.*").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := mgr.SetOffline(ctx, "alice@example.com"); err != nil {
		t.Fatalf("set offline failed: %v", err)
	}

	if err := mgr.UpdateLastSeen(ctx, "alice@example.com"); err != nil {
		t.Fatalf("update last seen failed: %v", err)
	}

	mini.Set("presence:user:bob@example.com", `{"email":"bob@example.com","online":true,"last_seen":"2026-04-10T00:00:00Z"}`)
	statuses, err := mgr.GetStatuses(ctx, []string{"alice@example.com", "bob@example.com"})
	if err != nil {
		t.Fatalf("get statuses failed: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("unexpected statuses size: %d", len(statuses))
	}

	if _, err := json.Marshal(statuses["bob@example.com"]); err != nil {
		t.Fatalf("marshal status failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
