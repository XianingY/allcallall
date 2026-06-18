# Mobile / Web Client Docs

The `mobile/` package is the shared Expo + React Native client for Android and Web. It also provides the Web surface consumed by the Electron desktop shell.

The current project focus is backend/Agent interview readiness, so client docs emphasize how to run and verify the frontend surfaces rather than commercial launch instructions.

## Current Scope

- Android development client.
- Web browser workspace.
- Electron desktop shell through Web.
- iOS target exists, but iOS real-device validation is paused.
- Web push and Web billing are intentionally not implemented.
- Realtime translation UI entry points are hidden; the backend compatibility endpoint remains.
- Meeting recording cards show transcription status when backend data is available.

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
- [Configuration](../configuration/configuration.md)

## Run Web

```bash
cd mobile
npm install
EXPO_PUBLIC_API_HTTP=http://127.0.0.1:8080 \
EXPO_PUBLIC_API_WS=ws://127.0.0.1:8080 \
npm run web
```

Useful routes:

- `/meetings`
- `/rooms/:roomId`
- `/conversations/:conversationId`

## Run Android Development Client

```bash
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
npx tsc --noEmit
npm run lint
npx expo export --platform web
```

If Web export fails, check for native-only imports and move them behind platform adapters.

## Important Source Areas

```text
mobile/src/api/          API client and backend integration
mobile/src/config/       EXPO_PUBLIC_* runtime config
mobile/src/context/      Auth, signaling, rooms, follow-ups, billing
mobile/src/platform/     Cross-platform adapters
mobile/src/screens/      Meetings, Inbox, contacts, settings
mobile/src/services/     Push, billing, media, audio/video/vibration
```

## Supporting Docs

- [Web/Desktop workflow](../development/web-desktop-workflow.md)
- [Web smoke tests](../testing/web-smoke.md)
- [Restricted network setup](../deployment/restricted-network-setup.md)
- [Audio files setup](setup/audio-files-setup.md)
- [Mobile scripts](../../mobile/scripts/README.md)
