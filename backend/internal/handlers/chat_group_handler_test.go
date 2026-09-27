package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

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

// chatTestEnv 组织了两个租户：orgA（u1 owner、u2 member）与 orgB（u2 owner，
// u1 不是成员），用于验证 chat 路由的跨组织写入防护。
type chatTestEnv struct {
	router *gin.Engine
	db     *gorm.DB
	pub    *chatFakePublisher
	orgA   uint64
	orgB   uint64
	u1     uint64
	u2     uint64
}

func newChatTestRouter(t *testing.T) *chatTestEnv {
	t.Helper()
	db := testutil.OpenSQLite(t, "chat_handler_test.db")
	testutil.AutoMigrateAll(t, db)
	u1 := testutil.SeedUser(t, db, models.User{Email: "a@x.com"}).ID
	u2 := testutil.SeedUser(t, db, models.User{Email: "b@x.com"}).ID
	orgA := testutil.SeedOrganization(t, db, models.Organization{Name: "Org"}, u1).ID
	orgB := testutil.SeedOrganization(t, db, models.Organization{Name: "Org B", Slug: "org-b"}, u2).ID
	// u2 同时是 orgA 的普通成员，保证既有用例里 u2 的读写仍在其组织内。
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: orgA,
		UserID:         u2,
		Role:           models.OrganizationRoleMember,
		JoinedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("seed orgA membership for u2: %v", err)
	}
	pub := &chatFakePublisher{}
	svc := chat.NewService(db, pub).WithLogger(zerolog.Nop())
	orgs := collaboration.NewService(db, nil)
	h := NewChatHandler(zerolog.Nop(), svc, orgs, nil)

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
	return &chatTestEnv{router: r, db: db, pub: pub, orgA: orgA, orgB: orgB, u1: u1, u2: u2}
}

func TestChatHandlerFlow(t *testing.T) {
	env := newChatTestRouter(t)
	r, pub, org, u2 := env.router, env.pub, env.orgA, env.u2

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

	// 请求一个与自己无关的组织应被拒绝（而非 200 返回空列表）
	req = httptest.NewRequest(http.MethodGet, "/api/v1/chat/groups?org_id=999999", nil)
	req.Header.Set("X-Test-User", "u1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("list groups for foreign org: expected 403, got %d %s", w.Code, w.Body.String())
	}
}

// TestChatHandlerCreateGroupRejectsNonMemberOrg 覆盖跨租户写入：u1 不是 orgB 的
// 成员，带着 ?org_id=orgB 建群必须被拒绝，且 orgB 不能落库任何群组。
func TestChatHandlerCreateGroupRejectsNonMemberOrg(t *testing.T) {
	env := newChatTestRouter(t)

	body, _ := json.Marshal(map[string]any{"name": "Intruder"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/groups?org_id="+u64str(env.orgB), bytes.NewReader(body))
	req.Header.Set("X-Test-User", "u1")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-member create group: expected 403, got %d %s", w.Code, w.Body.String())
	}
	var errResp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errResp.Code != "ORGANIZATION_ACCESS_DENIED" {
		t.Fatalf("expected code ORGANIZATION_ACCESS_DENIED, got %q (body: %s)", errResp.Code, w.Body.String())
	}
	var count int64
	if err := env.db.Model(&models.ChatGroup{}).Where("organization_id = ?", env.orgB).Count(&count).Error; err != nil {
		t.Fatalf("count orgB groups: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no groups created in foreign org, got %d", count)
	}
}

// TestChatHandlerCreateGroupAllowsOrgMember 覆盖合法路径：orgA 成员建群仍应 201。
func TestChatHandlerCreateGroupAllowsOrgMember(t *testing.T) {
	env := newChatTestRouter(t)

	body, _ := json.Marshal(map[string]any{"name": "Team"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/groups?org_id="+u64str(env.orgA), bytes.NewReader(body))
	req.Header.Set("X-Test-User", "u1")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("member create group: expected 201, got %d %s", w.Code, w.Body.String())
	}
	var grp struct {
		Group chat.GroupView `json:"group"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &grp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if grp.Group.Group.ID == 0 {
		t.Fatalf("expected group id in response, body: %s", w.Body.String())
	}
	if grp.Group.Group.OrganizationID != env.orgA {
		t.Fatalf("expected group in orgA (%d), got %d", env.orgA, grp.Group.Group.OrganizationID)
	}
}

// TestChatHandlerMarkReadRejectsMalformedJSON 覆盖被吞掉的 bind 错误：
// POST /chat/groups/:id/read 收到非法 JSON 必须 400，而不是静默 up_to_message_id=0。
func TestChatHandlerMarkReadRejectsMalformedJSON(t *testing.T) {
	env := newChatTestRouter(t)

	body, _ := json.Marshal(map[string]any{"name": "Reader"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/groups?org_id="+u64str(env.orgA), bytes.NewReader(body))
	req.Header.Set("X-Test-User", "u1")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
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

	req = httptest.NewRequest(http.MethodPost,
		"/api/v1/chat/groups/"+u64str(gid)+"/read?org_id="+u64str(env.orgA),
		strings.NewReader(`{"up_to_message_id":`))
	req.Header.Set("X-Test-User", "u1")
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("mark read with malformed JSON: expected 400, got %d %s", w.Code, w.Body.String())
	}
}

// TestChatHandlerRejectsNonMemberOnAllRoutes 逐条覆盖所有从 query 读取 org_id 的
// 聊天路由：非成员访问必须统一 403 ORGANIZATION_ACCESS_DENIED。
func TestChatHandlerRejectsNonMemberOnAllRoutes(t *testing.T) {
	env := newChatTestRouter(t)

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/chat/groups", `{"name":"x"}`},
		{http.MethodGet, "/chat/groups", ""},
		{http.MethodGet, "/chat/groups/1", ""},
		{http.MethodPost, "/chat/groups/1/members", `{"user_id":1}`},
		{http.MethodDelete, "/chat/groups/1/members/1", ""},
		{http.MethodPost, "/chat/groups/1/messages", `{"type":"text","body":"x"}`},
		{http.MethodGet, "/chat/groups/1/messages", ""},
		{http.MethodPatch, "/chat/groups/1/messages/1", `{"body":"x"}`},
		{http.MethodDelete, "/chat/groups/1/messages/1", ""},
		{http.MethodPost, "/chat/groups/1/read", `{"up_to_message_id":1}`},
		{http.MethodGet, "/chat/groups/1/messages/1/receipts", ""},
		{http.MethodGet, "/chat/groups/1/read-summary", ""},
	}
	for _, rt := range routes {
		var rd *strings.Reader
		if rt.body != "" {
			rd = strings.NewReader(rt.body)
		} else {
			rd = strings.NewReader("")
		}
		req := httptest.NewRequest(rt.method, "/api/v1"+rt.path+"?org_id="+u64str(env.orgB), rd)
		req.Header.Set("X-Test-User", "u1")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 for non-member org, got %d %s", rt.method, rt.path, w.Code, w.Body.String())
			continue
		}
		var errResp struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Errorf("%s %s: decode error body: %v", rt.method, rt.path, err)
			continue
		}
		if errResp.Code != "ORGANIZATION_ACCESS_DENIED" {
			t.Errorf("%s %s: expected code ORGANIZATION_ACCESS_DENIED, got %q", rt.method, rt.path, errResp.Code)
		}
	}
}

func u64str(u uint64) string {
	return strconv.FormatUint(u, 10)
}
