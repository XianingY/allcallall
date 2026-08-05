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
	Chat               *handlers.ChatHandler
	AuthMiddleware     gin.HandlerFunc
	ChatRealtimeAuth   gin.HandlerFunc
	SignalRealtimeAuth gin.HandlerFunc
	RoomRealtimeAuth   gin.HandlerFunc
	Metrics            *metrics.CounterStore
	ReadinessChecks    map[string]ReadinessCheck
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
	registerHealthRoutes(api, deps)
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
	protected.Use(deps.AuthMiddleware)
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
}
