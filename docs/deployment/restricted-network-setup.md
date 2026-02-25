# Restricted Network Deployment

This project can operate in networks that only allow outbound HTTPS/TLS and/or require HTTP proxies.

## Mobile configuration (Expo)

Use `EXPO_PUBLIC_*` environment variables.

- `EXPO_PUBLIC_API_HTTP`: override API base, e.g. `https://api.example.com`
- `EXPO_PUBLIC_API_WS`: override WS base, e.g. `wss://api.example.com`
- `EXPO_PUBLIC_FORCE_TLS`: set to `1` to force `https://` and `wss://`
- `EXPO_PUBLIC_RESTRICTED_NETWORK`: set to `1` to prefer `turns:...:443?transport=tcp` ICE servers
- `EXPO_PUBLIC_SIGNALING_TRANSPORT`: `auto` (default) or `poll` (HTTPS long-poll signaling)
- `EXPO_PUBLIC_SIGNALING_SHAPING`: set to `1` to enable keepalive `client.ping` messages

## Nginx TLS termination

`infra/nginx.tls.conf` is a template that:
- redirects `80 -> 443`
- terminates TLS on 443
- proxies `/api/v1/` and WebSocket upgrades on `/api/v1/ws`

Mount certs into the nginx container at:
- `/etc/nginx/ssl/fullchain.pem`
- `/etc/nginx/ssl/privkey.pem`

## TURN over TLS (TURNS) on 443

`infra/docker-compose.turn.yml` and `infra/turnserver.conf` provide a minimal coturn example.

Client ICE server entries should include:
- `turns:turn.example.com:443?transport=tcp`

Certificates for coturn are expected at:
- `/etc/coturn/ssl/fullchain.pem`
- `/etc/coturn/ssl/privkey.pem`

## Signaling HTTPS fallback (proxy-friendly)

Backend endpoints (JWT protected):
- `POST /api/v1/signaling/send`
- `GET /api/v1/signaling/poll?timeout_ms=25000`

These are intended for environments where WebSockets are blocked by corporate proxies.
