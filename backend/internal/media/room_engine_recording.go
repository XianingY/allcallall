package media

import (
	"fmt"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (r *RoomEngine) writeRecordingPacket(roomID, participantID string, track *webrtc.TrackRemote, packet *rtp.Packet) {
	if track.Kind() != webrtc.RTPCodecTypeAudio {
		return
	}

	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok || room.recording == nil {
		r.mu.Unlock()
		return
	}
	recording := room.recording
	r.mu.Unlock()

	recording.mu.Lock()
	defer recording.mu.Unlock()

	trackKey := fmt.Sprintf("%s:%s", participantID, track.ID())
	artifact, ok := recording.artifacts[trackKey]
	if !ok {
		ext := ".audio"
		contentType := "audio/octet-stream"
		clockRate := track.Codec().ClockRate
		channels := track.Codec().Channels
		if channels == 0 {
			channels = 2
		}
		if strings.Contains(strings.ToLower(track.Codec().MimeType), "opus") {
			ext = ".ogg"
			contentType = "audio/ogg"
		}
		path := filepath.Join(recording.baseDir, fmt.Sprintf("participant-%s-track-%s%s", participantID, sanitizeFilePart(track.ID()), ext))
		artifact = &trackRecordingArtifact{
			path:        path,
			contentType: contentType,
			startedAt:   time.Now(),
		}
		if ext == ".ogg" {
			writer, err := oggwriter.New(path, track.Codec().ClockRate, channels)
			if err != nil {
				r.logger.Warn().Err(err).Str("room_id", roomID).Msg("failed to create ogg writer")
				return
			}
			artifact.writer = writer
		} else {
			clockRate = 0
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				r.logger.Warn().Err(err).Str("room_id", roomID).Msg("failed to create raw recording file")
				return
			}
		}
		recording.artifacts[trackKey] = artifact
		_ = clockRate
	}

	if artifact.writer != nil {
		if err := artifact.writer.WriteRTP(packet); err != nil {
			r.logger.Debug().Err(err).Str("room_id", roomID).Msg("failed to write recording packet")
		}
	}
}
