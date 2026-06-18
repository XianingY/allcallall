package runtime

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type RedisStreamPublisher struct {
	client *redis.Client
}

func NewRedisStreamPublisher(client *redis.Client) *RedisStreamPublisher {
	return &RedisStreamPublisher{client: client}
}

func (p *RedisStreamPublisher) PublishToken(ctx context.Context, runID uint64, token string) error {
	if p.client == nil {
		return nil
	}
	channel := fmt.Sprintf("agent_run:%d:stream", runID)
	return p.client.Publish(ctx, channel, token).Err()
}
