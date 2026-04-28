#!/bin/bash
# Initialize the demo agent-wrapper

set -e

WRAPPER_URL="${WRAPPER_URL:-http://localhost:8080}"

echo "Initializing agent-wrapper at $WRAPPER_URL..."

curl -X POST "$WRAPPER_URL/_internal/init" \
  -H "Content-Type: application/json" \
  -d "{
    \"sealId\": \"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef\",
    \"tempKey\": \"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef\",
    \"attestorURL\": \"https://attestor.0g.io\"
  }"

echo ""
echo ""
echo "Check status:"
echo "  curl $WRAPPER_URL/_internal/status"
echo ""
echo "Check agent health (after initialization completes):"
echo "  curl http://localhost:9000/health"
