# Web Smoke Testing

AllCallAll Web uses the same Expo app as mobile, so the minimum useful smoke test is a real browser check against exported web assets.

## Anonymous route smoke

```bash
cd mobile
npx expo export --platform web --output-dir /tmp/allcallall-web-export
WEB_SMOKE_EXPORT_DIR=/tmp/allcallall-web-export node scripts/web-smoke.mjs
```

This starts a local static server with SPA fallback and checks:

- `/meetings`
- `/rooms/:roomId`
- `/conversations/:conversationId`

Without credentials, the test only verifies that direct routes render a usable page and do not throw browser page errors.

## Authenticated smoke

```bash
cd mobile
WEB_SMOKE_BASE_URL=http://127.0.0.1:8081 \
WEB_SMOKE_EMAIL=user@example.com \
WEB_SMOKE_PASSWORD='password' \
WEB_SMOKE_ROOM_ID=1 \
WEB_SMOKE_CONVERSATION_ID=1 \
node scripts/web-smoke.mjs
```

When credentials are provided, the script logs in first and then checks the Web collaboration paths after authentication. `WEB_SMOKE_ROOM_ID` and `WEB_SMOKE_CONVERSATION_ID` should point to records the test user can access.

Set these optional flags for a stricter browser flow:

- `WEB_SMOKE_JOIN_MEETING=1`: after opening `/rooms/:roomId`, use Chromium fake media devices, click Join, verify the meeting page, then leave back to the thread or meetings page.
- `WEB_SMOKE_DOWNLOAD_RECORDING=1`: open `/recordings`, click the first Download action, and verify the browser receives a downloadable file. The test user must have at least one accessible recording asset.

Example strict run:

```bash
cd mobile
WEB_SMOKE_BASE_URL=http://127.0.0.1:8081 \
WEB_SMOKE_EMAIL=user@example.com \
WEB_SMOKE_PASSWORD='password' \
WEB_SMOKE_ROOM_ID=1 \
WEB_SMOKE_CONVERSATION_ID=1 \
WEB_SMOKE_JOIN_MEETING=1 \
WEB_SMOKE_DOWNLOAD_RECORDING=1 \
node scripts/web-smoke.mjs
```

## Notes

- Web billing is intentionally degraded; subscription purchase and restore should be tested on native mobile until a separate Web billing flow exists.
- Web push is intentionally disabled in the first Web version.
- Browser media permissions must be granted manually when testing actual meetings.
