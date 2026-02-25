#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ENV_FILE="${REPO_ROOT}/.env"
ANDROID_DIR="${REPO_ROOT}/mobile/android"

read_env_var() {
  local key="$1"
  local line
  line="$(grep -E "^${key}=" "${ENV_FILE}" 2>/dev/null | tail -n 1 || true)"
  if [[ -z "${line}" ]]; then
    return 0
  fi
  printf '%s' "${line#*=}"
}

strip_wrapping_quotes() {
  local value="$1"
  if [[ "${value}" =~ ^\".*\"$ ]] || [[ "${value}" =~ ^\'.*\'$ ]]; then
    value="${value:1:${#value}-2}"
  fi
  printf '%s' "${value}"
}

if [[ -f "${ENV_FILE}" ]]; then
  env_http_raw="$(read_env_var "EXPO_PUBLIC_API_HTTP")"
  env_ws_raw="$(read_env_var "EXPO_PUBLIC_API_WS")"
  public_ip_raw="$(read_env_var "PUBLIC_SERVER_IP")"

  env_http="$(strip_wrapping_quotes "${env_http_raw}")"
  env_ws="$(strip_wrapping_quotes "${env_ws_raw}")"
  public_ip="$(strip_wrapping_quotes "${public_ip_raw}")"

  # If PUBLIC_SERVER_IP is set, prefer it as the single source of truth.
  if [[ -n "${public_ip}" ]]; then
    env_http="http://${public_ip}"
    env_ws="ws://${public_ip}"
  fi

  if [[ -n "${env_http}" ]]; then
    export EXPO_PUBLIC_API_HTTP="${env_http}"
    echo "Using EXPO_PUBLIC_API_HTTP=${EXPO_PUBLIC_API_HTTP}"
  fi
  if [[ -n "${env_ws}" ]]; then
    export EXPO_PUBLIC_API_WS="${env_ws}"
    echo "Using EXPO_PUBLIC_API_WS=${EXPO_PUBLIC_API_WS}"
  fi
fi

cd "${ANDROID_DIR}"
exec ./gradlew -I gradle-mirrors.init.gradle "$@"
