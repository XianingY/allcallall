package signaling

import (
	"encoding/json"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/fcm"
	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/user"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

// Hub 管理所有 WebSocket 连接
// Hub orchestrates signaling sessions across users and instances.
// 现在同时支持 WebSocket 信令和 Pion WebRTC 媒体引擎
// Now supports both WebSocket signaling and Pion WebRTC media engine
type Hub struct {
	redis       *redis.Client
	logger      zerolog.Logger
	presence    *presence.Manager
	mediaEngine *media.Engine
	users       *user.Service
	fcmManager  *fcm.Manager
	commercial  *commerce.Service
	collab      *collaboration.Service
	metrics     metrics.Recorder

	mu      sync.RWMutex
	clients map[string]map[*client]struct{}
	nodeID  string
}

// SignalMessage 信令消息
// SignalMessage represents the payload exchanged between peers.
type SignalMessage struct {
	Type    string          `json:"type"`
	CallID  string          `json:"call_id,omitempty"`
	To      string          `json:"to"`
	From    string          `json:"from"`
	Trace   string          `json:"traceparent,omitempty"` // Distributed tracing
	Payload json.RawMessage `json:"payload"`
}

const (
	TypeCallInvite    = "call.invite"
	TypeCallInviteAck = "call.invite.ack"
	TypeCallAccept    = "call.accept"
	TypeCallReject    = "call.reject"
	TypeCallEnd       = "call.end"
	TypeCallError     = "call.error"
	TypeIceCandidate  = "ice.candidate"
	TypeClientPing    = "client.ping"
	TypeServerPong    = "server.pong"
	TypeServerPing    = "server.ping"
)

const (
	// pongWait bounds how long a connection may stay silent before it is
	// declared dead. Combined with the server ping ticker this detects
	// half-open connections (e.g. phone in tunnel) that TCP alone would not.
	pongWait = 60 * time.Second
	// pingPeriod is slightly below pongWait so a live client always answers
	// before the read deadline trips.
	pingPeriod = 45 * time.Second
)

const (
	signalQueuePrefix = "signalq:"
	signalQueueMaxLen = 200
)

type client struct {
	email string
	conn  *websocket.Conn
	send  chan []byte
}

type redisEnvelope struct {
	NodeID string          `json:"node_id"`
	Data   json.RawMessage `json:"data"`
}

// NewHub 创建 Hub
// NewHub constructs a signaling hub.
func NewHub(redis *redis.Client, logger zerolog.Logger, presence *presence.Manager) *Hub {
	return &Hub{
		redis:    redis,
		logger:   logger.With().Str("component", "signaling_hub").Logger(),
		presence: presence,
		clients:  make(map[string]map[*client]struct{}),
		nodeID:   uuid.NewString(),
	}
}

// WithUserService 附加用户服务到 Hub
// WithUserService attaches user service to the hub
func (h *Hub) WithUserService(users *user.Service) {
	h.users = users
	h.logger.Info().Msg("user service attached to signaling hub")
}

// WithFCMManager 附加 FCM 管理器到 Hub
// WithFCMManager attaches FCM manager to the hub
func (h *Hub) WithFCMManager(fcmMgr *fcm.Manager) {
	h.fcmManager = fcmMgr
	h.logger.Info().Msg("fcm manager attached to signaling hub")
}

// WithCommercialService attaches commercialization service to signaling flows.
func (h *Hub) WithCommercialService(service *commerce.Service, counters metrics.Recorder) {
	h.commercial = service
	h.metrics = counters
	h.logger.Info().Msg("commercial service attached to signaling hub")
}

func (h *Hub) WithCollaborationService(service *collaboration.Service) {
	h.collab = service
	h.logger.Info().Msg("collaboration service attached to signaling hub")
}
