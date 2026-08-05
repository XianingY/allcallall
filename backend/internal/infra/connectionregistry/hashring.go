package connectionregistry

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
)

// HashRing 一致哈希环：用虚拟节点把连接键映射到后端节点，
// 节点增减时仅少量键需要重新映射，适合连接层负载均衡。
type HashRing struct {
	replicas int
	mu       sync.RWMutex
	nodeSet  map[string]struct{}
	ring     []uint32
	members  map[uint32]string
}

// NewHashRing 构造哈希环。replicas 为每节点的虚拟节点数，默认 100。
func NewHashRing(replicas int) *HashRing {
	if replicas <= 0 {
		replicas = 100
	}
	return &HashRing{
		replicas: replicas,
		nodeSet:  make(map[string]struct{}),
		members:  make(map[uint32]string),
	}
}

func (r *HashRing) hashKey(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// Add 加入一个或多个节点（幂等）。
func (r *HashRing) Add(nodes ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range nodes {
		if n == "" {
			continue
		}
		r.nodeSet[n] = struct{}{}
	}
	r.rebuild()
}

// Remove 移除节点。
func (r *HashRing) Remove(nodes ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range nodes {
		delete(r.nodeSet, n)
	}
	r.rebuild()
}

// SetNodes 用给定节点集合整体重建环（内部加锁）。
func (r *HashRing) SetNodes(nodes []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodeSet = make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		if n == "" {
			continue
		}
		r.nodeSet[n] = struct{}{}
	}
	r.rebuild()
}

func (r *HashRing) rebuild() {
	r.ring = r.ring[:0]
	r.members = make(map[uint32]string, len(r.nodeSet)*r.replicas)
	for node := range r.nodeSet {
		for i := 0; i < r.replicas; i++ {
			h := r.hashKey(fmt.Sprintf("%s:%d", node, i))
			r.ring = append(r.ring, h)
			r.members[h] = node
		}
	}
	sort.Slice(r.ring, func(i, j int) bool { return r.ring[i] < r.ring[j] })
}

// Get 用一致哈希把 key 映射到某个节点 ID；无节点时返回 false。
func (r *HashRing) Get(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.ring) == 0 {
		return "", false
	}
	h := r.hashKey(key)
	idx := sort.Search(len(r.ring), func(i int) bool { return r.ring[i] >= h })
	if idx == len(r.ring) {
		idx = 0
	}
	return r.members[r.ring[idx]], true
}

// Nodes 返回当前环上的节点 ID（无序）。
func (r *HashRing) Nodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.nodeSet))
	for n := range r.nodeSet {
		out = append(out, n)
	}
	return out
}
