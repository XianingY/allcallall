#!/bin/sh
set -eu

js_string() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

cat > /usr/share/nginx/html/config.js <<EOF
window.__ALLCALLALL_CONFIG__ = {
  apiBaseUrl: "$(js_string "${PUBLIC_API_BASE_URL:-/api/v1}")",
  wsBaseUrl: "$(js_string "${PUBLIC_WS_BASE_URL:-}")",
  firebase: {
    apiKey: "$(js_string "${FIREBASE_API_KEY:-}")",
    authDomain: "$(js_string "${FIREBASE_AUTH_DOMAIN:-}")",
    projectId: "$(js_string "${FIREBASE_PROJECT_ID:-}")",
    storageBucket: "$(js_string "${FIREBASE_STORAGE_BUCKET:-}")",
    messagingSenderId: "$(js_string "${FIREBASE_MESSAGING_SENDER_ID:-}")",
    appId: "$(js_string "${FIREBASE_APP_ID:-}")",
    vapidKey: "$(js_string "${FIREBASE_VAPID_KEY:-}")"
  },
  revenueCatPublicApiKey: "$(js_string "${REVENUECAT_PUBLIC_API_KEY:-}")"
};
EOF
