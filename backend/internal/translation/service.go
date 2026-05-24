package translation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var (
	// ErrSessionLimitExceeded 用户并发会话数达到上限
	// ErrSessionLimitExceeded indicates per-user session quota reached.
	ErrSessionLimitExceeded = errors.New("translation session limit exceeded")
	// ErrBadStartRequest 启动参数非法
	// ErrBadStartRequest indicates invalid start payload.
	ErrBadStartRequest = errors.New("bad translation start request")
)

// Service 翻译服务
// Service manages translation sessions.
type Service struct {
	logger             zerolog.Logger
	provider           Provider
	maxSessionsPerUser int
	commercial         *commerce.Service
	users              *user.Service

	mu                 sync.Mutex
	sessionCountByUser map[string]int
}

type Dependencies struct {
	Commerce *commerce.Service
	Users    *user.Service
}

// NewService 创建翻译服务
// NewService initializes translation service.
func NewService(
	log zerolog.Logger,
	provider Provider,
	maxSessionsPerUser int,
	deps ...Dependencies,
) *Service {
	if maxSessionsPerUser <= 0 {
		maxSessionsPerUser = 2
	}
	var dep Dependencies
	if len(deps) > 0 {
		dep = deps[0]
	}
	return &Service{
		logger:             log.With().Str("component", "translation_service").Logger(),
		provider:           provider,
		maxSessionsPerUser: maxSessionsPerUser,
		commercial:         dep.Commerce,
		users:              dep.Users,
		sessionCountByUser: make(map[string]int),
	}
}

// StartSession 启动新会话
// StartSession validates request and opens provider stream.
func (s *Service) StartSession(ctx context.Context, owner string, req StartRequest) (*Session, error) {
	if s.provider == nil {
		return nil, errors.New("translation provider is not configured")
	}
	if err := validateStartRequest(req); err != nil {
		return nil, err
	}
	if s.commercial != nil {
		userID := req.OwnerID
		if userID == 0 {
			var err error
			userID, err = parseOwnerUserID(ctx, s.users, owner)
			if err != nil {
				return nil, err
			}
		}
		req.OwnerID = userID
	}

	s.mu.Lock()
	count := s.sessionCountByUser[owner]
	if count >= s.maxSessionsPerUser {
		s.mu.Unlock()
		return nil, ErrSessionLimitExceeded
	}
	s.sessionCountByUser[owner] = count + 1
	s.mu.Unlock()

	sessionID := uuid.NewString()
	session := newSession(sessionID, owner, req, func() {
		s.releaseSession(owner)
	}, func(hookCtx context.Context, eventTimestampMS int64) error {
		if s.commercial == nil || req.OwnerID == 0 {
			return nil
		}
		_, err := s.commercial.RecordTranslationUsageSlice(hookCtx, req.OwnerID, req.CallID, eventTimestampMS)
		return err
	})

	providerSession, err := s.provider.Start(ctx, sessionID, req, session.emit)
	if err != nil {
		_ = session.Stop(context.Background())
		return nil, fmt.Errorf("start provider session: %w", err)
	}
	session.setProvider(providerSession)

	s.logger.Info().
		Str("session_id", sessionID).
		Str("owner", owner).
		Str("to", req.To).
		Str("source_lang", req.SourceLang).
		Str("target_lang", req.TargetLang).
		Int("chunk_ms", req.ChunkMS).
		Msg("translation session started")

	return session, nil
}

func (s *Service) RecordTranscriptSegment(
	ctx context.Context,
	userID uint64,
	callID string,
	fromEmail string,
	toEmail string,
	sourceLang string,
	targetLang string,
	result Result,
) error {
	if s.commercial == nil || userID == 0 || strings.TrimSpace(callID) == "" || !result.IsFinal {
		return nil
	}
	peerID, err := parseOwnerUserID(ctx, s.users, toEmail)
	if err != nil {
		return err
	}
	return s.commercial.RecordTranscriptSegment(ctx, models.CallTranscriptSegment{
		CallID:         strings.TrimSpace(callID),
		UserID:         userID,
		PeerUserID:     peerID,
		FromEmail:      fromEmail,
		ToEmail:        toEmail,
		OriginalText:   strings.TrimSpace(result.OriginalText),
		TranslatedText: strings.TrimSpace(result.TranslatedText),
		SourceLang:     normalizeLang(sourceLang),
		TargetLang:     normalizeLang(targetLang),
		TimestampMS:    result.TimestampMS,
	})
}

func parseOwnerUserID(ctx context.Context, users *user.Service, owner string) (uint64, error) {
	if users == nil {
		return 0, errors.New("user service not configured")
	}
	userModel, err := users.GetByEmail(ctx, owner)
	if err != nil {
		return 0, err
	}
	return userModel.ID, nil
}

func (s *Service) releaseSession(owner string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := s.sessionCountByUser[owner]
	if count <= 1 {
		delete(s.sessionCountByUser, owner)
		return
	}
	s.sessionCountByUser[owner] = count - 1
}

func validateStartRequest(req StartRequest) error {
	if strings.TrimSpace(req.CallID) == "" {
		return fmt.Errorf("%w: call_id is required", ErrBadStartRequest)
	}
	if strings.TrimSpace(req.To) == "" {
		return fmt.Errorf("%w: to is required", ErrBadStartRequest)
	}
	if req.ChunkMS <= 0 {
		return fmt.Errorf("%w: chunk_ms must be positive", ErrBadStartRequest)
	}

	source := normalizeLang(req.SourceLang)
	target := normalizeLang(req.TargetLang)
	if !isSupportedLanguage(source) || !isSupportedLanguage(target) {
		return fmt.Errorf("%w: only zh/en are supported", ErrBadStartRequest)
	}
	if source == target {
		return fmt.Errorf("%w: source_lang and target_lang must differ", ErrBadStartRequest)
	}
	return nil
}

func normalizeLang(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(lang))
	switch lang {
	case "zh-cn", "zh_hans", "cn":
		return "zh"
	case "en-us", "en-gb":
		return "en"
	default:
		return lang
	}
}

func isSupportedLanguage(lang string) bool {
	return lang == "zh" || lang == "en"
}
