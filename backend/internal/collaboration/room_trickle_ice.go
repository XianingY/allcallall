package collaboration

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/allcallall/backend/internal/media/sfu"
)

// RoomICECandidateEvent is the realtime event name carrying a server side ICE
// candidate towards a single meeting participant.
const RoomICECandidateEvent = "room.ice.candidate"

// trickleDispatchTimeout bounds the realtime publish of one candidate. The
// dispatcher is a single background goroutine, so a hung publish would stall
// every subsequent candidate.
const trickleDispatchTimeout = 5 * time.Second

// roomOrgRegistry remembers which organization a media room belongs to.
//
// ICE candidates are produced asynchronously by the media engine, long after
// the HTTP request that carried the offer is gone. Resolving the organization
// from the database on that path would add a query per candidate, so the
// mapping is captured once when the offer is handled.
type roomOrgRegistry struct {
	mu        sync.RWMutex
	orgByRoom map[uint64]uint64
}

func newRoomOrgRegistry() *roomOrgRegistry {
	return &roomOrgRegistry{orgByRoom: make(map[uint64]uint64)}
}

func (r *roomOrgRegistry) remember(roomID, organizationID uint64) {
	if r == nil || roomID == 0 || organizationID == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orgByRoom[roomID] = organizationID
}

func (r *roomOrgRegistry) lookup(roomID uint64) (uint64, bool) {
	if r == nil {
		return 0, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	organizationID, ok := r.orgByRoom[roomID]
	return organizationID, ok
}

func (r *roomOrgRegistry) forget(roomID uint64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.orgByRoom, roomID)
}

func (r *roomOrgRegistry) size() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.orgByRoom)
}

// TrickleICEEnabled reports whether server side candidates are delivered out
// of band instead of being embedded in the SDP answer.
func (s *Service) TrickleICEEnabled() bool {
	return s.trickleICE
}

// wireTrickleICE connects the media engine's candidate sink to the realtime
// delivery pipeline. It is a no-op unless trickle ICE was enabled, so a
// deployment whose clients only understand vanilla ICE keeps the previous
// behaviour of answering with a fully gathered SDP.
func (s *Service) wireTrickleICE() {
	if !s.trickleICE || s.media == nil {
		return
	}
	s.media.SetRoomICECandidateSink(s.publishRoomICECandidate)
}

func (s *Service) publishRoomICECandidate(candidate sfu.LocalCandidate) {
	roomID, err := strconv.ParseUint(candidate.RoomID, 10, 64)
	if err != nil {
		return
	}
	participantID, err := strconv.ParseUint(candidate.ParticipantID, 10, 64)
	if err != nil {
		return
	}
	organizationID, ok := s.roomOrgs.lookup(roomID)
	if !ok {
		// The room was torn down (or never offered through this instance);
		// dropping the candidate is correct, the peer connection is gone too.
		return
	}

	payload := map[string]any{
		"room_id":           roomID,
		"candidate":         candidate.Candidate,
		"end_of_candidates": candidate.EndOfCandidates,
	}
	if candidate.SDPMid != nil {
		payload["sdpMid"] = *candidate.SDPMid
	}
	if candidate.SDPMLineIndex != nil {
		payload["sdpMLineIndex"] = *candidate.SDPMLineIndex
	}
	if candidate.UsernameFragment != nil {
		payload["usernameFragment"] = *candidate.UsernameFragment
	}

	ctx, cancel := context.WithTimeout(context.Background(), trickleDispatchTimeout)
	defer cancel()
	s.publishRealtimeEvent(ctx, organizationID, []uint64{participantID}, RoomICECandidateEvent, payload)
}
