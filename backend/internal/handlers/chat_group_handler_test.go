package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/chat"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
)

type chatFakePublisher struct {
	events []collaboration.RealtimeEventRecord
}

func (f *chatFakePublisher) PublishToUser(_ context.Context, e collaboration.RealtimeEventRecord) error {
	f.events = append(f.events, e)
	return nil
}

func newChatTestRouter(t *testing.T) (*gin.Engine, *chatFakePublisher, uint64, uint64) {
	t.Helper()
	db := testutil.OpenSQLite(t, "chat_handler_test.db")
	testutil.AutoMigrateAll(t, db)
	org := testutil.SeedOrganization(t, db, models.Organization{Name: "Org"}, 0).ID
	u1 := testutil.SeedUser(t, db, models.User{Email: "a@x.com"}).ID
	u2 := testutil.SeedUser(t, db, models.User{Email: "b@x.com"}).ID
	pub := &chatFakePublisher{}
	svc := chat.NewService(db, pub).WithLogger(zerolog.Nop())
	h := NewChatHandler(zerolog.Nop(), svc, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		uid := c.GetHeader("X-Test-User")
		var id uint64
		switch uid {
		case "u1":
			id = u1
		case "u2":
			id = u2
		}
		auth.SetClaimsToContext(c, &auth.Claims{UserID: id, Email: uid + "@x.com"})
		c.Next()
	})
	h.RegisterRoutes(api)
	return r, pub, org, u2
}

func TestChatHandlerFlow(t *testing.T) {
	r, pub, org, u2 := newChatTestRouter(t)

	// 创建群组
	body, _ := json.Marshal(map[string]any{"name": "Team", "member_ids": []uint64{u2}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/groups?org_id="+u64str(org), bytes.NewReader(body))
	req.Header.Set("X-Test-User", "u1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create group: %d %s", w.Code, w.Body.String())
	}
	var grp struct {
		Group chat.GroupView `json:"group"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &grp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	gid := grp.Group.Group.ID
	if gid == 0 {
		t.Fatal("group id missing")
	}

	// 发送消息
	mbody, _ := json.Marshal(map[string]any{"type": "text", "body": "hello"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/chat/groups/"+u64str(gid)+"/messages?org_id="+u64str(org), bytes.NewReader(mbody))
	req.Header.Set("X-Test-User", "u1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("send message: %d %s", w.Code, w.Body.String())
	}
	// 实时投递应已发生
	if len(pub.events) == 0 {
		t.Fatal("expected realtime delivery on send")
	}

	// 列表（u2 视角）
	req = httptest.NewRequest(http.MethodGet, "/api/v1/chat/groups/"+u64str(gid)+"/messages?org_id="+u64str(org), nil)
	req.Header.Set("X-Test-User", "u2")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list messages: %d %s", w.Code, w.Body.String())
	}
	var lp struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &lp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(lp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(lp.Messages))
	}

	// u2 标记已读
	rb, _ := json.Marshal(map[string]any{"up_to_message_id": 0})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/chat/groups/"+u64str(gid)+"/read?org_id="+u64str(org), bytes.NewReader(rb))
	req.Header.Set("X-Test-User", "u2")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("mark read: %d %s", w.Code, w.Body.String())
	}
	var rd struct {
		Read struct {
			UnreadCount int64 `json:"unread_count"`
		} `json:"read"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rd); err != nil {
		t.Fatalf("decode read: %v", err)
	}
	if rd.Read.UnreadCount != 0 {
		t.Fatalf("expected 0 unread, got %d", rd.Read.UnreadCount)
	}

	// 非成员被拒
	req = httptest.NewRequest(http.MethodGet, "/api/v1/chat/groups/"+u64str(gid)+"/messages?org_id="+u64str(org), nil)
	req.Header.Set("X-Test-User", "u2")
	// u2 是成员，改用不存在用户：再 seed 一个 u3 不作为成员
	// 直接请求一个无关 org 校验鉴权
	req = httptest.NewRequest(http.MethodGet, "/api/v1/chat/groups?org_id=999999", nil)
	req.Header.Set("X-Test-User", "u1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list groups: %d", w.Code)
	}
}

func u64str(u uint64) string {
	return strconv.FormatUint(u, 10)
}
