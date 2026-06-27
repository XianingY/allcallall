package transcription

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAICompatibleProviderTranscribesVerboseSegments(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization %q", got)
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("model"); got != "whisper-test" {
			t.Fatalf("unexpected model %q", got)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("missing audio file: %v", err)
		}
		defer file.Close()
		if raw, _ := io.ReadAll(file); string(raw) != "audio" {
			t.Fatalf("unexpected file body %q", string(raw))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":     "hello world",
			"language": "en",
			"duration": 2.5,
			"segments": []map[string]any{
				{"start": 0.25, "end": 1.5, "text": "hello", "avg_logprob": -0.1},
				{"start": 1.5, "end": 2.5, "text": "world", "confidence": 0.8},
			},
		})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		BaseURL: server.URL + "/v1",
		APIKey:  "secret",
		Model:   "whisper-test",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	path := filepath.Join(t.TempDir(), "track.ogg")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	segments, err := provider.TranscribeFile(context.Background(), FileInput{
		LocalPath:       path,
		MetadataJSON:    `{"track_key":"7:microphone"}`,
		DurationSeconds: 2,
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if requests.Load() != 1 || len(segments) != 2 {
		t.Fatalf("unexpected requests=%d segments=%+v", requests.Load(), segments)
	}
	if segments[0].StartMS != 250 || segments[1].EndMS != 2500 {
		t.Fatalf("unexpected timestamps %+v", segments)
	}
	if segments[0].SpeakerUserID == nil || *segments[0].SpeakerUserID != 7 || segments[0].TrackKey != "7:microphone" {
		t.Fatalf("unexpected track metadata %+v", segments[0])
	}
}

func TestOpenAICompatibleProviderClassifiesHTTPFailures(t *testing.T) {
	for _, item := range []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "rate limit", status: http.StatusTooManyRequests, retryable: true},
		{name: "server", status: http.StatusBadGateway, retryable: true},
		{name: "auth", status: http.StatusUnauthorized, retryable: false},
	} {
		t.Run(item.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(item.status)
				_, _ = w.Write([]byte("provider failure"))
			}))
			defer server.Close()
			provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{BaseURL: server.URL, Model: "test"})
			if err != nil {
				t.Fatalf("new provider: %v", err)
			}
			path := filepath.Join(t.TempDir(), "track.ogg")
			_ = os.WriteFile(path, []byte("audio"), 0o644)
			_, err = provider.TranscribeFile(context.Background(), FileInput{LocalPath: path, DurationSeconds: 1})
			if err == nil || IsRetryable(err) != item.retryable {
				t.Fatalf("unexpected error retryable=%v err=%v", IsRetryable(err), err)
			}
		})
	}
}

func TestOpenAICompatibleProviderChunksAndOffsetsTimeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":     "chunk",
			"segments": []map[string]any{{"start": 1.0, "end": 2.0, "text": "chunk"}},
		})
	}))
	defer server.Close()

	tempDir := t.TempDir()
	ffmpeg := filepath.Join(tempDir, "fake-ffmpeg")
	script := `#!/bin/sh
for arg in "$@"; do output="$arg"; done
first=$(printf '%s' "$output" | sed 's/%06d/000000/')
second=$(printf '%s' "$output" | sed 's/%06d/000001/')
printf 'a' > "$first"
printf 'b' > "$second"
`
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		BaseURL:       server.URL,
		Model:         "test",
		ChunkDuration: 10 * time.Second,
		FFmpegPath:    ffmpeg,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	path := filepath.Join(tempDir, "long.ogg")
	_ = os.WriteFile(path, []byte("audio"), 0o644)
	segments, err := provider.TranscribeFile(context.Background(), FileInput{LocalPath: path, DurationSeconds: 20})
	if err != nil {
		t.Fatalf("transcribe chunks: %v", err)
	}
	if len(segments) != 2 || segments[0].StartMS != 1000 || segments[1].StartMS != 11000 {
		t.Fatalf("unexpected chunk offsets %+v", segments)
	}
}

func TestNewOpenAICompatibleProviderRequiresConfiguration(t *testing.T) {
	if _, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{Model: "test"}); err == nil {
		t.Fatal("expected missing base url to fail")
	}
	if _, err := NewOpenAICompatibleProvider(OpenAICompatibleConfig{BaseURL: "https://example.test"}); err == nil {
		t.Fatal("expected missing model to fail")
	}
}

func TestProviderErrorWrapsCause(t *testing.T) {
	cause := errors.New("cause")
	err := &ProviderError{Operation: "request", Retryable: true, Err: cause}
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "request") {
		t.Fatalf("unexpected provider error %v", err)
	}
}
