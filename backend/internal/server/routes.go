package server

import (
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
	AuthMiddleware   gin.HandlerFunc
}

// RegisterRoutes 注册所有 HTTP 路由
// RegisterRoutes wires handlers into the Gin engine.
func RegisterRoutes(router *gin.Engine, deps RouteDependencies) {
	api := router.Group("/api/v1")

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
	}
}
