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
	engine.Use(requestLogger(log.With().Str("component", "http").Logger(), counterStore))

	return engine
}
