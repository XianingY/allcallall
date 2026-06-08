# Web Auth Session

AllCallAll Web uses a split session model:

- Access token: short-lived JWT returned in the JSON auth response and stored by the Web client in `sessionStorage`.
- Refresh token: JWT stored only as an HttpOnly cookie named `allcallall_refresh`.
- Native mobile still uses secure Keychain storage for the same JSON auth response.

## Endpoints

- `POST /api/v1/auth/login`: returns `access_token` and sets the HttpOnly refresh cookie.
- `POST /api/v1/auth/register`: returns `access_token` and sets the HttpOnly refresh cookie.
- `POST /api/v1/auth/refresh`: reads the refresh cookie, returns a new `access_token`, and rotates the refresh cookie.
- `POST /api/v1/auth/logout`: clears the refresh cookie.

## Cookie Behavior

- Cookie path is `/api/v1/auth`.
- Cookie is `HttpOnly`.
- Cookie uses `SameSite=Lax`.
- Cookie is marked `Secure` when the backend receives HTTPS directly or `X-Forwarded-Proto: https`.

## CORS

Web/Desktop browser clients need credentialed CORS when the app origin and API origin differ.

- Set `CORS_ALLOWED_ORIGINS` to a comma-separated allowlist, for example `https://app.example.com,https://desktop.example.com`.
- The backend returns `Access-Control-Allow-Credentials: true` only for allowlisted origins.
- Do not use `*` for credentialed auth traffic.
- If `CORS_ALLOWED_ORIGINS` is not set, the backend allows only local development origins such as `http://localhost:8081`.

## Current Boundary

This is the first production hardening step for Web sessions. It avoids long-lived auth material in browser persistent storage, but it does not yet maintain a server-side refresh-token rotation table. Add server-side refresh session records before treating refresh-token revocation, device management, and suspicious-session invalidation as complete.
