# Observability (Prometheus + Grafana + Loki)

An optional, self-hosted observability stack is shipped with the repo so you do
not have to wire up monitoring separately. It is delivered as a Compose file
(`infra/docker-compose.observability.yml`) alongside the production deployment
and provides:

- **Prometheus** — scrapes the backend metrics endpoint `/api/v1/metrics`.
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

## Metrics endpoint & bearer token

The backend `/api/v1/metrics` endpoint is restricted to internal networks by
default and may optionally require a `METRICS_BEARER_TOKEN`. Because Prometheus
runs inside `allcallall_network`, the default setup needs no token. If you turn
the token on, edit `infra/observability/prometheus.yml` and uncomment the
`authorization:` block under the `backend` job, setting `credentials` to the
same `METRICS_BEARER_TOKEN`.

## Configuration files

All config lives under `infra/observability/`:

- `prometheus.yml` — scrape jobs (backend + self).
- `loki.yml`, `promtail.yml` — log ingestion (filesystem storage, Docker SD).
- `grafana/provisioning/datasources/` — auto-provisioned datasources.
- `grafana/provisioning/dashboards/` — a starter "AllCallAll Backend" dashboard;
  add your own JSON files here.

## Stop / teardown

```bash
docker compose -f infra/docker-compose.production.yml \
               -f infra/docker-compose.observability.yml down
```

Persistent data is kept in named volumes (`prometheus_data`, `loki_data`,
`grafana_data`) and is removed only with `down -v`.
