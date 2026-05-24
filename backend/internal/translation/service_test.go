package translation

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
)

type mockProvider struct {
	startFn func(ctx context.Context, sessionID string, req StartRequest, onEvent func(Event)) (ProviderSession, error)
}

func (p *mockProvider) Name() string { return "mock" }

func (p *mockProvider) Start(ctx context.Context, sessionID string, req StartRequest, onEvent func(Event)) (ProviderSession, error) {
	if p.startFn != nil {
		return p.startFn(ctx, sessionID, req, onEvent)
	}
	return &mockProviderSession{}, nil
}

type mockProviderSession struct {
	sendCalls int
	stopCalls int
}

func (s *mockProviderSession) SendAudio(ctx context.Context, chunk AudioChunk) error {
	s.sendCalls++
	return nil
}

func (s *mockProviderSession) Stop(ctx context.Context) error {
	s.stopCalls++
	return nil
}

func TestServiceStartSessionValidation(t *testing.T) {
	svc := NewService(zerolog.Nop(), &mockProvider{}, 1)
	_, err := svc.StartSession(context.Background(), "u@test.com", StartRequest{
		CallID:     "call-1",
		To:         "peer@test.com",
		SourceLang: "zh",
		TargetLang: "zh",
		ChunkMS:    400,
	})
	if !errors.Is(err, ErrBadStartRequest) {
		t.Fatalf("expected ErrBadStartRequest, got %v", err)
	}
}

func TestServiceSessionLimit(t *testing.T) {
	provider := &mockProvider{}
	svc := NewService(zerolog.Nop(), provider, 1)

	s1, err := svc.StartSession(context.Background(), "u@test.com", StartRequest{
		CallID:     "call-1",
		To:         "peer@test.com",
		SourceLang: "zh",
		TargetLang: "en",
		ChunkMS:    400,
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	defer s1.Stop(context.Background())

	_, err = svc.StartSession(context.Background(), "u@test.com", StartRequest{
		CallID:     "call-2",
		To:         "peer@test.com",
		SourceLang: "zh",
		TargetLang: "en",
		ChunkMS:    400,
	})
	if !errors.Is(err, ErrSessionLimitExceeded) {
		t.Fatalf("expected ErrSessionLimitExceeded, got %v", err)
	}
}

func TestServiceEmitsProviderEvent(t *testing.T) {
	provider := &mockProvider{}
	provider.startFn = func(ctx context.Context, sessionID string, req StartRequest, onEvent func(Event)) (ProviderSession, error) {
		onEvent(Event{Result: &Result{
			SegmentID:      "seg-1",
			Revision:       1,
			IsFinal:        false,
			OriginalText:   "你好",
			TranslatedText: "hello",
			TimestampMS:    123,
			LatencyMS:      321,
			Source:         "online",
		}})
		return &mockProviderSession{}, nil
	}

	svc := NewService(zerolog.Nop(), provider, 1)
	session, err := svc.StartSession(context.Background(), "u@test.com", StartRequest{
		CallID:     "call-1",
		To:         "peer@test.com",
		SourceLang: "zh",
		TargetLang: "en",
		ChunkMS:    400,
	})
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	defer session.Stop(context.Background())

	select {
	case evt := <-session.Events():
		if evt.Result == nil || evt.Result.TranslatedText != "hello" {
			t.Fatalf("unexpected event payload: %+v", evt)
		}
	default:
		t.Fatal("expected event from provider callback")
	}
}
