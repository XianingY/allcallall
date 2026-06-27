# Web / Desktop Development Workflow

AllCallAll's primary browser client is the independent `web/` React + Vite + TypeScript app. The Expo app in `mobile/` remains the native Android/iOS client and is no longer used for production Web export.

## Web

Start the backend first:

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/server
```

Run the Web app:

```bash
cd web
npm install
npm run dev
```

Vite serves `http://localhost:5173` and proxies `/api` plus WebSocket traffic to `http://127.0.0.1:8080`.

Useful routes:

- `/inbox`
- `/meetings`
- `/meetings/:roomId/preflight`
- `/meetings/:roomId`
- `/recordings/:recordingId`
- `/agent-lab`
- `/knowledge`
- `/settings/billing`
- `/settings/notifications`

Browser auth uses:

- access token in memory only
- HttpOnly refresh cookie `allcallall_refresh`
- one-shot realtime tickets for browser WebSocket connections

Web verification:

```bash
cd web
npm run generate:api
npm run typecheck
npm run lint
npm test
npm run build
npx playwright test
```

Production Web config is generated at container startup as `/config.js` from public runtime variables:

- `PUBLIC_API_BASE_URL`
- `PUBLIC_WS_BASE_URL`
- `FIREBASE_*` and `FIREBASE_VAPID_KEY`
- `REVENUECAT_PUBLIC_API_KEY`

## Desktop

Desktop is an Electron shell around the Web client. Keep business logic in `web/`, not in Electron.

```bash
cd desktop
npm install
ALLCALLALL_WEB_URL=http://localhost:5173 npm run dev
```

Desktop checks:

```bash
cd desktop
npm run check
npm run build
```

Desktop behavior:

- opens to `/meetings`
- normalizes `allcallall://rooms/:roomId` to `/meetings/:roomId`
- normalizes `allcallall://conversations/:id` to `/conversations/:id`
- sends external `http:`, `https:`, and `mailto:` links to the system browser
- uses Electron's managed download flow for recording downloads
- stores managed downloads in `~/Downloads/AllCallAll` by default, or `ALLCALLALL_DOWNLOAD_DIR` when set

## Deployment

Production Compose builds `infra/Dockerfile.web`, which runs `npm run build` in `web/` and serves `web/dist` through Nginx. Nginx disables long-term caching for `index.html`, `config.js`, and `firebase-messaging-sw.js`; hashed assets under `/assets/` are cached immutably.

Legacy browser URLs:

- `/rooms/:id` redirects to `/meetings/:id`
- `/rooms/:id/preflight` redirects to `/meetings/:id/preflight`
- `/agent-demo` redirects to `/agent-lab`

## Intentional Limits

- Web Push requires Firebase Web app config, VAPID key, and backend `FCM_SERVICE_ACCOUNT_PATH`.
- Web billing requires `REVENUECAT_PUBLIC_API_KEY`; backend RevenueCat webhook remains the final entitlement authority.
- Desktop auto-update and native screen sharing are not implemented.
