package signaling

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/fcm"
	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/presence"
	"github.com/allcallall/backend/internal/user"
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
	Payload json.RawMessage `json:"payload"`
}

const (
	TypeCallInvite    = "call.invite"
	TypeCallInviteAck = "call.invite.ack"
	TypeCallAccept    = "call.accept"
	TypeCallReject    = "call.reject"
	TypeCallEnd       = "call.end"
	TypeIceCandidate  = "ice.candidate"
	TypeClientPing    = "client.ping"
	TypeServerPong    = "server.pong"
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

// HandleConnection 处理单个连接
// HandleConnection attaches websocket connection to the hub.
func (h *Hub) HandleConnection(ctx context.Context, email string, conn *websocket.Conn) {
	cl := &client{
		email: email,
		conn:  conn,
		send:  make(chan []byte, 16),
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if h.presence != nil {
		if err := h.presence.SetOnline(ctx, email); err != nil {
			h.logger.Warn().Err(err).Str("email", email).Msg("failed to mark user online")
		}
		defer func() {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := h.presence.SetOffline(timeoutCtx, email); err != nil {
				h.logger.Warn().Err(err).Str("email", email).Msg("failed to mark user offline")
			}
		}()
	}

	h.addClient(cl)
	defer h.removeClient(cl)

	go h.writeLoop(ctx, cl)

	// Redis channel for cross-instance delivery.
	sub := h.redis.Subscribe(ctx, h.channelName(email))
	defer sub.Close()

	go h.redisForwarder(ctx, sub, cl)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if err := h.handleIncoming(ctx, cl, data); err != nil {
			h.logger.Warn().Err(err).Msg("failed to handle incoming signaling message")
		}
	}
}

func (h *Hub) handleIncoming(ctx context.Context, fromClient *client, data []byte) error {
	if h.presence != nil {
		if err := h.presence.UpdateLastSeen(ctx, fromClient.email); err != nil {
			h.logger.Debug().Err(err).Str("email", fromClient.email).Msg("failed to refresh last seen")
		}
	}

	var msg SignalMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}
	msg.From = fromClient.email
	if msg.Type != TypeClientPing && msg.To == "" {
		return fmt.Errorf("missing target 'to'")
	}

	ackMsg, err := h.applyProtocolRules(&msg)
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if msg.Type != TypeClientPing {
		h.dispatchLocal(msg.To, encoded)
		h.enqueue(ctx, msg.To, encoded)
	}

	envBytes, err := json.Marshal(redisEnvelope{
		NodeID: h.nodeID,
		Data:   encoded,
	})
	if err != nil {
		return err
	}

	if ackMsg != nil {
		if ackBytes, err := json.Marshal(ackMsg); err == nil {
			h.dispatchLocal(msg.From, ackBytes)
			h.enqueue(ctx, msg.From, ackBytes)
		} else {
			h.logger.Warn().Err(err).Msg("failed to marshal ack message")
		}
	}

	// 如果是 call.invite 消息，需要发送推送通知
	// If this is a call.invite message, send push notification
	if msg.Type == TypeCallInvite {
		h.sendCallNotification(ctx, msg.To, msg.From)
	}

	if msg.Type == TypeClientPing {
		return nil
	}

	return h.redis.Publish(ctx, h.channelName(msg.To), envBytes).Err()
}

func (h *Hub) HandleHTTPMessage(ctx context.Context, fromEmail string, data []byte) error {
	if h.presence != nil {
		if err := h.presence.UpdateLastSeen(ctx, fromEmail); err != nil {
			h.logger.Debug().Err(err).Str("email", fromEmail).Msg("failed to refresh last seen")
		}
	}

	var msg SignalMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}
	msg.From = fromEmail
	if msg.Type != TypeClientPing && msg.To == "" {
		return fmt.Errorf("missing target 'to'")
	}

	ackMsg, err := h.applyProtocolRules(&msg)
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if msg.Type != TypeClientPing {
		h.enqueue(ctx, msg.To, encoded)
		h.dispatchLocal(msg.To, encoded)
	}

	if ackMsg != nil {
		if ackBytes, err := json.Marshal(ackMsg); err == nil {
			h.enqueue(ctx, msg.From, ackBytes)
			h.dispatchLocal(msg.From, ackBytes)
		} else {
			h.logger.Warn().Err(err).Msg("failed to marshal ack message")
		}
	}

	if msg.Type == TypeCallInvite {
		h.sendCallNotification(ctx, msg.To, msg.From)
	}

	if msg.Type == TypeClientPing {
		return nil
	}

	envBytes, err := json.Marshal(redisEnvelope{
		NodeID: h.nodeID,
		Data:   encoded,
	})
	if err != nil {
		return err
	}

	return h.redis.Publish(ctx, h.channelName(msg.To), envBytes).Err()
}

func (h *Hub) applyProtocolRules(msg *SignalMessage) (*SignalMessage, error) {
	switch msg.Type {
	case TypeClientPing:
		return &SignalMessage{
			Type:    TypeServerPong,
			To:      msg.From,
			From:    msg.From,
			Payload: json.RawMessage("null"),
		}, nil
	case TypeCallInvite:
		if msg.CallID == "" {
			msg.CallID = uuid.NewString()
		}
		return &SignalMessage{
			Type:    TypeCallInviteAck,
			CallID:  msg.CallID,
			To:      msg.From,
			From:    msg.From,
			Payload: msg.Payload,
		}, nil
	case TypeCallAccept, TypeCallReject, TypeCallEnd:
		if msg.CallID == "" {
			return nil, fmt.Errorf("call_id required for message type %s", msg.Type)
		}
	case TypeIceCandidate:
		if msg.CallID == "" {
			return nil, fmt.Errorf("call_id required for ice candidate message")
		}
		if len(msg.Payload) == 0 {
			return nil, fmt.Errorf("payload required for ice candidate message")
		}
	default:
		// Legacy types (offer/answer/etc.) are still allowed without additional validation.
	}
	return nil, nil
}

func (h *Hub) enqueue(ctx context.Context, target string, payload []byte) {
	if h.redis == nil {
		return
	}
	key := signalQueuePrefix + target
	pipe := h.redis.Pipeline()
	pipe.LPush(ctx, key, payload)
	pipe.LTrim(ctx, key, 0, signalQueueMaxLen-1)
	pipe.Expire(ctx, key, time.Hour)
	_, err := pipe.Exec(ctx)
	if err != nil {
		h.logger.Debug().Err(err).Str("email", target).Msg("failed to enqueue signaling payload")
	}
}

func (h *Hub) Poll(ctx context.Context, email string, timeout time.Duration) ([]byte, bool, error) {
	if h.redis == nil {
		return nil, false, nil
	}
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	key := signalQueuePrefix + email
	res, err := h.redis.BRPop(ctx, timeout, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(res) != 2 {
		return nil, false, nil
	}
	return []byte(res[1]), true, nil
}

func (h *Hub) addClient(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[cl.email]; !ok {
		h.clients[cl.email] = make(map[*client]struct{})
	}
	h.clients[cl.email][cl] = struct{}{}
	h.logger.Info().Str("email", cl.email).Msg("client connected")
}

func (h *Hub) removeClient(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[cl.email]; ok {
		delete(conns, cl)
		if len(conns) == 0 {
			delete(h.clients, cl.email)

			// 联动：当用户所有信号连接断开时，清理其 WebRTC 媒体会话
			// Linkage: Clean up media sessions if no active signaling connections remain
			if h.mediaEngine != nil {
				go h.mediaEngine.CloseUserSessions(cl.email)
			}
		}
	}
	close(cl.send)
	_ = cl.conn.Close()
	h.logger.Info().Str("email", cl.email).Msg("client disconnected")
}

func (h *Hub) dispatchLocal(target string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for cl := range h.clients[target] {
		select {
		case cl.send <- payload:
		default:
			h.logger.Warn().Str("email", target).Msg("dropping signaling message due to slow client")
		}
	}
}

func (h *Hub) writeLoop(ctx context.Context, cl *client) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-cl.send:
			if !ok {
				return
			}
			if err := cl.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				h.logger.Warn().Err(err).Str("email", cl.email).Msg("write message failed")
				return
			}
		}
	}
}

func (h *Hub) redisForwarder(ctx context.Context, sub *redis.PubSub, cl *client) {
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var env redisEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
				h.logger.Warn().Err(err).Msg("failed to decode redis envelope")
				continue
			}
			if env.NodeID == h.nodeID {
				continue
			}
			select {
			case cl.send <- env.Data:
			default:
				h.logger.Warn().Str("email", cl.email).Msg("drop redis message due to slow client")
			}
		}
	}
}

func (h *Hub) channelName(email string) string {
	return fmt.Sprintf("signal:%s", email)
}

// sendCallNotification 发送来电推送通知
// sendCallNotification sends push notification for incoming call
func (h *Hub) sendCallNotification(ctx context.Context, toEmail string, fromEmail string) {
	// 如果没有 FCM 管理器或用户服务，跳过
	// Skip if FCM manager or user service not available
	if h.fcmManager == nil || h.users == nil {
		h.logger.Debug().
			Str("to", toEmail).
			Str("from", fromEmail).
			Msg("fcm manager or user service not available, skipping notification")
		return
	}

	// 不阻塞地发送推送通知，使用 goroutine
	// Send notification asynchronously to avoid blocking
	go func() {
		// 调用上上下文以取消悠斶
		// Create a new context with timeout
		notifCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 获取接收者的用户信息
		// Get recipient user info
		toUser, err := h.users.GetByEmail(notifCtx, toEmail)
		if err != nil {
			h.logger.Warn().Err(err).Str("email", toEmail).Msg("failed to get recipient user info")
			return
		}

		if toUser.FCMToken == "" {
			h.logger.Debug().Str("email", toEmail).Msg("recipient has no fcm token")
			return
		}

		// 获取勳者的用户信息
		// Get initiator user info
		fromUser, err := h.users.GetByEmail(notifCtx, fromEmail)
		if err != nil {
			h.logger.Warn().Err(err).Str("email", fromEmail).Msg("failed to get initiator user info")
			return
		}

		// 发送推送通知
		// Send the notification
		err = h.fcmManager.SendCallNotification(
			notifCtx,
			toUser.FCMToken,
			fromEmail,
			fromUser.DisplayName,
			"", // callID will be added later if needed
		)
		if err != nil {
			h.logger.Error().Err(err).
				Str("to", toEmail).
				Str("from", fromEmail).
				Msg("failed to send call notification")
			return
		}

		h.logger.Info().
			Str("to", toEmail).
			Str("from", fromEmail).
			Msg("call notification sent successfully")
	}()
}
