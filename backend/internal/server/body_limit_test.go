package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/handlers"
)

const bodyLimitTestMB = 1024 * 1024

// newBodyLimitTestEngine 复用 routes_test.go 的最小依赖注册手法：
// RegisterRoutes 会无条件调用 auth/email handler，因此必须显式构造零值版本；
// 其余依赖保持 nil，只请求会在触碰它们之前就失败的路由。
// newBodyLimitTestEngine builds an engine the same way routes_test.go does:
// minimal deps, routes registered on top of NewEngine's middleware chain.
func newBodyLimitTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	deps := RouteDependencies{
		ReadinessChecks: map[string]ReadinessCheck{
			"mysql": func(context.Context) error { return nil },
		},
		AuthHandler:  handlers.NewAuthHandler(zerolog.Nop(), nil, nil, nil),
		EmailHandler: handlers.NewEmailHandler(zerolog.Nop(), nil),
	}
	engine := NewEngine(zerolog.Nop())
	RegisterRoutes(engine, deps)
	return engine
}

// P0-1: 声明了超限 Content-Length 的请求必须在读取 body 之前直接返回 413 JSON。
// 目前登录 handler 会读完整个 body 并回 400 —— 这就是本用例要锁定的 RED 失败。
// Oversized Content-Length must be rejected with 413 JSON before the handler runs.
func TestBodyLimitRejectsOversizedContentLengthBeforeHandler(t *testing.T) {
	engine := newBodyLimitTestEngine(t)

	payload := bytes.NewReader(bytes.Repeat([]byte("a"), 5*bodyLimitTestMB))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", payload)
	req.Header.Set("Content-Type", "application/json")
	if req.ContentLength != 5*bodyLimitTestMB {
		t.Fatalf("test setup: expected Content-Length %d, got %d", int64(5*bodyLimitTestMB), req.ContentLength)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("5 MB POST with Content-Length to /api/v1/auth/login: expected 413, got %d body=%s",
			rec.Code, rec.Body.String())
	}
	var body struct {
		Error   string `json:"error"`
		Code    string `json:"code"`
		Success *bool  `json:"success"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("413 response must be JSON: %v body=%s", err, rec.Body.String())
	}
	if body.Error == "" || body.Code != "PAYLOAD_TOO_LARGE" || body.Success == nil || *body.Success {
		t.Fatalf("413 body must match the repo error JSON shape {error, code, success:false}, got %s",
			rec.Body.String())
	}
}

// 无 Content-Length 的分块请求只能靠 http.MaxBytesReader 兜底：
// handler 侧的 bind 错误必须能 errors.As 出 *http.MaxBytesError。
// Chunked bodies over the limit must be bounded by http.MaxBytesReader.
func TestBodyLimitBoundsChunkedBodyWithMaxBytesReader(t *testing.T) {
	engine := newBodyLimitTestEngine(t)

	var bindErr error
	engine.POST("/__bodylimit/chunked", func(c *gin.Context) {
		var req struct {
			Padding string `json:"padding"`
		}
		bindErr = c.ShouldBindJSON(&req)
		if bindErr != nil {
			handlers.JSONError(c, http.StatusBadRequest, "invalid body")
			return
		}
		c.Status(http.StatusOK)
	})

	body := `{"padding":"` + strings.Repeat("a", 5*bodyLimitTestMB) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/__bodylimit/chunked", strings.NewReader(body))
	// 模拟 chunked：没有 Content-Length，中间件无法走预检分支。
	req.ContentLength = -1
	req.TransferEncoding = []string{"chunked"}
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code >= 200 && rec.Code < 300 {
		t.Fatalf("chunked 5 MB body must not succeed against the 4 MB default limit, got %d", rec.Code)
	}
	var maxErr *http.MaxBytesError
	if !errors.As(bindErr, &maxErr) {
		t.Fatalf("handler bind error must satisfy errors.As(err, *http.MaxBytesError), got %v", bindErr)
	}
}

// 回归护栏：刚好低于上限的请求必须照常到达 handler（400/401/2xx，绝不能是 413）。
// Requests just under the limit must still reach the handler.
func TestBodyLimitPassesUnderLimitRequestToHandler(t *testing.T) {
	engine := newBodyLimitTestEngine(t)

	payload := bytes.NewReader(bytes.Repeat([]byte("a"), 4*bodyLimitTestMB-1024))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", payload)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("request under the 4 MB default limit must reach the handler, got 413 body=%s",
			rec.Body.String())
	}
	switch rec.Code {
	case http.StatusOK, http.StatusBadRequest, http.StatusUnauthorized:
		// handler reached: invalid JSON binds to 400.
	default:
		t.Fatalf("expected the request to reach the login handler (2xx/400/401), got %d body=%s",
			rec.Code, rec.Body.String())
	}
}

// /api/v1/knowledge/sources 走 7 MB 路径上限（handler 自身 6 MB 的 MaxBytesReader
// 通过嵌套取 min 继续生效，见 knowledge_sources.go），4 MB 默认值不得拦它。
// The knowledge sources path must use the 7 MB limit, not the 4 MB default.
func TestBodyLimitKnowledgeSourcesPathUsesSevenMegabyteLimit(t *testing.T) {
	engine := newBodyLimitTestEngine(t)

	var read int64
	engine.POST("/api/v1/knowledge/sources", func(c *gin.Context) {
		n, err := io.Copy(io.Discard, c.Request.Body)
		read = n
		if err != nil {
			handlers.JSONError(c, http.StatusBadRequest, err.Error())
			return
		}
		c.Status(http.StatusOK)
	})

	t.Run("5.5MB passes", func(t *testing.T) {
		read = 0
		size := 5*bodyLimitTestMB + bodyLimitTestMB/2
		payload := bytes.NewReader(bytes.Repeat([]byte("k"), size))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/sources", payload)

		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		if rec.Code == http.StatusRequestEntityTooLarge {
			t.Fatalf("knowledge path must use the 7 MB limit instead of the 4 MB default, got 413 body=%s",
				rec.Body.String())
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected the handler to read the 5.5 MB body, got %d body=%s", rec.Code, rec.Body.String())
		}
		if read != int64(size) {
			t.Fatalf("handler should read the whole body, read %d bytes", read)
		}
	})

	t.Run("8MB rejected", func(t *testing.T) {
		payload := bytes.NewReader(bytes.Repeat([]byte("k"), 8*bodyLimitTestMB))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/sources", payload)

		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("8 MB POST must exceed the 7 MB knowledge limit, got %d body=%s",
				rec.Code, rec.Body.String())
		}
	})
}

// 会话附件上传（collaboration_handler.go 注册的 POST /conversations/:id/attachments）
// 是 multipart 大文件路径，上限 26 MB；4 MB 默认值不得拦 5 MB 附件。
// The conversation attachments path must allow uploads up to 26 MB.
func TestBodyLimitConversationAttachmentsPathAllows26Megabytes(t *testing.T) {
	engine := newBodyLimitTestEngine(t)

	var read int64
	engine.POST("/api/v1/conversations/:id/attachments", func(c *gin.Context) {
		n, err := io.Copy(io.Discard, c.Request.Body)
		read = n
		if err != nil {
			handlers.JSONError(c, http.StatusBadRequest, err.Error())
			return
		}
		c.Status(http.StatusOK)
	})

	t.Run("5MB passes", func(t *testing.T) {
		read = 0
		payload := bytes.NewReader(bytes.Repeat([]byte("f"), 5*bodyLimitTestMB))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/42/attachments", payload)

		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		if rec.Code == http.StatusRequestEntityTooLarge {
			t.Fatalf("attachments path must allow 5 MB under the 26 MB limit, got 413 body=%s",
				rec.Body.String())
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected the handler to read the 5 MB body, got %d body=%s", rec.Code, rec.Body.String())
		}
		if read != 5*bodyLimitTestMB {
			t.Fatalf("handler should read the whole body, read %d bytes", read)
		}
	})

	t.Run("27MB rejected", func(t *testing.T) {
		payload := bytes.NewReader(bytes.Repeat([]byte("f"), 27*bodyLimitTestMB))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/42/attachments", payload)

		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("27 MB POST must exceed the 26 MB attachments limit, got %d body=%s",
				rec.Code, rec.Body.String())
		}
	})
}
