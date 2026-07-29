package collaboration

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type ChatHub struct {
	logger  zerolog.Logger
	redis   *redis.Client
	nodeID  string
	mu      sync.RWMutex
	clients map[uint64]map[*chatClient]struct{}
}

const chatSendBufferSize = 256

// chatRedisEnvelope wraps a realtime event for cross-instance delivery. The
// originating node id is included so a node can ignore its own publications
// (Redis pub/sub also delivers the message back to the publisher) and avoid
// double-delivering to its local connections.
type chatRedisEnvelope struct {
	NodeID         string          `json:"node_id"`
	UserID         uint64          `json:"user_id"`
	OrganizationID uint64          `json:"organization_id"`
	Data           json.RawMessage `json:"data"`
}

// chatChannel returns the Redis channel a user's events are published to.
func chatChannel(userID uint64) string {
	return "chat:user:" + strconv.FormatUint(userID, 10)
}

// chatWriteDeadline bounds how long a single websocket write may block before
// the per-connection write goroutine gives up and tears down the connection.
// Without it a slow/hung client that stops draining its TCP receive buffer
// would block the writer indefinitely and leak the goroutine.
const chatWriteDeadline = 10 * time.Second

type chatClient struct {
	userID uint64
	orgID  uint64
	conn   *websocket.Conn
	send   chan []byte
}

// NewChatHub constructs a chat hub. When redis is non-nil the hub bridges
// messages across backend instances via Redis pub/sub so a user connected to
// any node receives every event. When redis is nil the hub operates in
// local-only mode (fail open) and never panics if Redis is unavailable.
func NewChatHub(redis *redis.Client, logger zerolog.Logger) *ChatHub {
	return &ChatHub{
		logger:  logger.With().Str("component", "chat_hub").Logger(),
		redis:   redis,
		nodeID:  uuid.NewString(),
		clients: make(map[uint64]map[*chatClient]struct{}),
	}
}

// Start subscribes to cross-instance chat deliveries. It is a no-op when Redis
// is not configured. It returns immediately; the subscription loop runs in its
// own goroutine until ctx is cancelled, so callers (including the server's main
// goroutine) are never blocked.
func (h *ChatHub) Start(ctx context.Context) {
	if h.redis == nil {
		return
	}
	sub := h.redis.PSubscribe(ctx, "chat:user:*")
	go func() {
		defer sub.Close()
		h.redisForwarder(ctx, sub)
	}()
}

func (h *ChatHub) redisForwarder(ctx context.Context, sub *redis.PubSub) {
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var env chatRedisEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
				h.logger.Warn().Err(err).Msg("failed to decode redis chat envelope")
				continue
			}
			// Ignore our own publications: Redis pub/sub also delivers the
			// message back to the publishing node, but we already delivered it
			// locally in PublishToUser.
			if env.NodeID == h.nodeID {
				continue
			}
			h.deliverLocal(env.UserID, env.OrganizationID, env.Data)
		}
	}
}

func (h *ChatHub) HandleConnection(ctx context.Context, userID, orgID uint64, conn *websocket.Conn, loadBacklog func() []RealtimeEventRecord) {
	client := &chatClient{
		userID: userID,
		orgID:  orgID,
		conn:   conn,
		send:   make(chan []byte, chatSendBufferSize),
	}
	h.addClient(client)
	defer h.removeClient(client)

	go h.writeLoop(ctx, client)
	if loadBacklog != nil {
		for _, event := range loadBacklog() {
			body, err := marshalRealtimeEvent(event)
			if err != nil {
				h.logger.Warn().Err(err).Uint64("event_id", event.ID).Msg("failed to marshal replay chat event")
				continue
			}
			select {
			case client.send <- body:
			default:
				h.logger.Warn().Uint64("user_id", userID).Uint64("event_id", event.ID).Msg("dropping replay chat event due to slow client")
			}
		}
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *ChatHub) PublishToUser(ctx context.Context, event RealtimeEventRecord) error {
	body, err := marshalRealtimeEvent(event)
	if err != nil {
		return err
	}
	// Local delivery always happens first, regardless of Redis availability.
	h.deliverLocal(event.UserID, event.OrganizationID, body)

	// Bridge to other instances. Publication failures degrade gracefully to
	// local-only delivery (fail open) so a Redis outage cannot block chat.
	if h.redis != nil {
		envBytes, err := json.Marshal(chatRedisEnvelope{
			NodeID:         h.nodeID,
			UserID:         event.UserID,
			OrganizationID: event.OrganizationID,
			Data:           body,
		})
		if err != nil {
			h.logger.Warn().Err(err).Uint64("user_id", event.UserID).Msg("failed to marshal chat redis envelope")
			return nil
		}
		if err := h.redis.Publish(ctx, chatChannel(event.UserID), envBytes).Err(); err != nil {
			h.logger.Warn().Err(err).Uint64("user_id", event.UserID).Msg("failed to publish chat event to redis")
		}
	}
	return nil
}

func (h *ChatHub) deliverLocal(userID, orgID uint64, body []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients[userID] {
		if client.orgID != orgID {
			continue
		}
		select {
		case client.send <- body:
		default:
			h.logger.Warn().Uint64("user_id", userID).Msg("dropping chat event due to slow client")
		}
	}
}

func marshalRealtimeEvent(event RealtimeEventRecord) ([]byte, error) {
	return json.Marshal(map[string]any{
		"event_id":        event.ID,
		"sequence":        event.Sequence,
		"event":           event.Event,
		"organization_id": event.OrganizationID,
		"payload":         event.Payload,
		"created_at":      event.CreatedAt,
	})
}

func (h *ChatHub) addClient(client *chatClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.userID] == nil {
		h.clients[client.userID] = make(map[*chatClient]struct{})
	}
	h.clients[client.userID][client] = struct{}{}
}

func (h *ChatHub) removeClient(client *chatClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.clients[client.userID]; ok {
		delete(set, client)
		if len(set) == 0 {
			delete(h.clients, client.userID)
		}
	}
	_ = client.conn.Close()
}

func (h *ChatHub) writeLoop(ctx context.Context, client *chatClient) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-client.send:
			if !ok {
				return
			}
			// Reset the deadline on every write so a hung client eventually
			// unblocks the writer instead of stalling the goroutine forever.
			_ = client.conn.SetWriteDeadline(time.Now().Add(chatWriteDeadline))
			if err := client.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				h.logger.Debug().Err(err).Uint64("user_id", client.userID).Msg("chat websocket write failed")
				return
			}
		}
	}
}
