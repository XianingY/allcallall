package connectionregistry

import (
	"strconv"
	"testing"
)

func TestHashRingEmpty(t *testing.T) {
	r := NewHashRing(10)
	if _, ok := r.Get("anything"); ok {
		t.Fatalf("expected false for empty ring")
	}
}

func TestHashRingAddGetDeterministic(t *testing.T) {
	r := NewHashRing(50)
	r.Add("node-a", "node-b", "node-c")
	got, ok := r.Get("user-123")
	if !ok {
		t.Fatal("expected a node")
	}
	again, _ := r.Get("user-123")
	if again != got {
		t.Fatalf("ring not deterministic: %s != %s", got, again)
	}
	valid := map[string]bool{"node-a": true, "node-b": true, "node-c": true}
	if !valid[got] {
		t.Fatalf("returned unknown node %s", got)
	}
}

func TestHashRingDistribution(t *testing.T) {
	r := NewHashRing(100)
	r.Add("n1", "n2", "n3")
	counts := map[string]int{}
	for i := 0; i < 3000; i++ {
		n, ok := r.Get("key-" + strconv.Itoa(i))
		if !ok {
			t.Fatal("no node")
		}
		counts[n]++
	}
	if len(counts) != 3 {
		t.Fatalf("expected distribution across 3 nodes, got %d: %v", len(counts), counts)
	}
	for n, c := range counts {
		if c == 0 {
			t.Fatalf("node %s got zero keys", n)
		}
	}
}

func TestHashRingRemove(t *testing.T) {
	r := NewHashRing(50)
	r.Add("a", "b")
	before, _ := r.Get("xyz")
	r.Remove("a", "b")
	if _, ok := r.Get("xyz"); ok {
		t.Fatalf("expected no node after removing all")
	}
	r.Add(before)
	if got, ok := r.Get("xyz"); !ok || got != before {
		t.Fatalf("expected %s after re-add, got %s ok=%v", before, got, ok)
	}
}
