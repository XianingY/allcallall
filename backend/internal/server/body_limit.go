package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/handlers"
)

// 请求体大小上限（字节）。
// Body size caps in bytes. P0-1 (unauthenticated memory-exhaustion DoS):
// without a global cap an attacker can stream an unbounded body into memory.
const (
	// defaultBodyLimitBytes 4 MB：覆盖所有 JSON/API 端点。
	defaultBodyLimitBytes int64 = 4 << 20
	// knowledgeSourcesBodyLimitBytes 7 MB：知识源导入允许更大的 JSON/元数据；
	// knowledge_sources.go 里 handler 自己的 6 MB MaxBytesReader 通过嵌套取 min 继续生效。
	knowledgeSourcesBodyLimitBytes int64 = 7 << 20
	// conversationAttachmentBodyLimitBytes 26 MB：会话附件是 multipart 大文件上传
	// （POST /api/v1/conversations/:id/attachments，见 collaboration_handler.go）。
	conversationAttachmentBodyLimitBytes int64 = 26 << 20
)

// bodyLimitRule 按路径覆盖默认上限。prefix 必须命中；suffix 非空时还要求
// 路径以其结尾（附件路由的 :id 是动态段，无法用纯前缀表达）。
// bodyLimitRule overrides the default cap for a path prefix (optionally
// anchored by a suffix for parameterized routes).
type bodyLimitRule struct {
	prefix string
	suffix string
	limit  int64
}

var bodyLimitRules = []bodyLimitRule{
	{prefix: "/api/v1/knowledge/sources", limit: knowledgeSourcesBodyLimitBytes},
	{prefix: "/api/v1/conversations/", suffix: "/attachments", limit: conversationAttachmentBodyLimitBytes},
}

// bodyLimitForPath 返回该路径生效的上限：先取默认值，再按规则表覆盖。
// bodyLimitForPath resolves the effective body limit for a request path.
func bodyLimitForPath(path string, defaultLimit int64) int64 {
	limit := defaultLimit
	for _, rule := range bodyLimitRules {
		if !strings.HasPrefix(path, rule.prefix) {
			continue
		}
		if rule.suffix != "" && !strings.HasSuffix(path, rule.suffix) {
			continue
		}
		limit = rule.limit
	}
	return limit
}

// BodyLimit 为所有请求挂上体积上限，双保险：
//  1. 声明了 Content-Length 且超过上限 → 不读 body，立即返回 413 JSON；
//  2. 分块传输（无 Content-Length）→ 用 http.MaxBytesReader 限制实际读取量，
//     handler 侧读到上限时拿到 *http.MaxBytesError（errors.As 可判定）。
//
// BodyLimit installs the global request-body cap. Oversized Content-Length is
// rejected with 413 JSON before the body is read; chunked bodies are bounded
// via http.MaxBytesReader so handler bind errors surface as
// *http.MaxBytesError.
func BodyLimit(defaultLimit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := bodyLimitForPath(c.Request.URL.Path, defaultLimit)
		if limit <= 0 {
			c.Next()
			return
		}
		if c.Request.ContentLength > limit {
			handlers.JSONErrorWithCode(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
				fmt.Sprintf("request body exceeds the %d byte limit", limit))
			c.Abort()
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
