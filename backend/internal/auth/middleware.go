package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Middleware 返回 Gin 中间件
// Middleware validates Authorization header (Bearer token) or query parameter token for WebSocket.
func Middleware(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 优先尝试从 Authorization 请求头中提取 token
		token := extractToken(c.Request.Header.Get("Authorization"))

		// 如果没有找到，尝试从 URL 查询参数中获取（用于 WebSocket 连接）
		if token == "" {
			token = c.Query("token")
			if token != "" {
				// Debug: 从查询参数获取到 token
				_ = token // 仅用于调试
			}
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		claims, err := manager.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		SetClaimsToContext(c, claims)
		c.Next()
	}
}

func extractToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
