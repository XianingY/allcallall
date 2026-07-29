package mq

import (
	"context"
	"testing"
)

func TestParseBrokers(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"single", "localhost:9092", []string{"localhost:9092"}},
		{"multiple comma separated", "a:9092,b:9092,c:9092", []string{"a:9092", "b:9092", "c:9092"}},
		{"trims surrounding whitespace", " a:9092 , b:9092 ", []string{"a:9092", "b:9092"}},
		{"drops blank entries", "a:9092,,b:9092,", []string{"a:9092", "b:9092"}},
		{"whitespace only yields empty", "  ,  , ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBrokers(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseBrokers(%q) = %v, want %v", tt.raw, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("ParseBrokers(%q)[%d] = %q, want %q", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMemoryBrokerEnqueueDequeueSameMessage(t *testing.T) {
	broker := NewMemoryBroker()
	prod := broker.Producer()
	cons := broker.Consumer("topic-a")

	msg := Message{
		Key:     []byte("key-1"),
		Value:   []byte("hello"),
		Headers: map[string]string{"h": "v"},
	}
	if err := prod.Publish(context.Background(), "topic-a", msg); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	got, err := cons.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if got.Topic != "topic-a" {
		t.Errorf("got topic %q, want %q", got.Topic, "topic-a")
	}
	if string(got.Key) != "key-1" {
		t.Errorf("got key %q, want %q", string(got.Key), "key-1")
	}
	if string(got.Value) != "hello" {
		t.Errorf("got value %q, want %q", string(got.Value), "hello")
	}
	if got.Headers["h"] != "v" {
		t.Errorf("got header h=%q, want %q", got.Headers["h"], "v")
	}

	// Second fetch should be exhausted.
	if _, err := cons.Fetch(context.Background()); err != ErrNoMessages {
		t.Errorf("expected ErrNoMessages, got %v", err)
	}
}

func TestMemoryBrokerCommitIdempotent(t *testing.T) {
	broker := NewMemoryBroker()
	cons := broker.Consumer("topic-b")

	msg := Message{Value: []byte("x")}
	// Commit must succeed even with no prior fetch and when called repeatedly
	// (in-memory consumer commit is a no-op and therefore idempotent).
	if err := cons.Commit(context.Background(), msg); err != nil {
		t.Fatalf("first Commit returned error: %v", err)
	}
	if err := cons.Commit(context.Background(), msg); err != nil {
		t.Fatalf("repeated Commit returned error: %v", err)
	}
}
