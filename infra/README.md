# AllCallAll Infrastructure

Infrastructure assets for local development and interview/demo runtime profiles.

## Docs

- [Deployment Guide](../docs/deployment/deployment-guide.md)
- [Recording Storage And Transcription](../docs/deployment/recording-storage-deployment.md)
- [Restricted Network Setup](../docs/deployment/restricted-network-setup.md)

## Files

- `docker-compose.yml`: local MySQL/Redis plus optional worker, Kafka-compatible, and Elasticsearch profiles.
- `elasticsearch/Dockerfile`: Elasticsearch 8.19.10 with the checksum-pinned IK analysis plugin.
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
