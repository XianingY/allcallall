package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type FileInput struct {
	OrganizationID     uint64
	ConversationID     uint64
	RoomID             uint64
	RecordingSessionID uint64
	RecordingFileID    uint64
	LocalPath          string
	ContentType        string
	MetadataJSON       string
	DurationSeconds    int64
}

type Segment struct {
	SpeakerUserID *uint64
	TrackKey      string
	Language      string
	Text          string
	StartMS       int64
	EndMS         int64
	Confidence    float64
}

type Provider interface {
	Name() string
	TranscribeFile(ctx context.Context, input FileInput) ([]Segment, error)
}

type MockProvider struct{}

func NewMockProvider() MockProvider {
	return MockProvider{}
}

func (MockProvider) Name() string {
	return "mock"
}

func (MockProvider) TranscribeFile(_ context.Context, input FileInput) ([]Segment, error) {
	trackKey := trackKeyFromMetadata(input.MetadataJSON)
	if trackKey == "" {
		trackKey = filepath.Base(input.LocalPath)
	}
	participantID := participantIDFromTrackKey(trackKey)
	endMS := input.DurationSeconds * 1000
	if endMS <= 0 {
		endMS = 1000
	}
	text := fmt.Sprintf(
		"Mock meeting transcript for recording %d file %d track %s.",
		input.RecordingSessionID,
		input.RecordingFileID,
		trackKey,
	)
	return []Segment{{
		SpeakerUserID: participantID,
		TrackKey:      trackKey,
		Language:      "und",
		Text:          text,
		StartMS:       0,
		EndMS:         endMS,
		Confidence:    1,
	}}, nil
}

func trackKeyFromMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return ""
	}
	if value, ok := metadata["track_key"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func participantIDFromTrackKey(trackKey string) *uint64 {
	parts := strings.SplitN(trackKey, ":", 2)
	if len(parts) == 0 {
		return nil
	}
	value, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || value == 0 {
		return nil
	}
	return &value
}
