package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/config"
)

// WebRTCHandler exposes runtime WebRTC configuration (ICE/TURN servers) to clients.
type WebRTCHandler struct {
	logger zerolog.Logger
	cfg    config.WebRTCConfig
}

// NewWebRTCHandler 构造函数
// NewWebRTCHandler creates a WebRTCHandler instance.
func NewWebRTCHandler(log zerolog.Logger, cfg config.WebRTCConfig) *WebRTCHandler {
	return &WebRTCHandler{
		logger: log.With().Str("component", "webrtc_handler").Logger(),
		cfg:    cfg,
	}
}

// RegisterRoutes 注册路由
func (h *WebRTCHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/webrtc/config", h.GetConfig)
}

// GetConfig 返回 ICE/TURN 配置
func (h *WebRTCHandler) GetConfig(c *gin.Context) {
	ice := h.cfg.ICEServers
	if ice == nil {
		ice = []config.ICEServer{}
	}
	h.logger.Debug().Int("ice_server_count", len(ice)).Msg("serving webrtc config")

	c.JSON(http.StatusOK, gin.H{
		"ice_servers": ice,
	})
}
