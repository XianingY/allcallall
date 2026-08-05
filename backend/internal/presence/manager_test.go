package presence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/user"
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

func TestHeartbeatMarksOnlineAndTracksDevices(t *testing.T) {
	mgr, _, _, cleanup := newPresenceTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	if err := mgr.Heartbeat(ctx, "carol@example.com", "web", "web"); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}
	if err := mgr.Heartbeat(ctx, "carol@example.com", "mobile", "ios"); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}
	st, err := mgr.GetStatus(ctx, "carol@example.com")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if st.State != StateOnline || !st.Online {
		t.Fatalf("expected online, got %s", st.State)
	}
	if len(st.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(st.Devices))
	}
}

func TestManualStateOverridesHeartbeatAndClears(t *testing.T) {
	mgr, _, _, cleanup := newPresenceTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	_ = mgr.Heartbeat(ctx, "dave@example.com", "web", "web")
	if st, _ := mgr.GetStatus(ctx, "dave@example.com"); st.State != StateOnline {
		t.Fatalf("expected online before manual state, got %s", st.State)
	}

	if err := mgr.SetManualState(ctx, "dave@example.com", StateBusy, "in a meeting"); err != nil {
		t.Fatalf("set manual state failed: %v", err)
	}
	st, _ := mgr.GetStatus(ctx, "dave@example.com")
	if st.State != StateBusy || !st.Online {
		t.Fatalf("expected busy (still online), got %s online=%v", st.State, st.Online)
	}
	if st.CustomMessage != "in a meeting" {
		t.Fatalf("custom message not persisted: %q", st.CustomMessage)
	}

	// A heartbeat while busy must NOT flip the user back to online.
	_ = mgr.Heartbeat(ctx, "dave@example.com", "web", "web")
	if st, _ := mgr.GetStatus(ctx, "dave@example.com"); st.State != StateBusy {
		t.Fatalf("busy should override device heartbeat, got %s", st.State)
	}

	if err := mgr.ClearManualState(ctx, "dave@example.com"); err != nil {
		t.Fatalf("clear manual state failed: %v", err)
	}
	if st, _ := mgr.GetStatus(ctx, "dave@example.com"); st.State != StateOnline {
		t.Fatalf("expected online after clearing manual state, got %s", st.State)
	}
}

func TestSetOfflineClearsDevices(t *testing.T) {
	mgr, mock, mini, cleanup := newPresenceTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	_ = mgr.Heartbeat(ctx, "erin@example.com", "web", "web")
	_ = mgr.Heartbeat(ctx, "erin@example.com", "mobile", "ios")
	if st, _ := mgr.GetStatus(ctx, "erin@example.com"); len(st.Devices) != 2 {
		t.Fatalf("expected 2 devices before offline, got %d", len(st.Devices))
	}

	mock.ExpectQuery("SELECT .*FROM .*users.*LOWER\\(email\\).*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "display_name", "fcm_token", "created_at", "updated_at", "last_seen"}).
			AddRow(1, "erin@example.com", "hash", "Erin", "", time.Now(), time.Now(), nil))
	mock.ExpectExec("UPDATE .*users.*last_seen.*").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := mgr.SetOffline(ctx, "erin@example.com"); err != nil {
		t.Fatalf("set offline failed: %v", err)
	}
	st, _ := mgr.GetStatus(ctx, "erin@example.com")
	if st.State != StateOffline || st.Online {
		t.Fatalf("expected offline, got %s", st.State)
	}
	if len(st.Devices) != 0 {
		t.Fatalf("devices should be cleared on offline, got %d", len(st.Devices))
	}
	_ = mini // devices keys removed
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestPublishEventOnChange(t *testing.T) {
	mgr, _, mini, cleanup := newPresenceTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	sub := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer sub.Close()
	ch := sub.Subscribe(ctx, eventsChannel)
	defer ch.Close()
	msgCh := ch.Channel()

	// Trigger a state change; the manager must broadcast it.
	if err := mgr.Heartbeat(ctx, "frank@example.com", "web", "web"); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}

	select {
	case msg := <-msgCh:
		var ev PresenceEvent
		if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
			t.Fatalf("decode event failed: %v", err)
		}
		if ev.Email != "frank@example.com" || ev.State != StateOnline {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no presence event published within timeout")
	}
}

func TestGetStatusBackwardCompatible(t *testing.T) {
	mgr, _, mini, cleanup := newPresenceTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	mini.Set("presence:user:legacy@example.com", `{"email":"legacy@example.com","online":true,"last_seen":"2026-04-10T00:00:00Z"}`)
	st, err := mgr.GetStatus(ctx, "legacy@example.com")
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if st.State != StateOnline || !st.Online {
		t.Fatalf("legacy online json should normalize to online, got %s", st.State)
	}
}
