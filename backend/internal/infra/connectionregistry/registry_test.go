package connectionregistry

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// runRegistryContract 在两个实现（内存 / Redis）上跑同一套契约，确保语义一致。
func runRegistryContract(t *testing.T, newReg func() Registry) {
	t.Helper()
	ctx := context.Background()
	reg := newReg()
	node1 := Node{ID: "n1", Addr: "10.0.0.1:8080", Metadata: map[string]string{"region": "cn"}}

	if err := reg.Register(ctx, node1, time.Second); err != nil {
		t.Fatalf("register: %v", err)
	}
	active, err := reg.ListActive(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(active) != 1 || active[0].ID != "n1" || active[0].Addr != "10.0.0.1:8080" {
		t.Fatalf("expected 1 active n1, got %v", active)
	}

	if err := reg.Heartbeat(ctx, node1, time.Second); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	if err := reg.Deregister(ctx, "n1"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	active, _ = reg.ListActive(ctx)
	if len(active) != 0 {
		t.Fatalf("expected 0 after deregister, got %v", active)
	}
	// 重复注销安全
	if err := reg.Deregister(ctx, "n1"); err != nil {
		t.Fatalf("double deregister: %v", err)
	}
	// 空 ID 应报错
	if err := reg.Register(ctx, Node{}, time.Second); err == nil {
		t.Fatalf("expected error for empty id")
	}
}

func TestMemoryRegistryContract(t *testing.T) {
	runRegistryContract(t, func() Registry { return NewMemoryRegistry() })
}

func TestRedisRegistryContract(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	runRegistryContract(t, func() Registry { return NewRedisRegistry(client) })
}

func TestRedisRegistryExpiry(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	ctx := context.Background()
	reg := NewRedisRegistry(client)
	if err := reg.Register(ctx, Node{ID: "e1", Addr: "a"}, time.Second); err != nil {
		t.Fatalf("register: %v", err)
	}
	mr.FastForward(2 * time.Second)
	active, err := reg.ListActive(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected expired node gone, got %v", active)
	}
}
