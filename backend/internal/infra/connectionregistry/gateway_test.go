package connectionregistry

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"

	appcfg "github.com/allcallall/backend/internal/config"
)

func TestGatewayRoute(t *testing.T) {
	reg := NewMemoryRegistry()
	cfg1 := appcfg.ConnectionGatewayConfig{SelfID: "n1", AdvertiseAddr: "10.0.0.1:8080", HeartbeatSec: 1, NodeTTLSec: 3, HashReplicas: 10}
	cfg2 := appcfg.ConnectionGatewayConfig{SelfID: "n2", AdvertiseAddr: "10.0.0.2:8080", HeartbeatSec: 1, NodeTTLSec: 3, HashReplicas: 10}
	g1 := New(cfg1, reg).WithLogger(zerolog.Nop())
	g2 := New(cfg2, reg).WithLogger(zerolog.Nop())

	if err := g1.Register(context.Background(), ""); err != nil {
		t.Fatalf("g1 register: %v", err)
	}
	if err := g2.Register(context.Background(), ""); err != nil {
		t.Fatalf("g2 register: %v", err)
	}
	g1.SyncOnce(context.Background())

	n, ok := g1.Route("user-42")
	if !ok {
		t.Fatal("expected route hit")
	}
	if n.ID != "n1" && n.ID != "n2" {
		t.Fatalf("unexpected node %s", n.ID)
	}
	if n.Addr == "" {
		t.Fatal("node addr empty")
	}

	// 注销 n2 并重新同步后，路由绝不应再返回 n2
	if err := g2.Deregister(context.Background()); err != nil {
		t.Fatalf("deregister n2: %v", err)
	}
	g1.SyncOnce(context.Background())
	for i := 0; i < 100; i++ {
		rn, ok := g1.Route("k-" + strconv.Itoa(i))
		if !ok {
			t.Fatalf("route miss")
		}
		if rn.ID == "n2" {
			t.Fatalf("routed to deregistered n2")
		}
	}
}

func TestGatewayStartBackground(t *testing.T) {
	reg := NewMemoryRegistry()
	cfg := appcfg.ConnectionGatewayConfig{SelfID: "n1", AdvertiseAddr: "10.0.0.1:8080", HeartbeatSec: 1, NodeTTLSec: 3, HashReplicas: 10}
	g := New(cfg, reg).WithLogger(zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	go g.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	nodes := g.Nodes()
	if len(nodes) != 1 || nodes[0].ID != "n1" {
		t.Fatalf("expected self node present, got %v", nodes)
	}
	cancel()
}
