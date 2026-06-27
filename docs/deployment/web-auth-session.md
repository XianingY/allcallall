# Web Auth Session

AllCallAll Web uses a split session model:

- Access token: short-lived JWT returned in the JSON auth response and stored by the Web client in memory.
- Refresh token: JWT stored only as an HttpOnly cookie named `allcallall_refresh`.
- Native mobile still uses secure Keychain storage for the same JSON auth response.

## Endpoints

- `POST /api/v1/auth/login`: returns `access_token` and sets the HttpOnly refresh cookie.
- `POST /api/v1/auth/register`: returns `access_token` and sets the HttpOnly refresh cookie.
- `POST /api/v1/auth/refresh`: reads the refresh cookie, returns a new `access_token`, and rotates the refresh cookie.
- `POST /api/v1/auth/logout`: revokes the current refresh session and clears the refresh cookie.
- `GET /api/v1/auth/sessions`: requires a bearer access token and returns a redacted session list for the current user.
- `DELETE /api/v1/auth/sessions/:sessionID`: revokes a non-current refresh session owned by the current user.
- `POST /api/v1/auth/logout-all`: requires a bearer access token, revokes all active refresh sessions for the current user, and clears the current refresh cookie.

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
- If `CORS_ALLOWED_ORIGINS` is not set, the backend allows only local development origins such as `http://localhost:5173`.

## Current Boundary

The backend persists refresh sessions in `refresh_sessions` using a SHA-256 hash of the refresh token, not the raw token value. Login and registration create a session, refresh rotates the session, logout revokes the current session, and logout-all revokes every active refresh session for the authenticated user.

If a previously rotated or revoked refresh token is reused, the backend records `invalid_use_count` and `last_invalid_use_at` on the original session row, and increments `refresh_session_invalid_use_total` in `/api/v1/metrics`. Treat these as support and risk signals; the refresh request still fails and clears the cookie.

The internal support user summary includes a redacted `refresh_sessions` block with active/revoked/expired counts, recent session metadata, invalid refresh reuse counters, `risk_level`, and `risk_reasons`. It never returns raw refresh tokens or token hashes. Support can force revoke one session with `DELETE /api/v1/internal/support/users/:userId/sessions/:sessionId` or all user sessions with `POST /api/v1/internal/support/users/:userId/sessions/revoke-all`.

Users can inspect redacted session records from Settings via **登录会话 / Active Sessions**. Active non-current sessions can be revoked individually, which prevents that device from refreshing its login state. The API refuses to revoke the current cookie-backed session and returns `CURRENT_SESSION_REVOKE_NOT_ALLOWED`; users should use regular logout or **退出所有设备 / Sign out everywhere** for the current device. The logout-all action calls `POST /api/v1/auth/logout-all`, clears the current refresh cookie, and removes the current device's local access-token cache.

Expired sessions are cleaned by the backend worker:

- `REFRESH_SESSION_CLEANUP_INTERVAL_MIN`: cleanup interval in minutes. Default: `1440`.
- `REFRESH_SESSION_REVOKED_RETENTION_DAYS`: how long revoked sessions are retained for support/debugging before deletion. Default: `7`.

Remaining production hardening items:

- Add external suspicious-session alert delivery.
