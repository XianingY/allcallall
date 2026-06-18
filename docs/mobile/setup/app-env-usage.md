# Mobile Runtime Configuration

AllCallAll mobile/Web uses Expo `EXPO_PUBLIC_*` variables. Do not use `APP_ENV`, `DEV_API`, or `PROD_API_IP` for new work.

## Defaults

- HTTP default: `http://127.0.0.1:8080`
- WebSocket default: `ws://127.0.0.1:8080`
- Android physical-device development usually uses ADB reverse.

```bash
adb reverse tcp:8080 tcp:8080
adb reverse tcp:8081 tcp:8081
```

## Supported Variables

| Variable | Purpose |
| --- | --- |
| `EXPO_PUBLIC_API_HTTP` | REST API base URL. |
| `EXPO_PUBLIC_API_WS` | WebSocket base URL. |
| `EXPO_PUBLIC_FORCE_TLS` | `1` upgrades HTTP/WS to HTTPS/WSS. |
| `EXPO_PUBLIC_RESTRICTED_NETWORK` | Prefer restricted-network ICE/signaling behavior. |
| `EXPO_PUBLIC_SIGNALING_TRANSPORT` | `auto` or `poll`. |
| `EXPO_PUBLIC_SIGNALING_SHAPING` | Enables conservative signaling shaping. |
| `EXPO_PUBLIC_E2EE_MODE` | `experimental` enables experimental E2EE mode. |
| `EXPO_PUBLIC_REVENUECAT_*` | Android subscription demo configuration. |

Translation variables still exist in config for compatibility, but realtime translation UI is hidden and should not be presented as the current primary workflow.

## Common Commands

Web:

```bash
cd mobile
EXPO_PUBLIC_API_HTTP=http://127.0.0.1:8080 \
EXPO_PUBLIC_API_WS=ws://127.0.0.1:8080 \
npm run web
```

Android dev client:

```bash
cd mobile
EXPO_PUBLIC_API_HTTP=http://127.0.0.1:8080 \
EXPO_PUBLIC_API_WS=ws://127.0.0.1:8080 \
npm run start:dev-client
```

Restricted network:

```bash
EXPO_PUBLIC_API_HTTP=https://api.example.com \
EXPO_PUBLIC_API_WS=wss://api.example.com \
EXPO_PUBLIC_FORCE_TLS=1 \
EXPO_PUBLIC_RESTRICTED_NETWORK=1 \
EXPO_PUBLIC_SIGNALING_TRANSPORT=poll \
npm run web
```

## Verification

```bash
cd mobile
bash scripts/verify-app-env.sh
npx tsc --noEmit
npm run lint
```
