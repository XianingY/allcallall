package presence

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// presenceFeedChannel returns the per-user presence fan-out channel that the
// signaling hub subscribes to for local delivery.
func presenceFeedChannel(email string) string {
	return "presence:feed:" + email
}

// Broadcaster 订阅 presence:events，将状态变更转发到各用户的
// presence:feed:<email> 频道，由信令 Hub 投递到该用户本节点已连接的设备。
// 该设计复用了既有 Redis pub/sub 跨实例桥接模式，无需引入新中间件。
type Broadcaster struct {
	redis  *redis.Client
	logger zerolog.Logger
}

// NewBroadcaster 创建状态广播器。
func NewBroadcaster(rdb *redis.Client, log zerolog.Logger) *Broadcaster {
	return &Broadcaster{
		redis:  rdb,
		logger: log.With().Str("component", "presence_broadcaster").Logger(),
	}
}

// Start 订阅并转发状态变更事件，直到 ctx 取消。失败降级为静默丢弃。
func (b *Broadcaster) Start(ctx context.Context) {
	if b.redis == nil {
		return
	}
	sub := b.redis.Subscribe(ctx, eventsChannel)
	defer sub.Close()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			b.handle(ctx, []byte(msg.Payload))
		}
	}
}

func (b *Broadcaster) handle(ctx context.Context, payload []byte) {
	var ev PresenceEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		b.logger.Warn().Err(err).Msg("presence broadcast: invalid event")
		return
	}
	if ev.Email == "" {
		return
	}
	out, err := json.Marshal(map[string]any{
		"event":          "presence.changed",
		"email":          ev.Email,
		"state":          string(ev.State),
		"last_seen":      ev.LastSeen,
		"custom_message": ev.CustomMessage,
		"timestamp":      ev.Timestamp,
	})
	if err != nil {
		return
	}
	if err := b.redis.Publish(ctx, presenceFeedChannel(ev.Email), out).Err(); err != nil {
		b.logger.Warn().Err(err).Str("email", ev.Email).Msg("presence broadcast: publish failed")
	}
}
