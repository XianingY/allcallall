# Web / Desktop Development Workflow

AllCallAll Web and Desktop reuse the Expo app and the same `/api/v1` backend. The current goal is a usable collaboration surface for demos and day-to-day internal work: `Meetings -> PreJoin -> Meeting -> Conversation -> Recording/Summary`.

## Web

Start the backend first, then run the Expo Web client:

```bash
cd mobile
EXPO_PUBLIC_API_HTTP=http://localhost:8080 \
EXPO_PUBLIC_API_WS=ws://localhost:8080 \
npm run web
```

Useful Web routes:

- `/meetings`
- `/rooms/:roomId`
- `/conversations/:conversationId`

Browser auth uses:

- short-lived access token in `sessionStorage`
- HttpOnly refresh cookie `allcallall_refresh`
- backend credentialed CORS via `CORS_ALLOWED_ORIGINS`

Recommended local backend env for Web:

```bash
export CONFIG_PATH=./configs/config.yaml
export CORS_ALLOWED_ORIGINS=http://localhost:8081,http://127.0.0.1:8081
```

Web verification:

```bash
cd mobile
npm run test:unit
npx tsc --noEmit
npm run lint
npm run web:smoke
```

`npm run web:smoke` can run without credentials for public route/export checks. Set `WEB_SMOKE_EMAIL`, `WEB_SMOKE_PASSWORD`, `WEB_SMOKE_ROOM_ID`, and `WEB_SMOKE_CONVERSATION_ID` to exercise authenticated collaboration routes.

## Desktop

Desktop is an Electron shell around the Web client. Keep business logic in the Expo/Web app, not in Electron main process.

```bash
cd desktop
npm install
ALLCALLALL_WEB_URL=http://localhost:8081 npm run build
```

Desktop checks:

```bash
cd desktop
npm run check
npm run build
```

Desktop behavior:

- opens to `/meetings`
- normalizes `allcallall://rooms/:roomId` and `allcallall://conversations/:id` into Web routes
- sends external `http:`, `https:`, and `mailto:` links to the system browser
- uses Electron's managed download flow for recording downloads

## Intentional Limitations

- Web push is disabled for the first Web version.
- Web billing is not implemented; subscription purchase/restore remains native-first.
- Desktop auto-update and native screen sharing are not implemented.
- iOS true-device closure is intentionally paused until device access is available.
