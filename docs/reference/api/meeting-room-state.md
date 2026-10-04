# Meeting Room State Protocol

All meeting clients should treat `/api/v1/rooms/:roomId/state` as the canonical room snapshot and `chat/ws` room events as patch updates.

## Snapshot fields

Each room member now exposes:

- `joined`
- `left`
- `audio_enabled`
- `video_enabled`
- `connection_state`
- `is_host`

Clients should render the meeting tile, participant list, and top status bar from the same room snapshot model.

## Realtime events

The collaboration websocket can emit the following room events:

- `room.member.updated`
- `room.state.updated`
- `room.recording.updated`
- `room.ended`

`room.updated` remains available as a compatibility event, but new clients should prefer the fine-grained room events above.

## Client sync rules

1. On meeting join, fetch `/rooms/:roomId/state`.
2. Apply `room.member.updated` to the matching member only.
3. Apply `room.state.updated` and `room.recording.updated` as room-level patches.
4. On websocket reconnect, refetch `/rooms/:roomId/state` before renegotiating media.
5. Do not rely on screen-wide reloads for steady-state updates.
