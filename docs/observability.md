# Observability (Prometheus + Grafana + Loki)

An optional, self-hosted observability stack is shipped with the repo so you do
not have to wire up monitoring separately. It is delivered as a Compose file
(`infra/docker-compose.observability.yml`) alongside the production deployment
and provides:

- **Prometheus** — scrapes the backend metrics endpoint `/api/v1/metrics`.
- **Alertmanager** — receives alerts from Prometheus and routes them.
- **Grafana** — dashboards, pre-provisioned with Prometheus + Loki datasources.
- **Loki + Promtail** — collects container logs via the Docker socket.

## Enable

Run it together with the production stack so the monitoring services share the
`allcallall_network` with the backend (Prometheus is then "internal" and can
scrape `/api/v1/metrics` without the `METRICS_BEARER_TOKEN`):

```bash
docker compose -f infra/docker-compose.production.yml \
               -f infra/docker-compose.observability.yml up -d
```

Then open:

- Grafana: http://localhost:3000 (default admin/admin; override with
  `GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD`)
- Prometheus: http://localhost:9090
- Alertmanager: http://localhost:9093

## Alerting

Rules live in `infra/observability/alert.rules.yml` and are evaluated by
Prometheus, which forwards firing alerts to Alertmanager. Routing and
receivers live in `infra/observability/alertmanager.yml`.

> **Alerting is not done until a receiver actually delivers.** The shipped
> receivers read their webhook URL from the environment and fall back to a
> placeholder that goes nowhere. Set `ALERTMANAGER_WEBHOOK_URL` (and the
> `_CRITICAL` / `_SECURITY` variants) to a real endpoint, otherwise alerts are
> evaluated and dropped — rules firing into a receiver nobody reads is the same
> silence as having no rules.

Available rules (all expressions use metrics the backend really publishes):

| Alert | Severity | Condition |
|---|---|---|
| `BackendDown` | critical | `up{job="backend"} == 0` for 2m |
| `OutboxBacklogHigh` | warning | `outbox_backlog > 1000` for 10m |
| `OutboxBacklogCritical` | critical | `outbox_backlog > 10000` for 5m |
| `OutboxDeadLettersIncreasing` | warning | any dead-lettered events in 1h |
| `OutboxWorkerErrors` | warning | >5 worker errors in 15m |
| `RealtimeDeliveryFailing` | warning | >50 chat delivery failures in 15m |
| `RefreshTokenReuseDetected` | warning | >10 invalid refresh-token uses in 1h |

Thresholds are intentionally loose — they are meant to catch sustained
breakage. Tighten them once you have a few weeks of baseline.

### Verify the chain end to end

```bash
# 1. Prometheus loaded the rules and sees Alertmanager
curl -s http://localhost:9090/api/v1/rules | jq '.data.groups[].rules[].name'
curl -s http://localhost:9090/api/v1/alertmanagers | jq .

# 2. Fire a test alert and confirm it is delivered to your channel
curl -XPOST http://localhost:9093/api/v2/alerts \
  -H 'Content-Type: application/json' \
  -d '[{"labels":{"alertname":"SmokeTest","severity":"warning","instance":"manual"}}]'
```

### Runbooks

#### Backend down

1. Check pod/container liveness and readiness.
2. Confirm the probe endpoints are reachable: they are registered on a route
   group that does **not** apply `RequireTLS`, because kubelet probes do not
   carry `X-Forwarded-Proto`. If `/api/v1/health` returns 403, that wiring has
   regressed (see `internal/server/routes.go`).
3. If `up` is 0 but the process is healthy, the scrape path is the problem —
   check that `/api/v1/metrics` is not being blocked by the internal-network
   guard or a `METRICS_BEARER_TOKEN` mismatch.

#### Refresh token reuse

`refresh_session_invalid_use_total` counts uses of a refresh token that the
server already considers consumed. A small steady trickle is usually a client
retrying with a stale token; a sudden sustained rate can mean a stolen token
being replayed. Pull the affected sessions before dismissing it as a client
bug.

## Metrics endpoint & bearer token

The backend `/api/v1/metrics` endpoint is restricted to internal networks by
default and may optionally require a `METRICS_BEARER_TOKEN`. Because Prometheus
runs inside `allcallall_network`, the default setup needs no token. If you turn
the token on, edit `infra/observability/prometheus.yml` and uncomment the
`authorization:` block under the `backend` job, setting `credentials` to the
same `METRICS_BEARER_TOKEN`.

## Configuration files

All config lives under `infra/observability/`:

- `prometheus.yml` — scrape jobs (backend + self), `rule_files`, Alertmanager
  target.
- `alert.rules.yml` — alerting rules.
- `alertmanager.yml` — routing, receivers and inhibition.
- `loki.yml`, `promtail.yml` — log ingestion (filesystem storage, Docker SD).
- `grafana/provisioning/datasources/` — auto-provisioned datasources.
- `grafana/provisioning/dashboards/` — a starter "AllCallAll Backend" dashboard;
  add your own JSON files here.

## Stop / teardown

```bash
docker compose -f infra/docker-compose.production.yml \
               -f infra/docker-compose.observability.yml down
```

Persistent data is kept in named volumes (`prometheus_data`,
`alertmanager_data`, `loki_data`, `grafana_data`) and is removed only with
`down -v`.
