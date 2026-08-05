# Backend Configuration

Runtime behaviour is tuned through environment variables. This document covers
the meeting-room / WebRTC related variables. See `internal/config/config.go`
for the full set (database, redis, mail, feature flags, etc.).

## Meeting rooms / WebRTC

### `ROOM_MAX_PARTICIPANTS`
Maximum number of participants allowed in a single meeting room.
- Default: `6`
- Notes: values `<= 0` fall back to the default. The SFU relays each
  published track to every other participant, so capacity is bounded by the
  server's forwarding budget rather than by a hard protocol limit.

### `ROOM_TRICKLE_ICE`
Enables Trickle ICE for meeting-room peer connections.
- Default: `false`
- When `false`, the server waits (bounded by the blocking gather timeout) for
  ICE gathering to complete before returning the SDP answer, which can add
  hundreds of milliseconds on weak networks.
- When `true`, server-side ICE candidates are trickled to clients over the
  realtime event channel (`room.ice.candidate`) and the answer is returned as
  soon as the local description is set. The `trickle_ice` flag in the offer
  response tells the client which mode is active.

### `ROOM_BANDWIDTH_ESTIMATION`
Enables GCC (Google Congestion Control) bandwidth estimation and bandwidth
aware forwarding for meeting rooms.
- Default: `false`
- When `false`, the media engine keeps Pion's auto-registered default
  interceptors (NACK / RTCP reports / TWCC sender); SDP and behaviour are
  unchanged.
- When `true`, a GCC send-side bandwidth estimator is attached to every room
  peer connection. Each participant's estimated downlink bitrate (derived from
  the TWCC feedback the client sends) is fed into the `BandwidthManager`. When a
  new video track would exceed a subscriber's remaining downlink budget (after a
  safety headroom), the SFU skips attaching that track so audio is preserved on
  weak links. Per-participant estimates and forwarding stats are exposed via
  `Engine.RoomBandwidthStats()`.
- Requires the peer connection API to be built with the interceptor registry
  produced by `sfu.BuildInterceptorRegistry` (wired in
  `internal/signaling/pion_init.go`).

### Multi-party renegotiation (automatic)
When a participant joins or leaves an established meeting, the server
re-offers the affected subscribers' peer connections so they pick up the new
(or dropped) tracks. This is what makes multi-party video work: the initial
offer/answer only covers the tracks that existed at join time.

- Renegotiation is **always enabled** once the media engine is attached; there
  is no feature flag. The server acts as the offerer and the client answers
  over `POST /api/v1/rooms/:roomId/renegotiate`.
- Offers are delivered via the **`room` realtime channel** as the
  `room.renegotiate` event. Clients subscribe by requesting a `room` channel
  ticket (`POST /api/v1/realtime/tickets`, `channel: "room"`) and opening the
  returned `/rooms/ws` websocket path. The `room` channel was added alongside
  the existing `chat` and `signaling` channels in `auth.RealtimeTicketService`.
- A per-participant negotiation tracker coalesces bursts of track changes into
  a single re-offer and replays any change that arrives while one is in flight,
  so renegotiation never deadlocks under rapid join/leave churn.
- Capacities are enforced by `ROOM_MAX_PARTICIPANTS` above.
