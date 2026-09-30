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
- Alertmanager: http://localhost:9093 — only when started with
  `--profile alerting`, see below

## Alerting

Alertmanager sits behind a Compose profile, because without a real
notification channel it is worse than nothing: it looks like alerting is
wired up while every alert is quietly discarded.

```bash
docker compose -f infra/docker-compose.production.yml \
               -f infra/docker-compose.observability.yml \
               --profile alerting up -d
```

Rules live in `infra/observability/alert.rules.yml` and are evaluated by
Prometheus, which forwards firing alerts to Alertmanager. Routing and
receivers live in `infra/observability/alertmanager.tmpl`.

> **That file is a template, not the config.** Alertmanager expands
> neither `{{ env "VAR" }}` nor `${VAR}`, so the webhook URLs are substituted
> at start-up by `render-alertmanager.sh`, which fails if any is missing. An
> earlier version put `{{ env ... }}` straight in the config and every alert
> was posted to a literal string - rules fired, nothing was delivered.

> **The container refuses to start without a webhook URL.** It checks
> `ALERTMANAGER_WEBHOOK_URL`, `ALERTMANAGER_WEBHOOK_URL_CRITICAL` and
> `ALERTMANAGER_WEBHOOK_URL_SECURITY` on boot and exits with an explanation
> if any is empty. This is deliberate: the previous default pointed at
> `http://localhost:9094/`, which started cleanly, passed config validation,
> and delivered nothing — indistinguishable from having no alerting, except
> the dashboard looked guarded.

Any endpoint that accepts Alertmanager's JSON payload works:

| Channel | What to put in the variable |
|---|---|
| Slack | an incoming webhook, or a relay that reshapes the payload for Slack |
| PagerDuty | `https://events.pagerduty.com/v2/enqueue` plus routing in PagerDuty |
| Email | not supported by `webhook_configs`; edit `alertmanager.tmpl` to use `email_configs` with SMTP settings instead |
| Generic | your own receiver; verify it with the smoke test below |

Set them in `.env` (see `.env.template`). Prometheus still runs without the
profile — it just logs that it cannot reach Alertmanager.

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

Do this once before believing the alerting works. "Prometheus shows the alert
as firing" only proves the rule evaluated — delivery is a separate hop.

```bash
# 0. Alertmanager actually came up (it exits if a webhook URL is missing)
docker compose -f infra/docker-compose.production.yml \
               -f infra/docker-compose.observability.yml \
               --profile alerting ps alertmanager

# 1. Prometheus loaded the rules and sees Alertmanager
curl -s http://localhost:9090/api/v1/rules | jq '.data.groups[].rules[].name'
curl -s http://localhost:9090/api/v1/alertmanagers | jq .

# 2. Fire a test alert and confirm it IS DELIVERED to your channel.
#    Seeing it in http://localhost:9093 is not the finish line.
curl -XPOST http://localhost:9093/api/v2/alerts \
  -H 'Content-Type: application/json' \
  -d '[{"labels":{"alertname":"SmokeTest","severity":"warning","instance":"manual"}}]'
```

If step 2 shows up in the Alertmanager UI but nothing arrives in Slack/mail,
the rules are fine and the receiver is not — fix the webhook URL, do not
touch the rules.

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

## Two metrics endpoints, on purpose

The backend exposes metrics on two different ports, which is easy to confuse:

| Port | Path | Content | How to reach it |
|---|---|---|---|
| 8080 | `/api/v1/metrics` | Self-rendered `CounterStore` text (`outbox_backlog`, `rag_runtime_*`, ...) used by the existing Grafana dashboard. Guarded by the internal-network check and optionally `METRICS_BEARER_TOKEN`. | Business port; already reachable from inside the network |
| 9090 | `/metrics` | Standard Prometheus registry: `http_requests_total`, `http_request_duration_seconds`, Go runtime and process metrics | Own port, so access can be limited to Prometheus only |

9090 exists because the standard registry had collectors registered with no
endpoint serving them — instrumentation that nothing could read. It is a
separate listener (see `internal/metrics.Serve`, configured under `metrics:` in
`configs/config.yaml`) so it never has to be threaded through the API
middleware chain.

### Kubernetes: allow Prometheus to scrape 9090

The api `NetworkPolicy` is default-deny. Scraping 9090 from another namespace
is therefore blocked until you declare where Prometheus lives:

```yaml
networkPolicy:
  prometheus:
    enabled: true
    namespace: monitoring          # namespace Prometheus runs in
    podLabels:                     # optional, narrows further
      app.kubernetes.io/name: prometheus
```

It ships disabled on purpose — enabling it without a namespace would open the
metrics port to the entire cluster.

## Configuration files

All config lives under `infra/observability/`:

- `prometheus.yml` — scrape jobs (backend + self), `rule_files`, Alertmanager
  target.
- `alert.rules.yml` — alerting rules.
- `alertmanager.tmpl` — routing, receivers and inhibition (rendered at start-up).
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
