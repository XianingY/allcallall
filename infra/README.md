# AllCallAll Infrastructure

Infrastructure assets for local development and interview/demo runtime profiles.

## Docs

- [Deployment Guide](../docs/deployment/deployment-guide.md)
- [Recording Storage And Transcription](../docs/deployment/recording-storage-deployment.md)
- [Restricted Network Setup](../docs/deployment/restricted-network-setup.md)

## Files

- `docker-compose.yml`: local MySQL/Redis plus optional worker, Kafka-compatible, and Elasticsearch profiles.
- `elasticsearch/Dockerfile`: Elasticsearch 8.19.10 with checksum-pinned IK and patched Bouncy Castle security-cli artifacts.
- `docker-compose.production.yml`: TLS Web/API, migration job, MySQL, Redis, persistent recordings, and Coturn Beta stack.

Older production-specific Compose and tunnel notes were removed from the maintained docs because they were host-specific. Use the deployment guide as the current source of truth.

## Common Commands

```bash
# Start local database/cache
docker compose -f infra/docker-compose.yml up -d mysql redis

# Stop local stack
docker compose -f infra/docker-compose.yml down

# Start interview infra profile
docker compose -f infra/docker-compose.yml \
  --profile microservices \
  --profile interview-infra \
  up api user-service outbox-worker data-worker search-worker kafka elasticsearch

# Validate the Beta stack after creating .env and infra/ssl certificates
docker compose --env-file .env -f infra/docker-compose.production.yml config
```

The Elasticsearch image exposes `ik_max_word` for indexing and `ik_smart` for
queries. Existing indices created before IK was enabled must be reindexed; the
backend intentionally rejects an existing incompatible mapping instead of
silently continuing with the standard analyzer.

## Observability (`observability/`)

Prometheus + Loki + Promtail + Grafana stack for local and demo environments.

| File | Purpose |
| --- | --- |
| `prometheus.yml` | Scrape config for API, workers and exporters |
| `loki.yml` | Log aggregation store |
| `promtail.yml` | Ships container logs into Loki |
| `grafana/` | Provisioned dashboards and datasources |

```bash
docker compose -f infra/docker-compose.observability.yml up -d
```

## High Availability (`ha/`)

Self-hosted HA reference for MySQL (3-node Group Replication + ProxySQL) and
Redis (Sentinel). Managed cloud databases or a Kubernetes Operator are preferred
in production. See [ha/README.md](./ha/README.md) for bootstrap ordering.

## WAF (`waf/`)

Two interchangeable rulesets placed in front of the edge:

| File | Target |
| --- | --- |
| `cloudflare-ruleset.yaml` | Cloudflare / edge gateway custom rules incl. AI-abuse and edge rate limiting |
| `modsecurity-rules.conf` | nginx + Coraza/ModSecurity3, OWASP CRS additions (prompt injection, SSRF, command injection) |

See [waf/README.md](./waf/README.md).

## Load Testing (`loadtest/`)

k6 end-to-end scripts covering status probe, chat flow, agent run and more.
See [loadtest/README.md](./loadtest/README.md). `scripts/load/` holds the
Node/Bash variants used by the interview suites.

## Backup (`backup/`)

`backup.sh` produces a consistent MySQL dump plus a Redis RDB snapshot, archives
them under `BACKUP_DIR`, rotates by `RETAIN_DAYS` and optionally ships offsite.
Restore is handled by `scripts/backup/restore.sh`. See [backup/README.md](./backup/README.md).

## Kubernetes and Helm

- `k8s/`: multi-AZ manifests for the API (namespace, configmap, secret template,
  deployment, service, HPA). See [k8s/README.md](./k8s/README.md).
- `helm/allcallall/`: the packaged chart. Start from
  `values-production.example.yaml` rather than editing `values.yaml` in place.

```bash
helm upgrade --install allcallall infra/helm/allcallall \
  -f infra/helm/allcallall/values-production.example.yaml
```

## Cloudflare Tunnel

`cloudflared-config.yml` and `deploy-cloudflare-tunnel.sh` are **host-specific
reference material** kept for deployments that must avoid exposing the origin IP.
Treat them as a starting point, not as the maintained source of truth — the
[deployment guide](../docs/deployment/deployment-guide.md) wins on conflicts.
