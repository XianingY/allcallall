package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/handlers"
)

func TestReadinessEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("ready", func(t *testing.T) {
		router := gin.New()
		registerHealthRoutesForTest(router, map[string]ReadinessCheck{
			"mysql": func(context.Context) error { return nil },
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", response.Code)
		}
	})

	t.Run("dependency failure", func(t *testing.T) {
		router := gin.New()
		registerHealthRoutesForTest(router, map[string]ReadinessCheck{
			"redis": func(context.Context) error { return errors.New("unavailable") },
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", response.Code)
		}
	})
}

func registerHealthRoutesForTest(router *gin.Engine, checks map[string]ReadinessCheck) {
	api := router.Group("/api/v1")
	registerHealthRoutes(api, RouteDependencies{ReadinessChecks: checks})
}

// TestProbeRoutesSurviveRequireTLS 锁定探针不受 TLS 强制影响。
// kubelet 的 liveness/readiness 直连 PodIP，不经过 Ingress，请求不带
// X-Forwarded-Proto。若探针落在 RequireTLS 组内，开启 SECURITY_REQUIRE_TLS
// 后全部返回 403，新副本永远 Ready 不了，滚动更新会卡死。
func TestProbeRoutesSurviveRequireTLS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deps := RouteDependencies{
		RequireTLS: true,
		ReadinessChecks: map[string]ReadinessCheck{
			"mysql": func(context.Context) error { return nil },
		},
		// RegisterRoutes 对这两个 handler 未做 nil 判断（auth_handler.go / email_handler.go
		// 的 RegisterRoutes 会被无条件调用），必须显式构造零值版本。这里只注册路由、
		// 不发请求，因此 nil 依赖不会被解引用。
		AuthHandler:  handlers.NewAuthHandler(zerolog.Nop(), nil, nil, nil),
		EmailHandler: handlers.NewEmailHandler(zerolog.Nop(), nil),
	}

	router := gin.New()
	RegisterRoutes(router, deps)

	for _, path := range []string{"/api/v1/health", "/api/v1/ready", "/api/v1/status"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			// 模拟 kubelet：明文 http，不带任何 Forwarded 头。
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("probe %s should bypass RequireTLS, got %d body=%s", path, response.Code, response.Body.String())
			}
		})
	}
}

// TestRequireTLSStillGuardsBusinessRoutes 反向锁定：探针豁免不能以整体关掉
// TLS 保护为代价，业务端点在明文下仍必须被拒绝。
func TestRequireTLSStillGuardsBusinessRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(RequireTLS(true))
	api.GET("/business", func(c *gin.Context) { c.Status(http.StatusOK) })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/business", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("business route must stay TLS-guarded, got %d", response.Code)
	}
}
