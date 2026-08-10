#!/usr/bin/env bash
set -euo pipefail

API_BASE="${API_BASE:-http://localhost:8080}"
API_KEY="${API_KEY:-dev-tenant-key}"
ENDPOINT_URL="${ENDPOINT_URL:-http://localhost:8090/webhook}"

echo "Health..."
curl -sf "$API_BASE/healthz" | tee /tmp/webhook-health.json
echo

echo "Create endpoint..."
EP=$(curl -sf -X POST "$API_BASE/api/v1/endpoints" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"smoke\",\"url\":\"$ENDPOINT_URL\",\"secret\":\"demo-secret\"}")
echo "$EP"
EP_ID=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])' <<<"$EP")

echo "Publish event..."
EV=$(curl -sf -X POST "$API_BASE/api/v1/events" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"endpoint_id\":\"$EP_ID\",\"event_type\":\"order.paid\",\"payload\":{\"ok\":true}}")
echo "$EV"
EV_ID=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["event"]["id"])' <<<"$EV")

echo "Waiting for delivery attempts..."
for i in $(seq 1 20); do
  DETAIL=$(curl -sf "$API_BASE/api/v1/events/$EV_ID" -H "X-API-Key: $API_KEY")
  STATUS=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["event"]["status"])' <<<"$DETAIL")
  echo "  attempt $i status=$STATUS"
  if [[ "$STATUS" == "delivered" || "$STATUS" == "dead_letter" ]]; then
    echo "$DETAIL"
    exit 0
  fi
  sleep 1
done

echo "Timed out waiting for terminal status"
exit 1
