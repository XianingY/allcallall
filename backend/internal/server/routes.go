package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/handlers"
	"github.com/allcallall/backend/internal/metrics"
)

// RouteDependencies 路由所需依赖
// RouteDependencies bundles handlers and middleware.
type RouteDependencies struct {
	AuthHandler      *handlers.AuthHandler
	EmailHandler     *handlers.EmailHandler
	UserHandler      *handlers.UserHandler
	Commercial       *handlers.CommercialHandler
	Collaboration    *handlers.CollaborationHandler
	Agent            *handlers.AgentHandler
	Knowledge        *handlers.KnowledgeHandler
	Invitations      *handlers.InvitationHandler
	SignalingHandler *handlers.SignalingHandler
	SignalingPoll    *handlers.SignalingPollHandler
	WebRTCHandler    *handlers.WebRTCHandler
	TranslationWS    *handlers.TranslationWSHandler
	AuthMiddleware   gin.HandlerFunc
	Metrics          *metrics.CounterStore
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

	// 健康检查
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	if deps.Metrics != nil {
		api.GET("/metrics", func(c *gin.Context) {
			c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(deps.Metrics.RenderPrometheus()))
		})
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
		protected.GET("/ws", deps.SignalingHandler.Handle)
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
