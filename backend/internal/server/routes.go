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
	}

	emailGroup := api.Group("")
	deps.EmailHandler.RegisterRoutes(emailGroup)

	protected := api.Group("/")
	protected.Use(deps.AuthMiddleware)
	{
		userGroup := protected.Group("/users")
		deps.UserHandler.RegisterRoutes(userGroup)
		if deps.Commercial != nil {
			deps.Commercial.RegisterProtectedRoutes(protected)
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
