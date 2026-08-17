package signaling

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"time"
)

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

	// Detect dead/half-open connections: require a pong (or any inbound frame)
	// within pongWait, and refresh that deadline whenever a pong arrives.
	cl.conn.SetReadDeadline(time.Now().Add(pongWait))
	cl.conn.SetPongHandler(func(string) error {
		cl.conn.SetReadDeadline(time.Now().Add(pongWait))
		if h.presence != nil {
			if err := h.presence.Heartbeat(ctx, email, "signaling", ""); err != nil {
				h.logger.Debug().Err(err).Str("email", email).Msg("presence heartbeat on pong failed")
			}
		}
		return nil
	})

	if h.presence != nil {
		if err := h.presence.Heartbeat(ctx, email, "signaling", ""); err != nil {
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
	go h.pingTicker(ctx, cl)

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
	if cl.conn != nil {
		_ = cl.conn.Close()
	}
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

// pingTicker periodically prods the client so idle but alive connections keep
// their read deadline alive, and dead ones are reaped by the deadline.
func (h *Hub) pingTicker(ctx context.Context, cl *client) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	ping, err := json.Marshal(SignalMessage{Type: TypeServerPing, To: cl.email, From: cl.email})
	if err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cl.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := cl.conn.WriteMessage(websocket.TextMessage, ping); err != nil {
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
