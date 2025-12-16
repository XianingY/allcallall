package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/handlers"
)

// RouteDependencies 路由所需依赖
// RouteDependencies bundles handlers and middleware.
type RouteDependencies struct {
	AuthHandler      *handlers.AuthHandler
	EmailHandler     *handlers.EmailHandler
	UserHandler      *handlers.UserHandler
	SignalingHandler *handlers.SignalingHandler
	WebRTCHandler    *handlers.WebRTCHandler
	AuthMiddleware   gin.HandlerFunc
}

// RegisterRoutes 注册所有 HTTP 路由
// RegisterRoutes wires handlers into the Gin engine.
func RegisterRoutes(router *gin.Engine, deps RouteDependencies) {
	api := router.Group("/api/v1")

	// 健康检查
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	authGroup := api.Group("/auth")
	deps.AuthHandler.RegisterRoutes(authGroup)

	emailGroup := api.Group("")
	deps.EmailHandler.RegisterRoutes(emailGroup)

	protected := api.Group("/")
	protected.Use(deps.AuthMiddleware)
	{
		userGroup := protected.Group("/users")
		deps.UserHandler.RegisterRoutes(userGroup)
		protected.GET("/ws", deps.SignalingHandler.Handle)
		if deps.WebRTCHandler != nil {
			deps.WebRTCHandler.RegisterRoutes(protected)
		}
	}
}
