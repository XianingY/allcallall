package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type CounterStore struct {
	mu       sync.RWMutex
	counters map[string]int64
}

func NewCounterStore() *CounterStore {
	return &CounterStore{counters: make(map[string]int64)}
}

func (s *CounterStore) Inc(name string) {
	s.Add(name, 1)
}

func (s *CounterStore) Add(name string, delta int64) {
	if strings.TrimSpace(name) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[name] += delta
}

func (s *CounterStore) Set(name string, value int64) {
	if strings.TrimSpace(name) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[name] = value
}

func (s *CounterStore) Snapshot() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int64, len(s.counters))
	for key, value := range s.counters {
		out[key] = value
	}
	return out
}

func (s *CounterStore) RenderPrometheus() string {
	snapshot := s.Snapshot()
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		metricType := "counter"
		if strings.HasSuffix(key, "_backlog") {
			metricType = "gauge"
		}
		builder.WriteString(fmt.Sprintf("# TYPE %s %s\n", key, metricType))
		builder.WriteString(fmt.Sprintf("%s %d\n", key, snapshot[key]))
	}
	return builder.String()
}

type NoopRecorder struct{}

func (n *NoopRecorder) Inc(name string) {}
func (n *NoopRecorder) Add(name string, delta int64) {}
func (n *NoopRecorder) Set(name string, value int64) {}

func NewNoopRecorder() Recorder {
	return &NoopRecorder{}
}

