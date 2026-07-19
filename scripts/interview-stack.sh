#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="${INTERVIEW_STATE_DIR:-/tmp/allcallall-interview-${USER:-user}}"
ENV_FILE="$STATE_DIR/interview.env"
COMPOSE_FILES=(-f "$ROOT_DIR/infra/docker-compose.yml" -f "$ROOT_DIR/infra/docker-compose.interview.yml")
SERVICES=(mysql redis elasticsearch migration interview-seed openbao sandbox-runner sandbox-control-plane rag-runtime agent-runtime backend search-worker web)

log() {
  printf '[interview-stack] %s\n' "$*"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'required command is unavailable: %s\n' "$1" >&2
    exit 1
  }
}

generate_state() {
  if [[ -f "$ENV_FILE" ]]; then
    return
  fi
  require_command openssl
  mkdir -p "$STATE_DIR/tls"
  chmod 700 "$STATE_DIR" "$STATE_DIR/tls"
  umask 077

  local mysql_password redis_password jwt_secret openbao_token interview_password
  mysql_password="$(openssl rand -hex 18)"
  redis_password="$(openssl rand -hex 18)"
  jwt_secret="$(openssl rand -hex 32)"
  openbao_token="interview-$(openssl rand -hex 18)"
  interview_password="Interview-$(openssl rand -hex 8)!"

  openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 2 \
    -subj "/CN=AllCallAll Interview CA" \
    -keyout "$STATE_DIR/tls/ca.key" -out "$STATE_DIR/tls/ca.crt" >/dev/null 2>&1
  openssl req -newkey rsa:2048 -sha256 -nodes \
    -subj "/CN=interview-mcp" \
    -keyout "$STATE_DIR/tls/interview-mcp.key" -out "$STATE_DIR/tls/interview-mcp.csr" >/dev/null 2>&1
  openssl x509 -req -sha256 -days 2 \
    -in "$STATE_DIR/tls/interview-mcp.csr" \
    -CA "$STATE_DIR/tls/ca.crt" -CAkey "$STATE_DIR/tls/ca.key" -CAcreateserial \
    -extfile <(printf 'subjectAltName=DNS:interview-mcp\n') \
    -out "$STATE_DIR/tls/interview-mcp.crt" >/dev/null 2>&1
  rm -f "$STATE_DIR/tls/interview-mcp.csr" "$STATE_DIR/tls/ca.srl"

  {
    printf 'COMPOSE_PROJECT_NAME=allcallall-interview\n'
    printf 'INTERVIEW_STATE_DIR=%s\n' "$STATE_DIR"
    printf 'MYSQL_ROOT_PASSWORD=%s\n' "root-$mysql_password"
    printf 'MYSQL_PASSWORD=%s\n' "$mysql_password"
    printf 'REDIS_PASSWORD=%s\n' "$redis_password"
    printf 'JWT_SECRET=%s\n' "$jwt_secret"
    printf 'MAIL_PASSWORD=mail-disabled-%s\n' "$redis_password"
    printf 'OPENBAO_TOKEN=%s\n' "$openbao_token"
    printf 'INTERVIEW_SEED_PASSWORD=%s\n' "$interview_password"
  } > "$ENV_FILE"
  "$ROOT_DIR/scripts/development/generate-agent-capability-keypair.sh" >> "$ENV_FILE"
  chmod 600 "$ENV_FILE" "$STATE_DIR"/tls/*
}

compose() {
	docker compose --env-file "$ENV_FILE" "${COMPOSE_FILES[@]}" --profile search --profile search-worker "$@"
}

wait_http() {
  local name="$1" url="$2" deadline
  deadline=$(( $(date +%s) + 180 ))
  until curl --fail --silent --show-error "$url" >/dev/null 2>&1; do
    if (( $(date +%s) >= deadline )); then
      log "$name did not become healthy: $url"
      compose ps
      return 1
    fi
    sleep 2
  done
  log "$name ready: $url"
}

show_access() {
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  printf '\nAllCallAll interview stack is ready.\n'
  printf 'Web:      http://localhost:3000/agent-lab\n'
  printf 'Tools:    http://localhost:3000/agent-tools\n'
  printf 'Email:    interview.owner@example.com\n'
  printf 'Password: %s\n\n' "$INTERVIEW_SEED_PASSWORD"
}

require_state() {
  [[ -f "$ENV_FILE" ]] || {
    log "no interview stack state exists; run make interview-up first"
    exit 1
  }
}

assert_job_succeeded() {
  local service="$1" container_id exit_code
  container_id="$(compose ps -a -q "$service")"
  [[ -n "$container_id" ]] || {
    log "$service container was not created"
    return 1
  }
  exit_code="$(docker inspect --format '{{.State.ExitCode}}' "$container_id")"
  [[ "$exit_code" == "0" ]] || {
    log "$service exited with code $exit_code"
    return 1
  }
}

up() {
  require_command docker
  require_command curl
  generate_state
  log "building and starting the interview stack"
  compose up --build -d "${SERVICES[@]}"
  wait_http backend http://localhost:18080/api/v1/ready
  wait_http agent-runtime http://localhost:18090/ready
  wait_http rag-runtime http://localhost:18091/ready
  wait_http sandbox-runner http://localhost:18093/health
  wait_http web http://localhost:3000/health
  wait_http elasticsearch http://localhost:19200/_cluster/health
}

smoke() {
  local login_payload login_response analyze_response
  require_command curl
  require_state
  # shellcheck disable=SC1090
  source "$ENV_FILE"

  wait_http backend http://localhost:18080/api/v1/ready
  wait_http agent-runtime http://localhost:18090/ready
  wait_http rag-runtime http://localhost:18091/ready
  wait_http sandbox-runner http://localhost:18093/health
  wait_http web http://localhost:3000/health
  wait_http elasticsearch http://localhost:19200/_cluster/health
  assert_job_succeeded migration
  assert_job_succeeded interview-seed

  login_payload="$(printf '{"email":"interview.owner@example.com","password":"%s"}' "$INTERVIEW_SEED_PASSWORD")"
  login_response="$(curl --fail --silent --show-error \
    -H 'Content-Type: application/json' \
    --data "$login_payload" \
    http://localhost:3000/api/v1/auth/login)"
  [[ "$login_response" == *'"access_token"'* ]] || {
    log "login smoke did not return an access token"
    return 1
  }

  analyze_response="$(curl --fail --silent --show-error \
    -X POST -H 'Content-Type: application/json' \
    --data '{"analyzer":"ik_smart","text":"腾讯全栈Agent工具审批流程"}' \
    http://localhost:19200/_analyze)"
  [[ "$analyze_response" == *'"腾讯"'* && "$analyze_response" == *'"审批"'* ]] || {
    log "Elasticsearch IK analyzer did not return the expected Chinese tokens"
    return 1
  }
  [[ -n "$(compose ps --status running -q search-worker)" ]] || {
    log "search-worker is not running"
    return 1
  }
  log "smoke passed: jobs, login, Web proxy, runtimes, Sandbox, and Elasticsearch IK"
}

chaos() {
  require_state
  log "restarting Agent Runtime"
  compose restart agent-runtime
  wait_http agent-runtime http://localhost:18090/ready
  log "basic runtime restart probe passed; approval-aware recovery is added in phase 3"
}

demo() {
  up
  smoke
  show_access
}

status() {
  require_state
  compose ps
  show_access
}

down() {
  if [[ ! -f "$ENV_FILE" ]]; then
    log "no interview stack state exists"
    return
  fi
  compose down --volumes --remove-orphans
  rm -rf "$STATE_DIR"
  log "removed containers, volumes, and temporary credentials"
}

case "${1:-up}" in
  up) up ;;
  smoke) smoke ;;
  chaos) chaos ;;
  demo) demo ;;
  status) status ;;
  down) down ;;
  *) printf 'usage: %s {up|smoke|chaos|demo|status|down}\n' "$0" >&2; exit 2 ;;
esac
