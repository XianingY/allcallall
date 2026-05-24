package handlers

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/translation"
)

type testProvider struct{}

func (p *testProvider) Name() string { return "test" }

func (p *testProvider) Start(ctx context.Context, sessionID string, req translation.StartRequest, onEvent func(translation.Event)) (translation.ProviderSession, error) {
	return &testProviderSession{onEvent: onEvent}, nil
}

type testProviderSession struct {
	onEvent func(translation.Event)
}

func (s *testProviderSession) SendAudio(ctx context.Context, chunk translation.AudioChunk) error {
	s.onEvent(translation.Event{Result: &translation.Result{
		SegmentID:      "seg-1",
		Revision:       1,
		IsFinal:        true,
		OriginalText:   "你好",
		TranslatedText: "hello",
		TimestampMS:    time.Now().UnixMilli(),
		LatencyMS:      300,
		Source:         "online",
	}})
	return nil
}

func (s *testProviderSession) Stop(ctx context.Context) error {
	return nil
}

type testSubtitleDispatcher struct {
	calls []translation.Result
}

func (d *testSubtitleDispatcher) DispatchSubtitle(ctx context.Context, fromEmail, toEmail, callID string, result translation.Result) error {
	d.calls = append(d.calls, result)
	return nil
}

func TestTranslationWSHandlerLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &testProvider{}
	svc := translation.NewService(zerolog.Nop(), provider, 1)
	dispatcher := &testSubtitleDispatcher{}
	handler := NewTranslationWSHandlerWithDispatcher(zerolog.Nop(), svc, dispatcher)

	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{Email: "alice@example.com"})
		handler.Handle(c)
	})

	ts := httptest.NewServer(router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	start := map[string]any{
		"type":        "translation.start",
		"call_id":     "call-1",
		"to":          "bob@example.com",
		"source_lang": "zh",
		"target_lang": "en",
		"chunk_ms":    400,
	}
	if err := conn.WriteJSON(start); err != nil {
		t.Fatalf("send start failed: %v", err)
	}

	audio := map[string]any{
		"type":         "translation.audio",
		"seq":          1,
		"pcm16_base64": "AA==",
		"sample_rate":  16000,
		"channels":     1,
		"timestamp_ms": time.Now().UnixMilli(),
	}
	if err := conn.WriteJSON(audio); err != nil {
		t.Fatalf("send audio failed: %v", err)
	}

	gotAck := false
	gotFinal := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (!gotAck || !gotFinal) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				continue
			}
			t.Fatalf("read message failed: %v", err)
		}

		var msg map[string]any
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatalf("unmarshal message failed: %v", err)
		}

		typ, _ := msg["type"].(string)
		if typ == "translation.ack" {
			gotAck = true
		}
		if typ == "translation.final" {
			gotFinal = true
		}
	}

	if !gotAck {
		t.Fatal("expected translation.ack")
	}
	if !gotFinal {
		t.Fatal("expected translation.final")
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected 1 dispatched subtitle, got %d", len(dispatcher.calls))
	}

	if err := conn.WriteJSON(map[string]any{"type": "translation.stop"}); err != nil {
		t.Fatalf("send stop failed: %v", err)
	}
}
