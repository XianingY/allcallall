package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/tasksched"
	"github.com/allcallall/backend/internal/testutil"
)

func stubAuth(ownerID uint64) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: ownerID})
		c.Next()
	}
}

func newTaskHandlerTestRouter(t *testing.T) (*gin.Engine, *tasksched.Service) {
	t.Helper()
	db := testutil.OpenSQLite(t, "task_handler")
	testutil.AutoMigrateAll(t, db)
	svc := tasksched.NewService(db)
	h := NewTaskSchedulerHandler(zerolog.Nop(), svc, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/api/v1")
	grp.Use(stubAuth(1))
	h.RegisterRoutes(grp)
	return r, svc
}

func TestTaskHandlerCreateAndList(t *testing.T) {
	r, _ := newTaskHandlerTestRouter(t)

	body, _ := json.Marshal(map[string]any{
		"title":           "weekly sync",
		"weekdays":        []int{1, 3, 5},
		"run_time_of_day": "09:30",
		"timezone":        "UTC",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", w.Code, w.Body.String())
	}
	var created struct {
		Task struct {
			ID uint64 `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Task.ID == 0 {
		t.Fatalf("expected non-zero task id")
	}

	// 列表应包含刚创建的任务
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	var listed struct {
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(listed.Tasks))
	}
}

func TestTaskHandlerPauseFlow(t *testing.T) {
	r, _ := newTaskHandlerTestRouter(t)

	body, _ := json.Marshal(map[string]any{"title": "t", "weekdays": []int{1}, "run_time_of_day": "09:00"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var created struct {
		Task struct {
			ID uint64 `json:"id"`
		} `json:"task"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	id := created.Task.ID

	// pause
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+itoa(id)+"/pause", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pause status = %d, body=%s", w.Code, w.Body.String())
	}

	// get -> paused
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+itoa(id), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d", w.Code)
	}
	var got struct {
		Task struct {
			Status string `json:"status"`
		} `json:"task"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Task.Status != "paused" {
		t.Fatalf("expected paused, got %s", got.Task.Status)
	}
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
