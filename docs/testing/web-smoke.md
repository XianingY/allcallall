# Web Smoke Testing

AllCallAll Web is the independent React + Vite app in `web/`. Browser smoke tests should run against the Vite dev server, a production `web/dist` build, or a deployed Beta URL.

## Local Static Build Smoke

```bash
cd web
npm install
npm run build
npx vite preview --host 127.0.0.1 --port 4173
```

In another terminal:

```bash
cd web
WEB_E2E_BASE_URL=http://127.0.0.1:4173 npx playwright test
```

The Playwright suite covers unauthenticated route guards, protected app shell behavior, responsive layout, Agent Lab, knowledge center, settings, billing stubs, notification setup states, transcript timeline states, and core navigation.

## Development Server Smoke

```bash
cd web
VITE_API_BASE_URL=http://127.0.0.1:8080/api/v1 npm run dev -- --host 127.0.0.1
```

Then run:

```bash
cd web
WEB_E2E_BASE_URL=http://127.0.0.1:5173 npx playwright test
```

Useful direct routes:

- `/login`
- `/meetings`
- `/meetings/:roomId/preflight`
- `/meetings/:roomId`
- `/recordings/:recordingId`
- `/conversations/:conversationId`
- `/agent-lab`
- `/knowledge`
- `/settings/notifications`
- `/settings/billing`

## Authenticated Beta Smoke

For a deployed environment, configure protected test credentials and run the same Web package:

```bash
cd web
WEB_E2E_BASE_URL=https://app.example.com \
WEB_E2E_EMAIL=user@example.com \
WEB_E2E_PASSWORD='password' \
npx playwright test
```

Manual Beta checks that still need a real browser, media device, TURN/TLS endpoint, and backend data:

- Join a meeting from two browser contexts.
- Start and stop a recording.
- Wait for transcription status to move through `processing` to `ready`.
- Open the transcript timeline and jump to a cited segment.
- Generate a meeting brief from a ready transcript.
- Approve or reject a write tool call and verify side effects.
- Register Web Push, receive a foreground/background notification, and unregister the device.
- Open billing, purchase or restore in a RevenueCat sandbox environment, and verify backend entitlement refresh.

For the maintained product-level Beta loop, follow [Beta Smoke Checklist](./beta-smoke-checklist.md). Web Push and Billing require external Firebase/RevenueCat configuration and are not core gates for the Beta v1 collaboration loop.

## Notes

- Browser media permissions must be granted manually when testing actual meetings.
- Web Push requires Firebase public config in `/config.js` plus backend FCM service-account config.
- Web billing requires a RevenueCat Web public API key and sandbox/test product setup.
