package collaboration

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/storage"
	"github.com/allcallall/backend/internal/transcription"
	"github.com/allcallall/backend/internal/user"
)

var (
	ErrOrganizationAccessDenied  = errors.New("organization access denied")
	ErrConversationAccessDenied  = errors.New("conversation access denied")
	ErrRoomAccessDenied          = errors.New("room access denied")
	ErrRoomParticipantLimit      = errors.New("room participant limit reached")
	ErrRecordingNotAllowed       = errors.New("recording not allowed")
	ErrTranscriptionNotRetryable = errors.New("recording transcription is not retryable")
	ErrInviteEmailMismatch       = errors.New("invite email mismatch")
)

type EventPublisher interface {
	PublishToUser(ctx context.Context, event RealtimeEventRecord) error
}



type redisCacheClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type Service struct {
	db                  *gorm.DB
	users               *user.Service
	publisher           EventPublisher
	media               *media.Engine
	storage             storage.RecordingStorage
	metrics             metrics.Recorder
	adminSummaryCache   redisCacheClient
	outbox              *events.Store
	transcriber         transcription.Provider
	maxRoomParticipants int
	trickleICE          bool
	roomOrgs            *roomOrgRegistry
	logger              zerolog.Logger
}

func NewService(db *gorm.DB, users *user.Service) *Service {
	maxRoomParticipants := 6
	if configured, err := strconv.Atoi(strings.TrimSpace(os.Getenv("ROOM_MAX_PARTICIPANTS"))); err == nil && configured > 0 {
		maxRoomParticipants = configured
	}
	svc := &Service{
		db:                  db,
		users:               users,
		outbox:              events.NewStore(db),
		maxRoomParticipants: maxRoomParticipants,
		trickleICE:          parseBoolEnv("ROOM_TRICKLE_ICE", false),
		roomOrgs:            newRoomOrgRegistry(),
	}
	svc.metrics = metrics.NewCounterStore()
	svc.logger = zerolog.Nop()
	if localStorage, err := storage.NewRecordingStorage(storage.Config{Driver: storage.DriverLocal}); err == nil {
		svc.storage = localStorage
	}
	return svc
}

func (s *Service) WithMaxRoomParticipants(limit int) *Service {
	if limit > 0 {
		s.maxRoomParticipants = limit
	}
	return s
}

func (s *Service) WithPublisher(publisher EventPublisher) {
	s.publisher = publisher
}

// WithLogger attaches a structured logger used for best-effort warnings on
// non-fatal errors that must not be silently swallowed.
func (s *Service) WithLogger(logger zerolog.Logger) {
	s.logger = logger
}

func (s *Service) WithMediaEngine(engine *media.Engine) {
	s.media = engine
	s.wireTrickleICE()
}

// WithTrickleICE toggles out-of-band delivery of server side ICE candidates.
// It must be called before WithMediaEngine to take effect on the sink wiring.
func (s *Service) WithTrickleICE(enabled bool) *Service {
	s.trickleICE = enabled
	s.wireTrickleICE()
	return s
}

func parseBoolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func (s *Service) WithRecordingStorage(recordingStorage storage.RecordingStorage) {
	if recordingStorage != nil {
		s.storage = recordingStorage
	}
}

func (s *Service) WithTranscriptionProvider(provider transcription.Provider) {
	s.transcriber = provider
}

func (s *Service) WithMetrics(counters metrics.Recorder) {
	if counters != nil {
		s.metrics = counters
	}
}

func (s *Service) WithAdminSummaryCache(client redisCacheClient) {
	if client != nil {
		s.adminSummaryCache = client
	}
}

func (s *Service) WithOutbox(outbox *events.Store) {
	s.outbox = outbox
}

func (s *Service) ListRealtimeEventsSince(ctx context.Context, organizationID, userID, sinceID uint64, limit int) ([]RealtimeEventRecord, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	return NewRealtimeEventStore(s.db).ListSince(ctx, organizationID, userID, sinceID, limit)
}

func (s *Service) publishConversationPatchUpdate(ctx context.Context, organizationID, conversationID uint64, changes map[string]any) {
	normalized := map[string]any{}
	for key, value := range changes {
		switch typed := value.(type) {
		case string:
			normalized[key] = typed
		default:
			normalized[key] = value
		}
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "conversation.updated", map[string]any{
		"conversation_id": conversationID,
		"changed_fields":  mapKeys(normalized),
		"changes":         normalized,
	})
}

func (s *Service) publishRoomMemberUpdated(ctx context.Context, organizationID, roomID uint64, member RoomMemberSummary) {
	payload := map[string]any{
		"room_id": roomID,
		"member":  member,
	}
	s.publishRoomEvent(ctx, organizationID, roomID, "room.member.updated", payload)
	s.publishRoomEvent(ctx, organizationID, roomID, "room.media.updated", payload)
}

func (s *Service) publishRoomStateUpdated(ctx context.Context, organizationID uint64, state *RoomState, eventType string) {
	if state == nil {
		return
	}
	payload := map[string]any{
		"room_id":             state.Room.ID,
		"event_type":          eventType,
		"status":              state.Room.Status,
		"participant_count":   state.ParticipantCount,
		"is_active":           state.IsActive,
		"has_recording":       state.HasRecording,
		"latest_recording_id": state.LatestRecordingID,
	}
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.state.updated", payload)
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.updated", payload)
}

func (s *Service) publishRoomRecordingUpdated(ctx context.Context, organizationID uint64, state *RoomState, recordingID uint64, eventType string) {
	if state == nil {
		return
	}
	payload := map[string]any{
		"room_id":             state.Room.ID,
		"event_type":          eventType,
		"participant_count":   state.ParticipantCount,
		"is_active":           state.IsActive,
		"has_recording":       true,
		"latest_recording_id": recordingID,
	}
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.recording.updated", payload)
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.updated", payload)
}

func (s *Service) publishRoomEnded(ctx context.Context, organizationID uint64, state *RoomState) {
	if state == nil {
		return
	}
	payload := map[string]any{
		"room_id":             state.Room.ID,
		"event_type":          "meeting.ended",
		"status":              state.Room.Status,
		"participant_count":   state.ParticipantCount,
		"is_active":           state.IsActive,
		"has_recording":       state.HasRecording,
		"latest_recording_id": state.LatestRecordingID,
	}
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.ended", payload)
	s.publishRoomEvent(ctx, organizationID, state.Room.ID, "room.updated", payload)
}
