# Historical Auto-Env Detection Note

> This compatibility page preserves an older link. The maintained references are listed below.

Older docs described automatic dev/prod IP switching. That is no longer the active configuration model.

Current rule:

- Use `EXPO_PUBLIC_API_HTTP` for HTTP.
- Use `EXPO_PUBLIC_API_WS` for WebSocket.
- Use `EXPO_PUBLIC_FORCE_TLS=1` when a deployment should force HTTPS/WSS.
- Use `EXPO_PUBLIC_SIGNALING_TRANSPORT=poll` when WebSocket signaling is unreliable.

See [app-env-usage.md](app-env-usage.md) and [runtime configuration](../../reference/configuration/runtime.md) for the maintained references.
