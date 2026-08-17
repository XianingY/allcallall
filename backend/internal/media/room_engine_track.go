package media

import (
	"fmt"
	"github.com/allcallall/backend/internal/media/sfu"
	"github.com/pion/webrtc/v4"
	"time"
)

// attachTrackLocked subscribes a participant to a published track. The caller
// must hold r.mu.
func (r *RoomEngine) attachTrackLocked(room *mediaRoom, subscriber *roomParticipant, published *publishedTrack) bool {
	if subscriber == nil || published == nil || subscriber.id == published.participantID {
		return false
	}
	if _, ok := subscriber.senders[published.key]; ok {
		return false
	}

	// Bandwidth-aware forwarding: when GCC estimation is active and this is a
	// video track, only attach it if the subscriber's downlink budget can
	// absorb it. Audio is never gated so voice is preserved on weak links. Both
	// the decision and the bookkeeping are no-ops when estimation is disabled.
	if r.bw != nil && published.kind == webrtc.RTPCodecTypeVideo {
		if !r.bw.Manager().ShouldForward(subscriber.id, published.key) {
			r.bw.Manager().RecordThrottled()
			r.logger.Debug().
				Str("room_id", room.id).
				Str("participant_id", subscriber.id).
				Str("track_key", published.key).
				Msg("skipping video track attach: subscriber downlink budget exceeded")
			return false
		}
		r.bw.Manager().MarkForwarded(subscriber.id, published.key)
	}

	sender, err := subscriber.pc.AddTrack(published.local)
	if err != nil {
		r.logger.Warn().Err(err).
			Str("room_id", room.id).
			Str("participant_id", subscriber.id).
			Str("track_key", published.key).
			Msg("failed to attach relay track to participant")
		return false
	}
	subscriber.senders[published.key] = sender
	go r.forwardSubscriberFeedback(room.id, published.key, sender)
	return true
}

// forwardSubscriberFeedback drains RTCP coming back from a subscriber and
// relays keyframe requests to the publisher. Draining is mandatory even when
// nothing is forwarded: an unread sender stalls the interceptor chain.
func (r *RoomEngine) forwardSubscriberFeedback(roomID, trackKey string, sender *webrtc.RTPSender) {
	for {
		packets, _, err := sender.ReadRTCP()
		if err != nil {
			return
		}
		if !sfu.ContainsKeyframeRequest(packets) {
			continue
		}
		r.requestKeyframeForTrack(roomID, trackKey)
	}
}

type keyframeTarget struct {
	pc   *webrtc.PeerConnection
	ssrc uint32
}

func (r *RoomEngine) requestKeyframesForSubscriber(roomID, subscriberID string) {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok {
		r.mu.Unlock()
		return
	}
	targets := make([]keyframeTarget, 0, len(room.tracks))
	for _, track := range room.tracks {
		if track.participantID == subscriberID || track.kind != webrtc.RTPCodecTypeVideo {
			continue
		}
		publisher, ok := room.participants[track.participantID]
		if !ok {
			continue
		}
		targets = append(targets, keyframeTarget{pc: publisher.pc, ssrc: track.ssrc})
	}
	r.mu.Unlock()

	r.sendKeyframeRequests(roomID, targets)
}

func (r *RoomEngine) requestKeyframeForTrack(roomID, trackKey string) {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok {
		r.mu.Unlock()
		return
	}
	track, ok := room.tracks[trackKey]
	if !ok || track.kind != webrtc.RTPCodecTypeVideo {
		r.mu.Unlock()
		return
	}
	publisher, ok := room.participants[track.participantID]
	if !ok {
		r.mu.Unlock()
		return
	}
	target := keyframeTarget{pc: publisher.pc, ssrc: track.ssrc}
	r.mu.Unlock()

	r.sendKeyframeRequests(roomID, []keyframeTarget{target})
}

func (r *RoomEngine) sendKeyframeRequests(roomID string, targets []keyframeTarget) {
	for _, target := range targets {
		if _, err := r.keyframes.Request(target.pc, target.ssrc); err != nil {
			r.logger.Debug().Err(err).
				Str("room_id", roomID).
				Uint32("ssrc", target.ssrc).
				Msg("failed to send keyframe request")
		}
	}
}

func (r *RoomEngine) handleRemoteTrack(roomID, participantID string, track *webrtc.TrackRemote) {
	relayTrackID := fmt.Sprintf("participant-%s-%s-%s", participantID, track.Kind().String(), sanitizeFilePart(track.ID()))
	relayStreamID := fmt.Sprintf("participant-%s", participantID)
	localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, relayTrackID, relayStreamID)
	if err != nil {
		r.logger.Warn().Err(err).Str("room_id", roomID).Msg("failed to create local relay track")
		return
	}

	rid := track.RID()
	key := fmt.Sprintf("%s:%s:%s", participantID, track.Kind().String(), track.ID())
	if rid != "" {
		key = fmt.Sprintf("%s:%s:%s:%s", participantID, track.Kind().String(), track.ID(), rid)
	}

	ssrc := uint32(track.SSRC())

	r.mu.Lock()
	room := r.ensureRoomLocked(roomID)
	published := &publishedTrack{
		key:           key,
		participantID: participantID,
		track:         track,
		local:         localTrack,
		kind:          track.Kind(),
		ssrc:          ssrc,
	}
	room.tracks[key] = published
	subscribers := 0
	for otherID, other := range room.participants {
		if otherID == participantID {
			continue
		}
		if r.attachTrackLocked(room, other, published) {
			subscribers++
			// A new sender was attached to this subscriber's peer connection;
			// renegotiate so the client can receive the new track.
			r.requestRenegotiation(room.id, otherID, other.pc)
		}
	}
	r.mu.Unlock()

	// Subscribers that were already in the room joined mid-stream and need an
	// IDR frame before they can render this track.
	if subscribers > 0 && track.Kind() == webrtc.RTPCodecTypeVideo {
		r.requestKeyframeForTrack(roomID, key)
	}

	var sampledBytes int64
	sampledAt := time.Now()
	for {
		packet, _, readErr := track.ReadRTP()
		if readErr != nil {
			break
		}
		if writeErr := localTrack.WriteRTP(packet); writeErr != nil {
			r.logger.Debug().Err(writeErr).Str("room_id", roomID).Msg("failed to forward room rtp packet")
			continue
		}
		r.writeRecordingPacket(roomID, participantID, track, packet)

		// Periodically re-measure this published track's send bitrate so the
		// bandwidth manager holds a fresh value for forwarding decisions.
		sampledBytes += int64(len(packet.Payload)) + rtpHeaderSize
		if elapsed := time.Since(sampledAt); elapsed >= bandwidthSampleWindow {
			bps := int(float64(sampledBytes*8) / elapsed.Seconds())
			r.bwRegisterTrack(key, bps)
			sampledBytes = 0
			sampledAt = time.Now()
		}
	}

	r.mu.Lock()
	if roomRef, ok := r.rooms[roomID]; ok {
		delete(roomRef.tracks, key)
		r.bwForgetTrack(key)
		for _, other := range roomRef.participants {
			if sender, ok := other.senders[key]; ok {
				if err := other.pc.RemoveTrack(sender); err != nil {
					r.logger.Warn().Err(err).Str("room_id", roomID).Str("participant_id", other.id).Msg("failed to remove track from participant")
				}
				delete(other.senders, key)
				r.bwUnmarkForwarded(other.id, key)
				r.requestRenegotiation(roomID, other.id, other.pc)
			}
		}
	}
	r.mu.Unlock()
	r.keyframes.Forget(ssrc)
}
