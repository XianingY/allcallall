package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/signaling"
)

// SignalingHandler 信令处理器
// SignalingHandler upgrades HTTP requests to WebSocket for signaling.
type SignalingHandler struct {
	logger   zerolog.Logger
	hub      *signaling.Hub
	upgrader websocket.Upgrader
}

// NewSignalingHandler 构造函数
// NewSignalingHandler creates a SignalingHandler.
func NewSignalingHandler(log zerolog.Logger, hub *signaling.Hub) *SignalingHandler {
	return &SignalingHandler{
		logger: log.With().Str("component", "signaling_handler").Logger(),
		hub:    hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Handle 升级 WebSocket
// Handle upgrades request to WebSocket and delegates to hub.
func (h *SignalingHandler) Handle(c *gin.Context) {
	// 中间件已经验证了 token（从 Authorization 第一个请求头或查询参数）
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		// 正常不会走到这里，因为中间件已经随之丢弃此请求
		h.logger.Error().Str("path", c.Request.URL.Path).Str("query", c.Request.URL.RawQuery).Msg("no auth claims in context")
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	h.logger.Info().Str("email", claims.Email).Msg("websocket upgrade attempt")

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to upgrade websocket")
		return
	}

	h.logger.Info().Str("email", claims.Email).Msg("websocket connection established")
	h.hub.HandleConnection(c.Request.Context(), claims.Email, conn)
}
