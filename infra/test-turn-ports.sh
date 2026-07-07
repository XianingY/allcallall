#!/bin/bash
# infra/test-turn-ports.sh

echo "Verifying Coturn port configuration..."
docker compose -f docker-compose.production.yml config | grep "40000-60000"

if [ $? -eq 0 ]; then
  echo "Coturn port range successfully expanded!"
else
  echo "Failed to find expanded port range in config"
  exit 1
fi
