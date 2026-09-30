package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/handlers"
	"github.com/allcallall/backend/internal/metrics"
)

type ReadinessCheck func(context.Context) error

// RouteDependencies 路由所需依赖
// RouteDependencies bundles handlers and middleware.
type RouteDependencies struct {
	AuthHandler        *handlers.AuthHandler
	EmailHandler       *handlers.EmailHandler
	UserHandler        *handlers.UserHandler
	Push               *handlers.PushHandler
	Commercial         *handlers.CommercialHandler
	Collaboration      *handlers.CollaborationHandler
	Agent              *handlers.AgentHandler
	Knowledge          *handlers.KnowledgeHandler
	Invitations        *handlers.InvitationHandler
	SignalingHandler   *handlers.SignalingHandler
	SignalingPoll      *handlers.SignalingPollHandler
	WebRTCHandler      *handlers.WebRTCHandler
	TranslationWS      *handlers.TranslationWSHandler
	Realtime           *handlers.RealtimeHandler
	TaskScheduler      *handlers.TaskSchedulerHandler
	OrgBilling         *handlers.OrgBillingHandler
	Chat               *handlers.ChatHandler
	AuthMiddleware     gin.HandlerFunc
	ChatRealtimeAuth   gin.HandlerFunc
	SignalRealtimeAuth gin.HandlerFunc
	RoomRealtimeAuth   gin.HandlerFunc
	Metrics            *metrics.CounterStore
	ReadinessChecks    map[string]ReadinessCheck
	RequireTLS         bool
}

// protectedMiddlewares 组装受保护路由组的中间件链。
// 组织归属不在此层解析：组织级端点（如 org billing）在各自 handler 内
// 从已认证主体派生，绝不采信客户端传入的 X-Organization-ID。
// protectedMiddlewares builds the middleware chain for authenticated routes.
func protectedMiddlewares(deps RouteDependencies) []gin.HandlerFunc {
	return []gin.HandlerFunc{deps.AuthMiddleware}
}

// RegisterRoutes 注册所有 HTTP 路由
// RegisterRoutes wires handlers into the Gin engine.
func RegisterRoutes(router *gin.Engine, deps RouteDependencies) {
	if deps.Commercial != nil {
		deps.Commercial.RegisterDocumentRoutes(router)
	}
	if deps.Invitations != nil {
		deps.Invitations.RegisterDocumentRoutes(router)
	}
	api := router.Group("/api/v1")
	api.Use(RequireTLS(deps.RequireTLS))
	// 探针组：前缀与 api 相同，但不套 RequireTLS。
	// kubelet 的 liveness/readiness 直连 PodIP 发起探测，不经过 Ingress/TLS 终结，
	// 请求里不会带 X-Forwarded-Proto；若把探针留在 api 组内，一旦开启
	// SECURITY_REQUIRE_TLS（生产合规要求），所有 Pod 探针都会返回 403，
	// 新副本永远无法 Ready，滚动更新必然卡死。
	// /status 按设计同样是可无鉴权抓取的运维端点（见 status.go 的 StatusResponse 注释），
	// /metrics 另有 MetricsAuthMiddleware 校验内网来源，因此一并放在该组。
	probeAPI := router.Group("/api/v1")
	registerHealthRoutes(probeAPI, deps)
	if deps.Collaboration != nil && deps.ChatRealtimeAuth != nil {
		deps.Collaboration.RegisterRealtimeRoutes(api, deps.ChatRealtimeAuth)
	}
	if deps.Collaboration != nil && deps.RoomRealtimeAuth != nil {
		deps.Collaboration.RegisterRoomRealtimeRoutes(api, deps.RoomRealtimeAuth)
	}
	if deps.SignalingHandler != nil && deps.SignalRealtimeAuth != nil {
		api.GET("/ws", deps.SignalRealtimeAuth, deps.SignalingHandler.Handle)
	}

	authGroup := api.Group("/auth")
	deps.AuthHandler.RegisterRoutes(authGroup)

	if deps.Commercial != nil {
		deps.Commercial.RegisterPublicRoutes(api)
		deps.Commercial.RegisterInternalRoutes(api)
	}
	if deps.Collaboration != nil {
		deps.Collaboration.RegisterInternalRoutes(api)
	}
	if deps.Agent != nil {
		deps.Agent.RegisterInternalRoutes(api)
	}
	if deps.Invitations != nil {
		deps.Invitations.RegisterPublicRoutes(api)
	}

	emailGroup := api.Group("")
	deps.EmailHandler.RegisterRoutes(emailGroup)

	protected := api.Group("/")
	for _, mw := range protectedMiddlewares(deps) {
		protected.Use(mw)
	}
	{
		protectedAuthGroup := protected.Group("/auth")
		deps.AuthHandler.RegisterProtectedRoutes(protectedAuthGroup)

		userGroup := protected.Group("/users")
		deps.UserHandler.RegisterRoutes(userGroup)
		if deps.Push != nil {
			deps.Push.RegisterProtectedRoutes(protected)
		}
		if deps.Commercial != nil {
			deps.Commercial.RegisterProtectedRoutes(protected)
		}
		if deps.Collaboration != nil {
			deps.Collaboration.RegisterProtectedRoutes(protected)
		}
		if deps.Agent != nil {
			deps.Agent.RegisterProtectedRoutes(protected)
		}
		if deps.Knowledge != nil {
			deps.Knowledge.RegisterProtectedRoutes(protected)
		}
		if deps.Invitations != nil {
			deps.Invitations.RegisterProtectedRoutes(protected)
		}
		if deps.Realtime != nil {
			deps.Realtime.RegisterProtectedRoutes(protected)
		}
		if deps.TaskScheduler != nil {
			deps.TaskScheduler.RegisterRoutes(protected)
		}
		if deps.OrgBilling != nil {
			deps.OrgBilling.RegisterProtectedRoutes(protected)
		}
		if deps.Chat != nil {
			deps.Chat.RegisterRoutes(protected)
		}
		if deps.SignalingPoll != nil {
			deps.SignalingPoll.RegisterRoutes(protected)
		}
		if deps.WebRTCHandler != nil {
			deps.WebRTCHandler.RegisterRoutes(protected)
		}
		if deps.TranslationWS != nil {
			protected.GET("/translation/ws", deps.TranslationWS.Handle)
		}
	}
}

// registerHealthRoutes 注册健康检查、就绪探针、状态页与指标端点。
//
// 调用方必须把它注册到**不含 RequireTLS** 的路由组上（见 RegisterRoutes 的 probeAPI），
// 否则开启 TLS 强制后 kubelet 的明文探针会被 403，导致滚动更新失败。
// registerHealthRoutes installs health, readiness, status and metrics endpoints.
// Callers must register it on a group that does NOT apply RequireTLS.
func registerHealthRoutes(api *gin.RouterGroup, deps RouteDependencies) {

	// 健康检查
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	api.GET("/ready", func(c *gin.Context) {
		failures := make(map[string]string)
		for name, check := range deps.ReadinessChecks {
			if check == nil {
				continue
			}
			if err := check(c.Request.Context()); err != nil {
				failures[name] = err.Error()
			}
		}
		if len(failures) > 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "checks": failures})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	if deps.Metrics != nil {
		api.GET("/metrics", handlers.MetricsAuthMiddleware(), func(c *gin.Context) {
			c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(deps.Metrics.RenderPrometheus()))
		})
	}
	RegisterStatusRoutes(api, deps)
}
