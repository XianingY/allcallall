package connectionregistry

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"

	appcfg "github.com/allcallall/backend/internal/config"
)

// errSelfIDRequired 表示网关自身节点 ID 尚未配置。
var errSelfIDRequired = errors.New("connection gateway self id required")

// ConnectionGateway 连接层负载均衡网关：
//   - 本节点向 Registry 注册并周期性心跳；
//   - 后台从 Registry 同步存活节点，维护一致哈希环；
//   - Route(key) 用一致哈希把连接键（userID / roomID 等）稳定路由到后端节点。
type ConnectionGateway struct {
	cfg      appcfg.ConnectionGatewayConfig
	registry Registry
	self     Node
	ring     *HashRing
	nodes    map[string]Node
	mu       sync.RWMutex
	logger   zerolog.Logger
}

// New 构造网关。registry 为 nil 时网关仅维护空环（Route 恒返回 false）。
func New(cfg appcfg.ConnectionGatewayConfig, registry Registry) *ConnectionGateway {
	replicas := cfg.HashReplicas
	if replicas <= 0 {
		replicas = 100
	}
	g := &ConnectionGateway{
		cfg:      cfg,
		registry: registry,
		ring:     NewHashRing(replicas),
		nodes:    make(map[string]Node),
		logger:   zerolog.Nop(),
	}
	g.self = Node{ID: cfg.SelfID, Addr: cfg.AdvertiseAddr, Metadata: map[string]string{}}
	return g
}

// WithLogger 注入结构化日志。
func (g *ConnectionGateway) WithLogger(l zerolog.Logger) *ConnectionGateway {
	g.logger = l
	return g
}

// SelfID 返回本节点 ID。
func (g *ConnectionGateway) SelfID() string { return g.self.ID }

// Register 把本节点注册到 Registry。addr 为空时使用配置中的 AdvertiseAddr。
func (g *ConnectionGateway) Register(ctx context.Context, addr string) error {
	if addr != "" {
		g.self.Addr = addr
	}
	if g.self.ID == "" {
		return errSelfIDRequired
	}
	if g.registry == nil {
		return errors.New("connection gateway registry is nil")
	}
	return g.registry.Register(ctx, g.self, g.nodeTTL())
}

// Heartbeat 刷新本节点在 Registry 中的 TTL。
func (g *ConnectionGateway) Heartbeat(ctx context.Context) error {
	if g.self.ID == "" {
		return errSelfIDRequired
	}
	if g.registry == nil {
		return errors.New("connection gateway registry is nil")
	}
	return g.registry.Heartbeat(ctx, g.self, g.nodeTTL())
}

// Deregister 从 Registry 注销本节点（优雅关闭时调用）。
func (g *ConnectionGateway) Deregister(ctx context.Context) error {
	if g.self.ID == "" {
		return errSelfIDRequired
	}
	if g.registry == nil {
		return nil
	}
	return g.registry.Deregister(ctx, g.self.ID)
}

func (g *ConnectionGateway) nodeTTL() time.Duration {
	sec := g.cfg.NodeTTLSec
	if sec <= 0 {
		sec = 30
	}
	return time.Duration(sec) * time.Second
}

// Start 启动后台循环：立即注册 + 同步，之后按 HeartbeatSec 周期心跳并同步环。
// 随 ctx 取消而退出。
func (g *ConnectionGateway) Start(ctx context.Context) {
	hb := g.cfg.HeartbeatSec
	if hb <= 0 {
		hb = 10
	}
	interval := time.Duration(hb) * time.Second
	if err := g.Register(ctx, g.self.Addr); err != nil {
		g.logger.Warn().Err(err).Msg("gateway self-register failed")
	}
	g.syncFromRegistry(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := g.Heartbeat(ctx); err != nil {
				g.logger.Warn().Err(err).Msg("gateway heartbeat failed")
			}
			g.syncFromRegistry(ctx)
		}
	}
}

// SyncOnce 立即从 Registry 同步存活节点并重建哈希环（测试可免等心跳周期）。
func (g *ConnectionGateway) SyncOnce(ctx context.Context) {
	g.syncFromRegistry(ctx)
}

func (g *ConnectionGateway) syncFromRegistry(ctx context.Context) {
	if g.registry == nil {
		return
	}
	nodes, err := g.registry.ListActive(ctx)
	if err != nil {
		g.logger.Warn().Err(err).Msg("gateway list active nodes failed")
		return
	}
	ids := make([]string, 0, len(nodes))
	g.mu.Lock()
	g.nodes = make(map[string]Node, len(nodes))
	for _, n := range nodes {
		g.nodes[n.ID] = n
		ids = append(ids, n.ID)
	}
	g.mu.Unlock()

	g.ring.SetNodes(ids)
}

// Nodes 返回当前已知存活节点快照。
func (g *ConnectionGateway) Nodes() []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	return out
}

// Route 用一致哈希把连接键路由到稳定后端节点。
func (g *ConnectionGateway) Route(key string) (Node, bool) {
	id, ok := g.ring.Get(key)
	if !ok {
		return Node{}, false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	return n, ok
}
