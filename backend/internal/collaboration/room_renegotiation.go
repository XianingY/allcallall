package collaboration

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/media"
)

// RoomRenegotiationEvent is the realtime event carrying a server-initiated
// renegotiation offer towards a single meeting participant.
const RoomRenegotiationEvent = "room.renegotiate"

// renegotiationDispatchTimeout bounds the realtime publish of one offer. The
// dispatcher is a single background goroutine, so a hung publish would stall
// every subsequent offer.
const renegotiationDispatchTimeout = 5 * time.Second

// publishRoomRenegotiation forwards a server-generated renegotiation offer to
// the affected participant over the realtime channel. It mirrors the trickle
// ICE delivery path: the room -> organization mapping was captured when the
// offer was first handled, so no database lookup is needed here.
func (s *Service) publishRoomRenegotiation(offer media.RenegotiationOffer) {
	roomID, err := strconv.ParseUint(offer.RoomID, 10, 64)
	if err != nil {
		return
	}
	participantID, err := strconv.ParseUint(offer.ParticipantID, 10, 64)
	if err != nil {
		return
	}
	organizationID, ok := s.roomOrgs.lookup(roomID)
	if !ok {
		// The room was torn down (or never offered through this instance);
		// dropping the offer is correct, the peer connection is gone too.
		return
	}

	payload := map[string]any{
		"room_id": roomID,
		"sdp":     offer.SDP,
		"type":    "offer",
	}

	ctx, cancel := context.WithTimeout(context.Background(), renegotiationDispatchTimeout)
	defer cancel()
	s.publishRealtimeEvent(ctx, organizationID, []uint64{participantID}, RoomRenegotiationEvent, payload)
}

// HandleRoomRenegotiationAnswer applies the answer a client produced in
// response to a server-initiated renegotiation offer.
func (s *Service) HandleRoomRenegotiationAnswer(ctx context.Context, organizationID, userID, roomID uint64, sdp string) error {
	if strings.TrimSpace(sdp) == "" {
		return errors.New("sdp is required")
	}
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return err
	}
	if err := s.ensureRoomParticipantJoined(ctx, organizationID, userID, roomID); err != nil {
		return err
	}
	if s.media == nil {
		return errors.New("media engine not attached")
	}
	if err := s.media.HandleRenegotiationAnswer(strconv.FormatUint(roomID, 10), strconv.FormatUint(userID, 10), sdp); err != nil {
		return err
	}
	return nil
}
