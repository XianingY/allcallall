package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
)

type benchConfig struct {
	Events       int
	Recipients   int
	ReplayWindow int
	ReplayLimit  int
	KeepDB       bool
}

type benchOutput struct {
	StartedAt          string       `json:"started_at"`
	Events             int          `json:"events"`
	Recipients         int          `json:"recipients"`
	TargetUserID       uint64       `json:"target_user_id"`
	TotalEventsWritten int64        `json:"total_events_written"`
	TargetEvents       int          `json:"target_events"`
	ReplaySinceID      uint64       `json:"replay_since_id"`
	ReplayLimit        int          `json:"replay_limit"`
	ReplayWindow       int          `json:"replay_window"`
	ReplayedEvents     int          `json:"replayed_events"`
	ExpectedReplayed   int          `json:"expected_replayed"`
	ScopedCorrectly    bool         `json:"scoped_correctly"`
	MonotonicIDs       bool         `json:"monotonic_ids"`
	MonotonicSequences bool         `json:"monotonic_sequences"`
	SequenceMismatch   int          `json:"sequence_mismatch"`
	TotalDurationMs    int64        `json:"total_duration_ms"`
	WriteLatency       latencyStats `json:"write_latency"`
	ReplayLatency      latencyStats `json:"replay_latency"`
	DatabasePath       string       `json:"database_path,omitempty"`
}

type latencyStats struct {
	Count int   `json:"count"`
	MinMs int64 `json:"min_ms"`
	P50Ms int64 `json:"p50_ms"`
	P95Ms int64 `json:"p95_ms"`
	MaxMs int64 `json:"max_ms"`
}

func main() {
	cfg := parseFlags()
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "realtime replay bench failed: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() benchConfig {
	cfg := benchConfig{}
	flag.IntVar(&cfg.Events, "events", 2000, "total realtime events to write")
	flag.IntVar(&cfg.Recipients, "recipients", 10, "number of recipient users to distribute events across")
	flag.IntVar(&cfg.ReplayWindow, "replay-window", 120, "target-user events to replay from the tail")
	flag.IntVar(&cfg.ReplayLimit, "replay-limit", 100, "ListSince replay limit")
	flag.BoolVar(&cfg.KeepDB, "keep-db", false, "keep the temporary sqlite database and print its path")
	flag.Parse()
	if cfg.Events <= 0 {
		cfg.Events = 2000
	}
	if cfg.Recipients <= 0 {
		cfg.Recipients = 10
	}
	if cfg.ReplayWindow <= 0 {
		cfg.ReplayWindow = 120
	}
	if cfg.ReplayLimit <= 0 {
		cfg.ReplayLimit = 100
	}
	return cfg
}

func run(ctx context.Context, cfg benchConfig, writer io.Writer) error {
	started := time.Now().UTC()
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("allcallall-realtime-replay-bench-%d.db", started.UnixNano()))
	if !cfg.KeepDB {
		defer func() {
			_ = os.Remove(dbPath)
		}()
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.AutoMigrate(&models.ChatEvent{}); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}

	store := collaboration.NewRealtimeEventStore(db)
	targetUserID := uint64(7000)
	targetIDs := make([]uint64, 0, cfg.Events/cfg.Recipients+1)
	writeDurations := make([]time.Duration, 0, cfg.Events)
	for i := 0; i < cfg.Events; i++ {
		userID := targetUserID + uint64(i%cfg.Recipients)
		startWrite := time.Now()
		record, err := store.Create(ctx, 1, userID, "bench.message.created", map[string]any{
			"index":   i,
			"user_id": userID,
		})
		if err != nil {
			return fmt.Errorf("create event %d: %w", i, err)
		}
		writeDurations = append(writeDurations, time.Since(startWrite))
		if userID == targetUserID {
			targetIDs = append(targetIDs, record.ID)
		}
	}

	sinceID := uint64(0)
	if len(targetIDs) > cfg.ReplayWindow {
		sinceID = targetIDs[len(targetIDs)-cfg.ReplayWindow-1]
	}
	replayDurations := make([]time.Duration, 0, 1)
	startReplay := time.Now()
	replayed, err := store.ListSince(ctx, 1, targetUserID, sinceID, cfg.ReplayLimit)
	if err != nil {
		return fmt.Errorf("list replay events: %w", err)
	}
	replayDurations = append(replayDurations, time.Since(startReplay))

	expected := len(targetIDs)
	if sinceID != 0 {
		expected = countIDsAfter(targetIDs, sinceID)
	}
	if expected > cfg.ReplayLimit {
		expected = cfg.ReplayLimit
	}

	output := benchOutput{
		StartedAt:          started.Format(time.RFC3339Nano),
		Events:             cfg.Events,
		Recipients:         cfg.Recipients,
		TargetUserID:       targetUserID,
		TargetEvents:       len(targetIDs),
		ReplaySinceID:      sinceID,
		ReplayLimit:        cfg.ReplayLimit,
		ReplayWindow:       cfg.ReplayWindow,
		ReplayedEvents:     len(replayed),
		ExpectedReplayed:   expected,
		ScopedCorrectly:    scopedToUser(replayed, targetUserID),
		MonotonicIDs:       monotonicIDs(replayed),
		MonotonicSequences: monotonicSequences(replayed),
		SequenceMismatch:   sequenceMismatchCount(replayed),
		TotalDurationMs:    time.Since(started).Milliseconds(),
		WriteLatency:       summarizeLatency(writeDurations),
		ReplayLatency:      summarizeLatency(replayDurations),
	}
	if cfg.KeepDB {
		output.DatabasePath = dbPath
	}
	if err := db.WithContext(ctx).Model(&models.ChatEvent{}).Count(&output.TotalEventsWritten).Error; err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func countIDsAfter(ids []uint64, sinceID uint64) int {
	count := 0
	for _, id := range ids {
		if id > sinceID {
			count++
		}
	}
	return count
}

func scopedToUser(events []collaboration.RealtimeEventRecord, userID uint64) bool {
	for _, event := range events {
		if event.UserID != userID {
			return false
		}
	}
	return true
}

func monotonicIDs(events []collaboration.RealtimeEventRecord) bool {
	for i := 1; i < len(events); i++ {
		if events[i].ID <= events[i-1].ID {
			return false
		}
	}
	return true
}

func monotonicSequences(events []collaboration.RealtimeEventRecord) bool {
	for i := 1; i < len(events); i++ {
		if events[i].Sequence <= events[i-1].Sequence {
			return false
		}
	}
	return true
}

func sequenceMismatchCount(events []collaboration.RealtimeEventRecord) int {
	count := 0
	for _, event := range events {
		if event.Sequence != event.ID {
			count++
		}
	}
	return count
}

func summarizeLatency(items []time.Duration) latencyStats {
	if len(items) == 0 {
		return latencyStats{}
	}
	values := make([]int64, len(items))
	for i, item := range items {
		values[i] = item.Milliseconds()
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i] < values[j]
	})
	return latencyStats{
		Count: len(values),
		MinMs: values[0],
		P50Ms: percentile(values, 0.50),
		P95Ms: percentile(values, 0.95),
		MaxMs: values[len(values)-1],
	}
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	return values[index]
}
