package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// ConversationCache provides Redis-backed caching for conversation data.
type ConversationCache struct {
	redis     *redis.Client
	ttl       time.Duration
	keyPrefix string
}

func NewConversationCache(redis *redis.Client, ttl time.Duration) *ConversationCache {
	return &ConversationCache{
		redis:     redis,
		ttl:       ttl,
		keyPrefix: "conv:",
	}
}

// GetOrCreate retrieves a cached conversation or executes the function and caches the result.
func (c *ConversationCache) GetOrCreate(conversationID string, fn func() (interface{}, error)) (interface{}, error) {
	key := c.keyPrefix + conversationID

	// Try to get from cache
	cached, err := c.redis.Get(context.Background(), key).Result()
	if err == nil {
		var result interface{}
		if err := json.Unmarshal([]byte(cached), &result); err == nil {
			return result, nil
		}
	}

	// Cache miss - execute function
	result, err := fn()
	if err != nil {
		return nil, err
	}

	// Cache the result
	data, err := json.Marshal(result)
	if err != nil {
		return result, nil // Return result even if caching fails
	}

	c.redis.Set(context.Background(), key, string(data), c.ttl)

	return result, nil
}

// Invalidate removes a conversation from cache.
func (c *ConversationCache) Invalidate(conversationID string) error {
	key := c.keyPrefix + conversationID
	return c.redis.Del(context.Background(), key).Err()
}

// BatchInvalidate removes multiple conversations from cache.
func (c *ConversationCache) BatchInvalidate(conversationIDs []string) error {
	if len(conversationIDs) == 0 {
		return nil
	}

	keys := make([]string, len(conversationIDs))
	for i, id := range conversationIDs {
		keys[i] = c.keyPrefix + id
	}

	return c.redis.Del(context.Background(), keys...).Err()
}
