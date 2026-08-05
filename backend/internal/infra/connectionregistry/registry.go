// Package connectionregistry 实现连接层负载均衡网关的基础设施：
// 节点注册表（Redis 多实例 / 内存单实例）与一致哈希环。
// 网关用注册表在多实例间宣告自身存活，并用一致哈希把连接键稳定路由到后端节点。
package connectionregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Node 表示一个已注册的后端连接网关节点。
type Node struct {
	ID       string            `json:"id"`
	Addr     string            `json:"addr"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Registry 节点注册表抽象：注册、心跳续租、列出存活节点、注销。
// 两个实现：RedisRegistry（多实例生产）、MemoryRegistry（单实例 / 测试）。
type Registry interface {
	Register(ctx context.Context, node Node, ttl time.Duration) error
	Heartbeat(ctx context.Context, node Node, ttl time.Duration) error
	ListActive(ctx context.Context) ([]Node, error)
	Deregister(ctx context.Context, nodeID string) error
}

const (
	redisNodesKey   = "cgw:nodes"
	redisNodeKeyFmt = "cgw:node:%s"
)

// RedisRegistry 基于 Redis 的节点注册表，键带 TTL，节点失活后自动过期。
// 命令均为 miniredis 兼容的原语（无 Lua）。
type RedisRegistry struct {
	client *redis.Client
}

// NewRedisRegistry 构造 Redis 注册表。
func NewRedisRegistry(client *redis.Client) *RedisRegistry {
	return &RedisRegistry{client: client}
}

func (r *RedisRegistry) nodeKey(id string) string {
	return fmt.Sprintf(redisNodeKeyFmt, id)
}

func (r *RedisRegistry) Register(ctx context.Context, node Node, ttl time.Duration) error {
	if node.ID == "" {
		return errors.New("node id required")
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	raw, err := json.Marshal(node)
	if err != nil {
		return err
	}
	if err := r.client.Set(ctx, r.nodeKey(node.ID), raw, ttl).Err(); err != nil {
		return err
	}
	return r.client.SAdd(ctx, redisNodesKey, node.ID).Err()
}

func (r *RedisRegistry) Heartbeat(ctx context.Context, node Node, ttl time.Duration) error {
	if node.ID == "" {
		return errors.New("node id required")
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	raw, err := json.Marshal(node)
	if err != nil {
		return err
	}
	// 刷新 TTL（节点已注册则续租，未注册则相当于重新注册）。
	return r.client.Set(ctx, r.nodeKey(node.ID), raw, ttl).Err()
}

func (r *RedisRegistry) ListActive(ctx context.Context) ([]Node, error) {
	ids, err := r.client.SMembers(ctx, redisNodesKey).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(ids))
	stale := make([]interface{}, 0)
	for _, id := range ids {
		raw, err := r.client.Get(ctx, r.nodeKey(id)).Result()
		if err == redis.Nil {
			stale = append(stale, id)
			continue
		}
		if err != nil {
			return nil, err
		}
		var n Node
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if len(stale) > 0 {
		// 清理已过期但仍在集合中的残留成员。
		_ = r.client.SRem(ctx, redisNodesKey, stale...).Err()
	}
	return out, nil
}

func (r *RedisRegistry) Deregister(ctx context.Context, nodeID string) error {
	if err := r.client.Del(ctx, r.nodeKey(nodeID)).Err(); err != nil {
		return err
	}
	return r.client.SRem(ctx, redisNodesKey, nodeID).Err()
}

// MemoryRegistry 进程内节点注册表，带 TTL 与惰性过期，用于单实例与测试。
type MemoryRegistry struct {
	mu    sync.Mutex
	nodes map[string]memEntry
	clock func() time.Time
}

type memEntry struct {
	node  Node
	expAt time.Time
}

// NewMemoryRegistry 构造内存注册表。
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{nodes: make(map[string]memEntry), clock: time.Now}
}

func (r *MemoryRegistry) Register(_ context.Context, node Node, ttl time.Duration) error {
	if node.ID == "" {
		return errors.New("node id required")
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[node.ID] = memEntry{node: node, expAt: r.clock().Add(ttl)}
	return nil
}

func (r *MemoryRegistry) Heartbeat(_ context.Context, node Node, ttl time.Duration) error {
	if node.ID == "" {
		return errors.New("node id required")
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[node.ID] = memEntry{node: node, expAt: r.clock().Add(ttl)}
	return nil
}

func (r *MemoryRegistry) ListActive(_ context.Context) ([]Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.clock()
	out := make([]Node, 0, len(r.nodes))
	for _, e := range r.nodes {
		if now.Before(e.expAt) {
			out = append(out, e.node)
		}
	}
	return out, nil
}

func (r *MemoryRegistry) Deregister(_ context.Context, nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeID)
	return nil
}
