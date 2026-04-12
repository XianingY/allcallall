package collaboration

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type ChatHub struct {
	logger  zerolog.Logger
	mu      sync.RWMutex
	clients map[uint64]map[*chatClient]struct{}
}

type chatClient struct {
	userID uint64
	orgID  uint64
	conn   *websocket.Conn
	send   chan []byte
}

func NewChatHub(logger zerolog.Logger) *ChatHub {
	return &ChatHub{
		logger:  logger.With().Str("component", "chat_hub").Logger(),
		clients: make(map[uint64]map[*chatClient]struct{}),
	}
}

func (h *ChatHub) HandleConnection(ctx context.Context, userID, orgID uint64, conn *websocket.Conn) {
	client := &chatClient{
		userID: userID,
		orgID:  orgID,
		conn:   conn,
		send:   make(chan []byte, 32),
	}
	h.addClient(client)
	defer h.removeClient(client)

	go h.writeLoop(ctx, client)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *ChatHub) PublishToUsers(_ context.Context, organizationID uint64, userIDs []uint64, event string, payload any) error {
	body, err := json.Marshal(map[string]any{
		"event":           event,
		"organization_id": organizationID,
		"payload":         payload,
	})
	if err != nil {
		return err
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, userID := range userIDs {
		for client := range h.clients[userID] {
			if client.orgID != organizationID {
				continue
			}
			select {
			case client.send <- body:
			default:
				h.logger.Warn().Uint64("user_id", userID).Msg("dropping chat event due to slow client")
			}
		}
	}
	return nil
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
			if err := client.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}
}
