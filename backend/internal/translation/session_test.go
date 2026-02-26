package translation

import (
	"context"
	"testing"
)

func TestSessionSendAudioAndStop(t *testing.T) {
	provider := &mockProviderSession{}
	session := newSession("s-1", "owner@test.com", StartRequest{
		CallID:     "call-1",
		To:         "peer@test.com",
		SourceLang: "zh",
		TargetLang: "en",
		ChunkMS:    400,
	}, nil)
	session.setProvider(provider)

	if err := session.SendAudio(context.Background(), AudioChunk{Seq: 1, PCM16Base64: "AA==", SampleRate: 16000, Channels: 1, TimestampMS: 1}); err != nil {
		t.Fatalf("send audio failed: %v", err)
	}
	if provider.sendCalls != 1 {
		t.Fatalf("expected sendCalls=1, got %d", provider.sendCalls)
	}

	if err := session.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if provider.stopCalls != 1 {
		t.Fatalf("expected stopCalls=1, got %d", provider.stopCalls)
	}

	if err := session.Stop(context.Background()); err != nil {
		t.Fatalf("second stop should be no-op, got error: %v", err)
	}
	if provider.stopCalls != 1 {
		t.Fatalf("expected stopCalls still 1, got %d", provider.stopCalls)
	}
}
