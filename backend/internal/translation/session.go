package translation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/allcallall/backend/internal/commerce"
)

// Session 翻译会话
// Session wraps provider stream state and event delivery.
type Session struct {
	ID         string
	Owner      string
	OwnerID    uint64
	CallID     string
	To         string
	SourceLang string
	TargetLang string
	CreatedAt  time.Time

	providerMu sync.RWMutex
	provider   ProviderSession

	events   chan Event
	eventsMu sync.RWMutex
	closed   atomic.Bool
	onClose  func()

	usageMu        sync.Mutex
	usageSliceHook func(ctx context.Context, eventTimestampMS int64) error
}

func newSession(
	sessionID string,
	owner string,
	req StartRequest,
	onClose func(),
	usageSliceHook func(ctx context.Context, eventTimestampMS int64) error,
) *Session {
	return &Session{
		ID:             sessionID,
		Owner:          owner,
		OwnerID:        req.OwnerID,
		CallID:         req.CallID,
		To:             req.To,
		SourceLang:     req.SourceLang,
		TargetLang:     req.TargetLang,
		CreatedAt:      time.Now().UTC(),
		events:         make(chan Event, 32),
		onClose:        onClose,
		usageSliceHook: usageSliceHook,
	}
}

func (s *Session) setProvider(provider ProviderSession) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	s.provider = provider
}

// Events 返回会话事件流
// Events returns session event stream.
func (s *Session) Events() <-chan Event {
	return s.events
}

func (s *Session) emit(evt Event) {
	if s.closed.Load() {
		return
	}
	if err := s.recordUsageFromEvent(evt); err != nil {
		if errors.Is(err, commerce.ErrTranslationQuotaExhausted) {
			evt = Event{
				Error: &ProviderError{
					Code:        "TRANSLATION_QUOTA_EXHAUSTED",
					Message:     err.Error(),
					Recoverable: false,
				},
			}
		} else {
			evt = Event{
				Error: &ProviderError{
					Code:        "USAGE_METERING_FAILED",
					Message:     err.Error(),
					Recoverable: true,
				},
			}
		}
	}

	s.eventsMu.RLock()
	ch := s.events
	s.eventsMu.RUnlock()

	select {
	case ch <- evt:
	default:
		// 保证 final/error 事件尽可能投递，partial 可丢弃。
		// Ensure final/error are prioritized when buffer is full.
		if evt.Error != nil || (evt.Result != nil && evt.Result.IsFinal) {
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- evt:
			default:
			}
		}
	}
}

// SendAudio 发送音频分片到供应商
// SendAudio forwards one audio chunk to provider.
func (s *Session) SendAudio(ctx context.Context, chunk AudioChunk) error {
	if s.closed.Load() {
		return errors.New("translation session already closed")
	}

	s.providerMu.RLock()
	provider := s.provider
	s.providerMu.RUnlock()
	if provider == nil {
		return errors.New("translation provider session not initialized")
	}
	return provider.SendAudio(ctx, chunk)
}

func (s *Session) recordUsageFromEvent(evt Event) error {
	if s.usageSliceHook == nil || evt.Result == nil {
		return nil
	}
	eventTimestampMS := evt.Result.TimestampMS
	if eventTimestampMS <= 0 {
		eventTimestampMS = time.Now().UnixMilli()
	}

	s.usageMu.Lock()
	defer s.usageMu.Unlock()

	if s.closed.Load() {
		return nil
	}
	return s.usageSliceHook(context.Background(), eventTimestampMS)
}

// Stop 停止会话
// Stop closes provider session and event stream.
func (s *Session) Stop(ctx context.Context) error {
	if s.closed.Swap(true) {
		return nil
	}

	var stopErr error
	s.providerMu.RLock()
	provider := s.provider
	s.providerMu.RUnlock()
	if provider != nil {
		stopErr = provider.Stop(ctx)
	}

	s.eventsMu.Lock()
	close(s.events)
	s.eventsMu.Unlock()

	if s.onClose != nil {
		s.onClose()
	}
	return stopErr
}
