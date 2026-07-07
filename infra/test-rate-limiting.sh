#!/bin/bash
# infra/test-rate-limiting.sh

# Start nginx with rate limiting
docker compose -f infra/docker-compose.yml up -d nginx

# Send 100 requests rapidly
echo "Testing rate limiting..."
for i in {1..100}; do
  response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost/api/v1/health)
  echo "Request $i: $response"
  
  # Check if we hit rate limit (429)
  if [ "$response" == "429" ]; then
    echo "Rate limiting working - got 429 on request $i"
    break
  fi
done

# Verify upstream keepalive
echo "Checking upstream keepalive..."
docker exec allcallall-nginx-1 cat /var/log/nginx/access.log | grep "keepalive" | tail -5
