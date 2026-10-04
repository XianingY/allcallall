# Mobile Native Client Docs

The `mobile/` package is the Expo + React Native client for Android and iOS. Browser production traffic now uses the independent React + Vite app in `web/`; the Electron desktop shell also loads that Web app instead of an Expo Web bundle.

The current mobile focus is keeping native Android/iOS workflows buildable while the primary product surface moves to Web.

## Current Scope

- Android development client.
- iOS target exists, but iOS real-device validation is paused.
- Native push, native billing, media permissions, and WebRTC adapters remain in this package.
- Realtime translation UI entry points are hidden; backend compatibility endpoints remain.
- Meeting recording cards can show transcription status when backend data is available.

For browser workflows, use [Web/Desktop workflow](../guides/development/web-desktop-workflow.md).

## Runtime Configuration

Use `EXPO_PUBLIC_*` variables only:

```bash
EXPO_PUBLIC_API_HTTP=http://127.0.0.1:8080
EXPO_PUBLIC_API_WS=ws://127.0.0.1:8080
EXPO_PUBLIC_FORCE_TLS=0
EXPO_PUBLIC_SIGNALING_TRANSPORT=auto
```

`APP_ENV`, `DEV_API`, and `PROD_API_IP` are historical names and are not active runtime controls.

Detailed references:

- [Mobile runtime config](setup/app-env-usage.md)
- [Configuration](../reference/configuration/runtime.md)

## Run Android Development Client

```bash
cd ../.. # repository root when starting from docs/mobile
npm ci
adb reverse tcp:8080 tcp:8080
adb reverse tcp:8081 tcp:8081
cd mobile
npm run start:dev-client
```

Build/install the dev client:

```bash
cd mobile
npm run android
```

## Quality Gates

```bash
cd mobile
npm run test:unit
npm run test:jest
npm run typecheck
npm run lint
```

Mobile intentionally uses two test runners. Pure TypeScript logic runs through
the explicit file list in `test:unit`; tests that import React Native run under
Jest with the `jest-expo` preset. Add every new pure-logic test to the
`test:unit` command in `mobile/package.json`, because unlisted files are not
discovered automatically.

This package no longer owns the production Web export. Run browser checks from `web/`:

```bash
cd web
npm run typecheck
npm run lint
npm test
npm run build
npx playwright test
```

## Important Source Areas

```text
mobile/src/app/          Startup composition, providers, and deep-link handling
mobile/src/api/          Native API client and backend integration
mobile/src/config/       EXPO_PUBLIC_* runtime config
mobile/src/context/      Auth, signaling, rooms, follow-ups, billing
mobile/src/platform/     Native/cross-platform adapters
mobile/src/screens/      Meetings, Inbox, contacts, settings
mobile/src/services/     Push, billing, media, audio/video/vibration
```

The application root is deliberately thin: `mobile/App.tsx` mounts navigation
and top-level UI, `src/app/AppProviders.tsx` owns provider ordering,
`src/app/linking.ts` owns the navigation map and pure URL-to-intent resolution,
and `src/app/useDeepLinks.ts` owns the React Native subscription and deferred
navigation behavior.

## Supporting Docs

- [Web/Desktop workflow](../guides/development/web-desktop-workflow.md)
- [Web smoke tests](../guides/development/web-smoke.md)
- [Restricted network setup](../guides/deployment/restricted-networks.md)
- [Audio files setup](setup/audio-files-setup.md)
- [Mobile scripts](../../mobile/scripts/README.md)
