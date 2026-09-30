package server

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/metrics"
)

// NewEngine 创建并返回 Gin 引擎
// NewEngine returns a Gin engine with baseline middleware.
func NewEngine(log zerolog.Logger, counters ...*metrics.CounterStore) *gin.Engine {
	var counterStore *metrics.CounterStore
	if len(counters) > 0 {
		counterStore = counters[0]
	}
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(SecurityHeadersMiddleware())
	engine.Use(requestLogger(log.With().Str("component", "http").Logger(), counterStore))
	// P0-1：全局请求体上限，必须排在 requestLogger 之后、路由之前生效。
	engine.Use(BodyLimit(defaultBodyLimitBytes))

	return engine
}
