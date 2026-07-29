package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/handlers"
	"github.com/allcallall/backend/internal/models"
)

type benchConfig struct {
	Events       int
	Recipients   int
	ReplayWindow int
	ReplayLimit  int
	Clients      int
	Timeout      time.Duration
}

type benchOutput struct {
	StartedAt             string       `json:"started_at"`
	Events                int          `json:"events"`
	Recipients            int          `json:"recipients"`
	Clients               int          `json:"clients"`
	TargetUserID          uint64       `json:"target_user_id"`
	OrganizationID        uint64       `json:"organization_id"`
	TargetEvents          int          `json:"target_events"`
	ReplaySinceID         uint64       `json:"replay_since_id"`
	ReplayLimit           int          `json:"replay_limit"`
	ReplayWindow          int          `json:"replay_window"`
	ExpectedPerClient     int          `json:"expected_per_client"`
	UpgradeSuccess        int          `json:"upgrade_success"`
	UpgradeErrors         int          `json:"upgrade_errors"`
	ClientErrors          int          `json:"client_errors"`
	TotalReplayed         int          `json:"total_replayed"`
	CompleteClients       int          `json:"complete_clients"`
	ScopedCorrectly       bool         `json:"scoped_correctly"`
	MonotonicIDs          bool         `json:"monotonic_ids"`
	MonotonicSequences    bool         `json:"monotonic_sequences"`
	DuplicateEvents       int          `json:"duplicate_events"`
	SequenceMismatch      int          `json:"sequence_mismatch"`
	ConnectToFirstLatency latencyStats `json:"connect_to_first_latency"`
	ConnectToLastLatency  latencyStats `json:"connect_to_last_latency"`
	TotalDurationMs       int64        `json:"total_duration_ms"`
}

type clientResult struct {
	upgradeOK      bool
	err            error
	events         []wsEvent
	firstLatency   time.Duration
	lastLatency    time.Duration
	completeReplay bool
}

type wsEvent struct {
	EventID        uint64          `json:"event_id"`
	Sequence       uint64          `json:"sequence"`
	Event          string          `json:"event"`
	OrganizationID uint64          `json:"organization_id"`
	Payload        json.RawMessage `json:"payload"`
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
		_, _ = fmt.Fprintf(os.Stderr, "chat websocket replay bench failed: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() benchConfig {
	var timeoutMs int
	cfg := benchConfig{}
	flag.IntVar(&cfg.Events, "events", 2000, "total realtime events to seed")
	flag.IntVar(&cfg.Recipients, "recipients", 10, "number of recipient users to distribute events across")
	flag.IntVar(&cfg.ReplayWindow, "replay-window", 120, "target-user events to replay from the tail")
	flag.IntVar(&cfg.ReplayLimit, "replay-limit", 100, "current websocket replay backlog limit")
	flag.IntVar(&cfg.Clients, "clients", 5, "concurrent websocket clients")
	flag.IntVar(&timeoutMs, "timeout-ms", 5000, "per-client replay timeout in milliseconds")
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
	if cfg.Clients <= 0 {
		cfg.Clients = 5
	}
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	cfg.Timeout = time.Duration(timeoutMs) * time.Millisecond
	return cfg
}

func run(ctx context.Context, cfg benchConfig, writer io.Writer) error {
	started := time.Now().UTC()
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("allcallall-chat-ws-replay-bench-%d.db", started.UnixNano()))
	defer func() {
		_ = os.Remove(dbPath)
	}()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.AutoMigrate(&models.Organization{}, &models.OrganizationMember{}, &models.ChatEvent{}); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	organizationID := uint64(1)
	targetUserID := uint64(7000)
	if err := seedMembership(ctx, db, organizationID, targetUserID); err != nil {
		return err
	}
	store := collaboration.NewRealtimeEventStore(db)
	targetIDs, err := seedRealtimeEvents(ctx, store, cfg, organizationID, targetUserID)
	if err != nil {
		return err
	}
	sinceID := uint64(0)
	if len(targetIDs) > cfg.ReplayWindow {
		sinceID = targetIDs[len(targetIDs)-cfg.ReplayWindow-1]
	}
	expectedPerClient := countIDsAfter(targetIDs, sinceID)
	if expectedPerClient > cfg.ReplayLimit {
		expectedPerClient = cfg.ReplayLimit
	}

	jwtManager, err := auth.NewManager(auth.Config{
		Secret:         "chat-ws-replay-bench-secret",
		Issuer:         "allcallall-interview-bench",
		AccessTokenTTL: time.Hour,
	})
	if err != nil {
		return err
	}
	token, err := jwtManager.GenerateAccessToken(targetUserID, "bench@example.com")
	if err != nil {
		return err
	}
	router := buildRouter(db, jwtManager)
	server := httptest.NewServer(router)
	defer server.Close()
	wsURL, err := buildWebSocketURL(server.URL, token, organizationID, sinceID)
	if err != nil {
		return err
	}

	results := runClients(wsURL, cfg.Clients, expectedPerClient, cfg.Timeout)
	output := buildOutput(started, cfg, organizationID, targetUserID, targetIDs, sinceID, expectedPerClient, results)
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func buildRouter(db *gorm.DB, jwtManager *auth.Manager) *gin.Engine {
	router := gin.New()
	log := zerolog.Nop()
	service := collaboration.NewService(db, nil)
	service.WithLogger(log)
	hub := collaboration.NewChatHub(nil, log)
	handler := handlers.NewCollaborationHandler(log, service, nil, hub)
	api := router.Group("/api/v1")
	protected := api.Group("")
	protected.Use(auth.Middleware(jwtManager))
	handler.RegisterProtectedRoutes(protected)
	handler.RegisterRealtimeRoutes(api, auth.Middleware(jwtManager))
	return router
}

func seedMembership(ctx context.Context, db *gorm.DB, organizationID, userID uint64) error {
	org := models.Organization{
		ID:        organizationID,
		Name:      "Replay Bench Org",
		Slug:      "replay-bench",
		CreatedBy: userID,
	}
	if err := db.WithContext(ctx).Create(&org).Error; err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	member := models.OrganizationMember{
		OrganizationID: organizationID,
		UserID:         userID,
		Role:           models.OrganizationRoleOwner,
		JoinedAt:       time.Now().UTC(),
	}
	if err := db.WithContext(ctx).Create(&member).Error; err != nil {
		return fmt.Errorf("create organization member: %w", err)
	}
	return nil
}

func seedRealtimeEvents(ctx context.Context, store *collaboration.RealtimeEventStore, cfg benchConfig, organizationID, targetUserID uint64) ([]uint64, error) {
	targetIDs := make([]uint64, 0, cfg.Events/cfg.Recipients+1)
	for i := 0; i < cfg.Events; i++ {
		userID := targetUserID + uint64(i%cfg.Recipients)
		record, err := store.Create(ctx, organizationID, userID, "bench.message.created", map[string]any{
			"index":   i,
			"user_id": userID,
		})
		if err != nil {
			return nil, fmt.Errorf("create event %d: %w", i, err)
		}
		if userID == targetUserID {
			targetIDs = append(targetIDs, record.ID)
		}
	}
	return targetIDs, nil
}

func buildWebSocketURL(serverURL, token string, organizationID, sinceID uint64) (string, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	parsed.Scheme = "ws"
	parsed.Path = "/api/v1/chat/ws"
	query := parsed.Query()
	query.Set("token", token)
	query.Set("organization_id", fmt.Sprintf("%d", organizationID))
	query.Set("since_id", fmt.Sprintf("%d", sinceID))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func runClients(wsURL string, clients, expected int, timeout time.Duration) []clientResult {
	results := make([]clientResult, clients)
	var wg sync.WaitGroup
	wg.Add(clients)
	for i := 0; i < clients; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i] = runClient(wsURL, expected, timeout)
		}()
	}
	wg.Wait()
	return results
}

func runClient(wsURL string, expected int, timeout time.Duration) clientResult {
	started := time.Now()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return clientResult{err: err}
	}
	defer conn.Close()
	result := clientResult{upgradeOK: true}
	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)
	for len(result.events) < expected {
		_, body, err := conn.ReadMessage()
		if err != nil {
			result.err = err
			return result
		}
		var event wsEvent
		if err := json.Unmarshal(body, &event); err != nil {
			result.err = err
			return result
		}
		if len(result.events) == 0 {
			result.firstLatency = time.Since(started)
		}
		result.events = append(result.events, event)
	}
	result.lastLatency = time.Since(started)
	result.completeReplay = len(result.events) == expected
	return result
}

func buildOutput(started time.Time, cfg benchConfig, organizationID, targetUserID uint64, targetIDs []uint64, sinceID uint64, expectedPerClient int, results []clientResult) benchOutput {
	output := benchOutput{
		StartedAt:          started.Format(time.RFC3339Nano),
		Events:             cfg.Events,
		Recipients:         cfg.Recipients,
		Clients:            cfg.Clients,
		TargetUserID:       targetUserID,
		OrganizationID:     organizationID,
		TargetEvents:       len(targetIDs),
		ReplaySinceID:      sinceID,
		ReplayLimit:        cfg.ReplayLimit,
		ReplayWindow:       cfg.ReplayWindow,
		ExpectedPerClient:  expectedPerClient,
		ScopedCorrectly:    true,
		MonotonicIDs:       true,
		MonotonicSequences: true,
		TotalDurationMs:    time.Since(started).Milliseconds(),
	}
	firstLatencies := make([]time.Duration, 0, len(results))
	lastLatencies := make([]time.Duration, 0, len(results))
	for _, result := range results {
		if result.upgradeOK {
			output.UpgradeSuccess++
		} else {
			output.UpgradeErrors++
		}
		if result.err != nil {
			output.ClientErrors++
		}
		if result.completeReplay {
			output.CompleteClients++
		}
		output.TotalReplayed += len(result.events)
		if result.firstLatency > 0 {
			firstLatencies = append(firstLatencies, result.firstLatency)
		}
		if result.lastLatency > 0 {
			lastLatencies = append(lastLatencies, result.lastLatency)
		}
		output.ScopedCorrectly = output.ScopedCorrectly && scopedToUser(result.events, targetUserID, organizationID)
		output.MonotonicIDs = output.MonotonicIDs && monotonicIDs(result.events)
		output.MonotonicSequences = output.MonotonicSequences && monotonicSequences(result.events)
		output.SequenceMismatch += sequenceMismatchCount(result.events)
		output.DuplicateEvents += duplicateEventCount(result.events)
	}
	output.ConnectToFirstLatency = summarizeLatency(firstLatencies)
	output.ConnectToLastLatency = summarizeLatency(lastLatencies)
	return output
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

func scopedToUser(events []wsEvent, userID, organizationID uint64) bool {
	for _, event := range events {
		if event.OrganizationID != organizationID {
			return false
		}
		var payload struct {
			UserID uint64 `json:"user_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.UserID != userID {
			return false
		}
	}
	return true
}

func monotonicIDs(events []wsEvent) bool {
	for i := 1; i < len(events); i++ {
		if events[i].EventID <= events[i-1].EventID {
			return false
		}
	}
	return true
}

func monotonicSequences(events []wsEvent) bool {
	for i := 1; i < len(events); i++ {
		if events[i].Sequence <= events[i-1].Sequence {
			return false
		}
	}
	return true
}

func sequenceMismatchCount(events []wsEvent) int {
	count := 0
	for _, event := range events {
		if event.Sequence != event.EventID {
			count++
		}
	}
	return count
}

func duplicateEventCount(events []wsEvent) int {
	seen := make(map[uint64]struct{}, len(events))
	duplicates := 0
	for _, event := range events {
		if _, ok := seen[event.EventID]; ok {
			duplicates++
			continue
		}
		seen[event.EventID] = struct{}{}
	}
	return duplicates
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
