package presence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/user"
)

const (
	presenceKeyPrefix = "presence:user:"
	deviceKeyPrefix   = "presence:dev:"
	manualKeyPrefix   = "presence:manual:"
	eventsChannel     = "presence:events"

	// heartbeatWindow is the sliding TTL applied to an online presence entry.
	// A process crash that never calls SetOffline leaves the key to expire here,
	// so a dead node cannot pin a user as online forever (the old 24h TTL did).
	defaultHeartbeatWindow = 90 * time.Second
	// manualTTL bounds how long an explicit busy/dnd/away state lingers if the
	// clearing event (e.g. call ended) is missed.
	defaultManualTTL = 24 * time.Hour
)

// State 表示用户的在线状态。
// State enumerates the user presence states.
type State string

const (
	StateOnline  State = "online"
	StateAway    State = "away"
	StateBusy    State = "busy"
	StateDND     State = "dnd"
	StateOffline State = "offline"
)

// manualStates are the states a user can set explicitly. online/offline are
// derived from device heartbeats and are never stored as manual overrides.
var manualStates = map[State]bool{
	StateAway: true,
	StateBusy: true,
	StateDND:  true,
}

// DevicePresence 表示单个设备的活跃信息。
// DevicePresence captures liveness for one connected device.
type DevicePresence struct {
	DeviceID      string    `json:"device_id"`
	Platform      string    `json:"platform"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

// Status 表示用户聚合后的在线状态（多端合并）。
// Status is the aggregated, multi-device presence of a user.
type Status struct {
	Email         string           `json:"email"`
	State         State            `json:"state"`
	Online        bool             `json:"online"`
	LastSeen      time.Time        `json:"last_seen"`
	Devices       []DevicePresence `json:"devices,omitempty"`
	CustomMessage string           `json:"custom_message,omitempty"`
}

// PresenceEvent 通过 Redis pub/sub 广播的状态变更事件。
// PresenceEvent is broadcast over Redis when a user's state changes.
type PresenceEvent struct {
	Email         string    `json:"email"`
	State         State     `json:"state"`
	LastSeen      time.Time `json:"last_seen"`
	DeviceID      string    `json:"device_id,omitempty"`
	CustomMessage string    `json:"custom_message,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// Manager 管理用户在线状态（Redis 支撑，支持多设备与心跳）。
// Manager handles presence updates backed by Redis.
type Manager struct {
	redis           *redis.Client
	logger          zerolog.Logger
	userSvc         *user.Service
	heartbeatWindow time.Duration
	manualTTL       time.Duration
	publishEnabled  bool
}

// NewManager 创建 presence 管理器。
// NewManager returns a presence manager. Options allow tuning the heartbeat
// window and manual-state TTL for tests or specialized deployments.
func NewManager(rdb *redis.Client, log zerolog.Logger, userSvc *user.Service, opts ...func(*Manager)) *Manager {
	m := &Manager{
		redis:           rdb,
		logger:          log.With().Str("component", "presence_manager").Logger(),
		userSvc:         userSvc,
		heartbeatWindow: defaultHeartbeatWindow,
		manualTTL:       defaultManualTTL,
		publishEnabled:  true,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// WithHeartbeatWindow overrides the sliding TTL for online presence.
func WithHeartbeatWindow(d time.Duration) func(*Manager) {
	return func(m *Manager) {
		if d > 0 {
			m.heartbeatWindow = d
		}
	}
}

// WithManualTTL overrides how long an explicit busy/dnd/away state lingers.
func WithManualTTL(d time.Duration) func(*Manager) {
	return func(m *Manager) {
		if d > 0 {
			m.manualTTL = d
		}
	}
}

func (m *Manager) userKey(email string) string { return presenceKeyPrefix + email }
func (m *Manager) deviceKey(email, deviceID string) string {
	return deviceKeyPrefix + email + ":" + deviceID
}
func (m *Manager) manualKey(email string) string { return manualKeyPrefix + email }

// Heartbeat 续活一个设备的连接，并将其计入在线状态。
// Heartbeat renews a device's liveness and marks the user online (unless a
// manual override such as busy/dnd is active).
func (m *Manager) Heartbeat(ctx context.Context, email, deviceID, platform string) error {
	if deviceID == "" {
		deviceID = "default"
	}
	now := time.Now()
	dev := DevicePresence{DeviceID: deviceID, Platform: platform, LastHeartbeat: now}
	devBytes, err := json.Marshal(dev)
	if err != nil {
		return err
	}
	if err := m.redis.Set(ctx, m.deviceKey(email, deviceID), devBytes, m.heartbeatWindow).Err(); err != nil {
		return err
	}
	return m.recompute(ctx, email, &now)
}

// SetOnline 标记用户在线（向后兼容：单次默认设备心跳）。
// SetOnline marks the user online via a default-device heartbeat.
func (m *Manager) SetOnline(ctx context.Context, email string) error {
	return m.Heartbeat(ctx, email, "default", "")
}

// SetOffline 标记用户离线，清除所有设备心跳并回写 last_seen。
// SetOffline clears every device heartbeat and persists last seen.
func (m *Manager) SetOffline(ctx context.Context, email string) error {
	members, err := m.deviceKeys(ctx, email)
	if err != nil {
		return err
	}
	pipe := m.redis.Pipeline()
	for _, k := range members {
		pipe.Del(ctx, k)
	}
	pipe.Del(ctx, m.manualKey(email))
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	now := time.Now()
	status := Status{Email: email, State: StateOffline, Online: false, LastSeen: now}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	// Persist as offline with a long TTL so a clean logout stays visible until
	// the next heartbeat, while a crash still degrades to offline via expiry.
	if err := m.redis.Set(ctx, m.userKey(email), data, m.manualTTL).Err(); err != nil {
		return err
	}
	m.publish(ctx, email, status, "")
	return m.persistLastSeen(ctx, email, now)
}

// UpdateLastSeen 仅更新 last_seen，不改变在线/状态（不写库，与原行为一致）。
// UpdateLastSeen refreshes the timestamp while keeping the current state.
func (m *Manager) UpdateLastSeen(ctx context.Context, email string) error {
	prev := m.peek(ctx, email)
	now := time.Now()
	if prev.State == "" {
		// No existing presence record: nothing to refresh in Redis.
		return nil
	}
	status := prev
	status.LastSeen = now
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return m.redis.Set(ctx, m.userKey(email), data, m.heartbeatWindow).Err()
}

// SetManualState 设置用户主动状态（away/busy/dnd），覆盖设备推导的在线态。
// SetManualState applies an explicit user state that overrides the derived
// device-online state until cleared.
func (m *Manager) SetManualState(ctx context.Context, email string, state State, custom string) error {
	if !manualStates[state] {
		return fmt.Errorf("presence: state %q is not a manual state", state)
	}
	now := time.Now()
	payload, err := json.Marshal(struct {
		State         State     `json:"state"`
		CustomMessage string    `json:"custom_message"`
		UpdatedAt     time.Time `json:"updated_at"`
	}{state, custom, now})
	if err != nil {
		return err
	}
	if err := m.redis.Set(ctx, m.manualKey(email), payload, m.manualTTL).Err(); err != nil {
		return err
	}
	return m.recompute(ctx, email, &now)
}

// ClearManualState 清除主动状态，恢复为设备推导的态。
// ClearManualState removes the explicit override, falling back to the derived
// device-online state.
func (m *Manager) ClearManualState(ctx context.Context, email string) error {
	if err := m.redis.Del(ctx, m.manualKey(email)).Err(); err != nil {
		return err
	}
	return m.recompute(ctx, email, nil)
}

// GetStatus 获取单个用户聚合状态。
// GetStatus fetches aggregated presence for a single email.
func (m *Manager) GetStatus(ctx context.Context, email string) (Status, error) {
	raw, err := m.redis.Get(ctx, m.userKey(email)).Result()
	if err == redis.Nil {
		return Status{Email: email, State: StateOffline, Online: false}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var s Status
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Status{}, err
	}
	return normalize(s), nil
}

// GetStatuses 批量获取用户状态。
// GetStatuses fetches presence for multiple emails.
func (m *Manager) GetStatuses(ctx context.Context, emails []string) (map[string]Status, error) {
	result := make(map[string]Status, len(emails))
	if len(emails) == 0 {
		return result, nil
	}
	keys := make([]string, 0, len(emails))
	for _, email := range emails {
		keys = append(keys, m.userKey(email))
	}
	values, err := m.redis.MGet(ctx, keys...).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	for i, raw := range values {
		email := emails[i]
		if raw == nil {
			result[email] = Status{Email: email, State: StateOffline, Online: false}
			continue
		}
		var s Status
		if err := json.Unmarshal([]byte(raw.(string)), &s); err != nil {
			result[email] = Status{Email: email, State: StateOffline, Online: false}
			continue
		}
		result[email] = normalize(s)
	}
	return result, nil
}

// recompute 计算并写回聚合状态，并在变化时广播。
// recompute derives the effective state, persists it, and broadcasts on change.
func (m *Manager) recompute(ctx context.Context, email string, lastSeen *time.Time) error {
	state, custom, err := m.computeState(ctx, email)
	if err != nil {
		return err
	}
	devs, err := m.listDevices(ctx, email)
	if err != nil {
		return err
	}
	prev := m.peek(ctx, email)
	ls := prev.LastSeen
	if lastSeen != nil && !lastSeen.IsZero() {
		ls = *lastSeen
	}
	if ls.IsZero() {
		ls = time.Now()
	}
	status := Status{
		Email:         email,
		State:         state,
		Online:        state != StateOffline,
		LastSeen:      ls,
		Devices:       devs,
		CustomMessage: custom,
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	if err := m.redis.Set(ctx, m.userKey(email), data, m.heartbeatWindow).Err(); err != nil {
		return err
	}
	m.publish(ctx, email, status, "")
	return nil
}

func (m *Manager) computeState(ctx context.Context, email string) (State, string, error) {
	manual, custom, err := m.getManual(ctx, email)
	if err != nil {
		return "", "", err
	}
	if manual != "" && manual != StateOffline {
		return manual, custom, nil
	}
	live, err := m.hasLiveDevice(ctx, email)
	if err != nil {
		return "", "", err
	}
	if live {
		return StateOnline, "", nil
	}
	return StateOffline, "", nil
}

func (m *Manager) getManual(ctx context.Context, email string) (State, string, error) {
	raw, err := m.redis.Get(ctx, m.manualKey(email)).Result()
	if err == redis.Nil {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	var v struct {
		State         State     `json:"state"`
		CustomMessage string    `json:"custom_message"`
		UpdatedAt     time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return "", "", nil
	}
	return v.State, v.CustomMessage, nil
}

func (m *Manager) deviceKeys(ctx context.Context, email string) ([]string, error) {
	var keys []string
	iter := m.redis.Scan(ctx, 0, m.deviceKey(email, "*"), 0).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	return keys, iter.Err()
}

func (m *Manager) hasLiveDevice(ctx context.Context, email string) (bool, error) {
	iter := m.redis.Scan(ctx, 0, m.deviceKey(email, "*"), 0).Iterator()
	if iter.Next(ctx) {
		return true, nil
	}
	return false, iter.Err()
}

func (m *Manager) listDevices(ctx context.Context, email string) ([]DevicePresence, error) {
	keys, err := m.deviceKeys(ctx, email)
	if err != nil {
		return nil, err
	}
	out := make([]DevicePresence, 0, len(keys))
	for _, k := range keys {
		raw, err := m.redis.Get(ctx, k).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var d DevicePresence
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastHeartbeat.After(out[j].LastHeartbeat)
	})
	return out, nil
}

func (m *Manager) peek(ctx context.Context, email string) Status {
	raw, err := m.redis.Get(ctx, m.userKey(email)).Result()
	if err != nil {
		return Status{}
	}
	var s Status
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Status{}
	}
	return normalize(s)
}

func (m *Manager) persistLastSeen(ctx context.Context, email string, ts time.Time) error {
	if m.userSvc == nil {
		return nil
	}
	u, err := m.userSvc.GetByEmail(ctx, email)
	if err != nil {
		m.logger.Warn().Err(err).Str("email", email).Msg("presence: failed to load user for last seen")
		return nil
	}
	if err := m.userSvc.UpdateLastSeen(ctx, u.ID, &ts); err != nil {
		m.logger.Warn().Err(err).Uint64("user_id", u.ID).Msg("presence: failed to persist last seen")
	}
	return nil
}

func (m *Manager) publish(ctx context.Context, email string, status Status, deviceID string) {
	if !m.publishEnabled || m.redis == nil {
		return
	}
	ev := PresenceEvent{
		Email:         email,
		State:         status.State,
		LastSeen:      status.LastSeen,
		DeviceID:      deviceID,
		CustomMessage: status.CustomMessage,
		Timestamp:     time.Now(),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if err := m.redis.Publish(ctx, eventsChannel, data).Err(); err != nil {
		m.logger.Debug().Err(err).Msg("presence: failed to publish event")
	}
}

// normalize 补齐缺失字段，保证旧格式 JSON 仍可解析。
// normalize backfills missing fields (e.g. legacy JSON without a state).
func normalize(s Status) Status {
	if s.State == "" {
		if s.Online {
			s.State = StateOnline
		} else {
			s.State = StateOffline
		}
	}
	s.Online = s.State != StateOffline
	return s
}
