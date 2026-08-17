package media

import (
	"fmt"
	"github.com/allcallall/backend/internal/media/sfu"
	"github.com/pion/webrtc/v4"
	"strings"
)

func (r *RoomEngine) HandleOffer(roomID, participantID, sdp string) (string, error) {
	if strings.TrimSpace(roomID) == "" || strings.TrimSpace(participantID) == "" {
		return "", fmt.Errorf("room_id and participant_id are required")
	}

	r.mu.Lock()
	room := r.ensureRoomLocked(roomID)
	participant, err := r.ensureParticipantLocked(room, participantID)
	if err != nil {
		r.mu.Unlock()
		return "", err
	}
	candidateBuffer, err := r.ensureCandidateBufferLocked(room, participantID)
	if err != nil {
		r.mu.Unlock()
		return "", err
	}
	for _, track := range room.tracks {
		if track.participantID == participantID {
			continue
		}
		r.attachTrackLocked(room, participant, track)
	}
	r.mu.Unlock()

	if err := participant.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		return "", err
	}

	// Candidates trickled in before the offer reached us could not be applied
	// yet; replay them now that a remote description exists.
	for _, pending := range candidateBuffer.MarkRemoteReady() {
		if addErr := participant.pc.AddICECandidate(pending); addErr != nil {
			r.logger.Warn().Err(addErr).
				Str("room_id", roomID).
				Str("participant_id", participantID).
				Msg("failed to apply buffered ice candidate")
		}
	}

	answer, err := participant.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	gatherComplete := webrtc.GatheringCompletePromise(participant.pc)
	if err := participant.pc.SetLocalDescription(answer); err != nil {
		return "", err
	}

	// Waiting for full gathering costs the slowest STUN/TURN probe in the ICE
	// server list. When a trickle transport is wired we only wait long enough
	// to pick up host candidates and deliver the rest asynchronously.
	trickle := r.trickleEnabled()
	if !sfu.WaitGather(gatherComplete, r.gatherTimeout(trickle)) {
		r.logger.Debug().
			Str("room_id", roomID).
			Str("participant_id", participantID).
			Bool("trickle", trickle).
			Msg("returning sdp answer before ice gathering completed")
	}

	// Inject Opus DTX (Discontinuous Transmission) to save bandwidth during silence
	// This is a commercial-grade optimization (Pillar A)
	finalSDP := participant.pc.LocalDescription().SDP
	finalSDP = strings.ReplaceAll(finalSDP, "useinbandfec=1", "useinbandfec=1;usedtx=1")

	return finalSDP, nil
}
