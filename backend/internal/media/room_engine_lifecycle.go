package media

import (
	"fmt"
	"github.com/allcallall/backend/internal/media/sfu"
	"github.com/pion/webrtc/v4"
	"os"
	"strings"
	"time"
)

func (r *RoomEngine) LeaveParticipant(roomID, participantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	room, ok := r.rooms[roomID]
	if !ok {
		return nil
	}
	r.removeParticipantLocked(room, participantID)
	if len(room.participants) == 0 {
		delete(r.rooms, roomID)
	}
	return nil
}

func (r *RoomEngine) StartRecording(roomID, baseDir string) error {
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("room_id is required")
	}
	if strings.TrimSpace(baseDir) == "" {
		return fmt.Errorf("recording base dir is required")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	room := r.ensureRoomLocked(roomID)
	room.recording = &roomRecording{
		baseDir:   baseDir,
		startedAt: time.Now(),
		artifacts: make(map[string]*trackRecordingArtifact),
	}
	return nil
}

func (r *RoomEngine) StopRecording(roomID string) ([]RecordingArtifact, error) {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok || room.recording == nil {
		r.mu.Unlock()
		return nil, nil
	}
	recording := room.recording
	room.recording = nil
	r.mu.Unlock()

	recording.mu.Lock()
	defer recording.mu.Unlock()

	artifacts := make([]RecordingArtifact, 0, len(recording.artifacts))
	for key, artifact := range recording.artifacts {
		if artifact.writer != nil {
			if err := artifact.writer.Close(); err != nil {
				r.logger.Warn().Err(err).Str("path", artifact.path).Msg("failed to close recording writer")
			}
		}
		artifacts = append(artifacts, RecordingArtifact{
			ObjectKey:       artifact.path,
			ContentType:     artifact.contentType,
			DurationSeconds: int64(time.Since(artifact.startedAt).Seconds()),
			MetadataJSON:    fmt.Sprintf(`{"track_key":"%s"}`, key),
		})
	}

	return artifacts, nil
}

func (r *RoomEngine) ensureRoomLocked(roomID string) *mediaRoom {
	room, ok := r.rooms[roomID]
	if ok {
		return room
	}
	room = &mediaRoom{
		id:           roomID,
		participants: make(map[string]*roomParticipant),
		tracks:       make(map[string]*publishedTrack),
		candidates:   make(map[string]*sfu.CandidateBuffer),
	}
	r.rooms[roomID] = room
	return room
}

func (r *RoomEngine) ensureParticipantLocked(room *mediaRoom, participantID string) (*roomParticipant, error) {
	if participant, ok := room.participants[participantID]; ok {
		return participant, nil
	}

	// Stash the participant id so the GCC estimator produced synchronously
	// inside NewPeerConnection can be bound to it (see BandwidthController).
	if r.bw != nil {
		r.bw.SetPending(participantID)
		defer r.bw.ClearPending()
	}

	var pc *webrtc.PeerConnection
	var err error
	if r.api != nil {
		pc, err = r.api.NewPeerConnection(r.defaultConfig)
	} else {
		pc, err = webrtc.NewPeerConnection(r.defaultConfig)
	}
	if err != nil {
		return nil, err
	}

	participant := &roomParticipant{
		id:      participantID,
		pc:      pc,
		senders: make(map[string]*webrtc.RTPSender),
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			r.emitCandidate(sfu.LocalCandidate{
				RoomID:          room.id,
				ParticipantID:   participantID,
				EndOfCandidates: true,
			})
			return
		}
		init := candidate.ToJSON()
		r.emitCandidate(sfu.LocalCandidate{
			RoomID:           room.id,
			ParticipantID:    participantID,
			Candidate:        init.Candidate,
			SDPMid:           init.SDPMid,
			SDPMLineIndex:    init.SDPMLineIndex,
			UsernameFragment: init.UsernameFragment,
		})
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			// A freshly connected subscriber cannot decode anything until the
			// publishers emit an IDR frame, so ask for one right away instead
			// of showing a black tile until the next natural keyframe.
			go r.requestKeyframesForSubscriber(room.id, participantID)
			return
		}
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			r.mu.Lock()
			defer r.mu.Unlock()
			if roomRef, ok := r.rooms[room.id]; ok {
				r.removeParticipantLocked(roomRef, participantID)
				if len(roomRef.participants) == 0 {
					delete(r.rooms, room.id)
				}
			}
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		r.handleRemoteTrack(room.id, participantID, track)
	})

	room.participants[participantID] = participant
	return participant, nil
}

func (r *RoomEngine) removeParticipantLocked(room *mediaRoom, participantID string) {
	participant, ok := room.participants[participantID]
	if !ok {
		return
	}
	if err := participant.pc.Close(); err != nil {
		r.logger.Warn().Err(err).Str("participant_id", participantID).Msg("failed to close peer connection on participant removal")
	}
	delete(room.participants, participantID)
	delete(room.candidates, participantID)
	r.bwForgetParticipant(participantID)

	for key, track := range room.tracks {
		if track.participantID != participantID {
			continue
		}
		for _, other := range room.participants {
			if sender, ok := other.senders[key]; ok {
				if err := other.pc.RemoveTrack(sender); err != nil {
					r.logger.Warn().Err(err).Str("room_id", room.id).Str("participant_id", other.id).Msg("failed to remove track from participant")
				}
				delete(other.senders, key)
				r.bwUnmarkForwarded(other.id, key)
				// The subscriber lost a sender; renegotiate so its peer
				// connection drops the now-removed m-line.
				r.requestRenegotiation(room.id, other.id, other.pc)
			}
		}
		delete(room.tracks, key)
	}
}
