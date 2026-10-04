# Firebase / FCM Integration Guide

This guide reflects the current backend behavior. FCM is no longer a placeholder path:

- When `FCM_SERVICE_ACCOUNT_PATH` is configured, the backend initializes Firebase Admin SDK and sends real notifications.
- When it is not configured, the backend safely disables FCM and logs a single clear startup message.
- The backend should not log raw FCM tokens, credentials, verification codes, JWTs, or service account contents.

## Firebase Setup

1. Create a Firebase project in [Firebase Console](https://console.firebase.google.com/).
2. Add the Android app package used by the current Expo/React Native build.
3. Download `google-services.json` and place it in the Android native project only when building a Firebase-enabled Android client.
4. Create a Firebase service account key JSON for the backend.
5. Store the service account file outside git.

Example local backend run:

```bash
cd backend
FCM_SERVICE_ACCOUNT_PATH=/absolute/path/firebase-service-account.json \
CONFIG_PATH=./configs/config.yaml \
go run ./cmd/server/main.go
```

## Backend Behavior

FCM initialization lives in `backend/internal/fcm/manager.go` and is wired from the server runtime.

Expected enabled startup log:

```text
fcm enabled
```

Expected disabled startup log:

```text
fcm disabled
```

Call invite notifications use the existing signaling path and include a `call_id` in the data payload. Failed sends are logged as structured errors and should increment the relevant metrics where available.

## Mobile Token Registration

Mobile token lifecycle is handled in `mobile/src/services/PushNotificationService.ts`:

- Initial token acquisition stores current token in service state.
- Token refresh reports the new token through the API client.
- Login and registration success paths should report the current token when available.
- Settings can control local permission/client behavior, but do not imply a server-side disable toggle.

Protected endpoint:

```http
POST /api/v1/users/fcm-token
Authorization: Bearer <access_token>
Content-Type: application/json

{"fcm_token":"<token>"}
```

## Docker / Deployment Notes

Mount the service account file as a secret or read-only file and set `FCM_SERVICE_ACCOUNT_PATH`.

Docker example:

```bash
docker run \
  -v /secure/firebase-service-account.json:/run/secrets/firebase-service-account.json:ro \
  -e FCM_SERVICE_ACCOUNT_PATH=/run/secrets/firebase-service-account.json \
  allcallall-backend
```

Kubernetes is intentionally not part of the current implementation plan. If this project is later deployed on Kubernetes, use a Secret volume and keep the same `FCM_SERVICE_ACCOUNT_PATH` runtime contract.

## Smoke Test

1. Start backend with `FCM_SERVICE_ACCOUNT_PATH`.
2. Login from a Firebase-enabled Android client.
3. Confirm `/api/v1/users/fcm-token` succeeds.
4. Start a 1:1 call invite to that user.
5. Confirm backend logs `call notification sent` or a structured Firebase error.

## Troubleshooting

- `fcm disabled`: `FCM_SERVICE_ACCOUNT_PATH` is missing or intentionally unset.
- Firebase initialization error: verify path, file permissions, JSON validity, and Firebase project permissions.
- Send error: verify token belongs to the configured Firebase project and the Android app package matches.
- No token registered: inspect mobile permission state and `PushNotificationService` initialization path.
