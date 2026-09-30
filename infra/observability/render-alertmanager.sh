#!/bin/sh
# Renders the Alertmanager template into the config Alertmanager actually reads.
#
# Alertmanager does not expand environment variables in its config file - it
# supports neither {{ env "VAR" }} nor ${VAR}. So the URLs have to be
# substituted before it starts. This script also refuses to produce a file
# while any value is missing, because a half-rendered config is what silently
# drops every alert.
set -eu

TEMPLATE="${TEMPLATE:-/etc/alertmanager/alertmanager.tmpl}"
OUTPUT="${OUTPUT:-/tmp/alertmanager.yml}"

for var in ALERTMANAGER_WEBHOOK_URL ALERTMANAGER_WEBHOOK_URL_CRITICAL ALERTMANAGER_WEBHOOK_URL_SECURITY; do
  eval "value=\"\${$var:-}\""
  if [ -z "$value" ]; then
    echo "ERROR: $var is not set." >&2
    echo "       Alerts would be evaluated and then dropped with no trace." >&2
    echo "       Set it in .env (see .env.template), or start the stack" >&2
    echo "       without --profile alerting." >&2
    exit 1
  fi
done

sed \
  -e "s|__ALERTMANAGER_WEBHOOK_URL__|${ALERTMANAGER_WEBHOOK_URL}|g" \
  -e "s|__ALERTMANAGER_WEBHOOK_URL_CRITICAL__|${ALERTMANAGER_WEBHOOK_URL_CRITICAL}|g" \
  -e "s|__ALERTMANAGER_WEBHOOK_URL_SECURITY__|${ALERTMANAGER_WEBHOOK_URL_SECURITY}|g" \
  "$TEMPLATE" > "$OUTPUT"

# Defence in depth: an unsubstituted placeholder would be posted to as a URL.
if grep -q "__ALERTMANAGER_" "$OUTPUT"; then
  echo "ERROR: placeholders remain in $OUTPUT after rendering." >&2
  exit 1
fi

echo "[alertmanager] rendered $OUTPUT"
