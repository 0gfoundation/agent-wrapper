#!/bin/bash
# Demo script for agent-wrapper
# This script runs the agent-wrapper in demo mode with mock servers

set -e

echo "=========================================="
echo "Agent Wrapper Demo"
echo "=========================================="
echo ""

# Check if wrapper binary exists
if [ ! -f "./wrapper" ]; then
    echo "Building wrapper..."
    go build -o wrapper ./cmd/wrapper/
fi

# Set demo mode
export DEMO_MODE=true
export PORT=8080

# Set mock server endpoints (will be overridden by internal mocks if needed)
export STORAGE_ENDPOINT="https://storage.0g.io"
export ATTESTOR_URL="https://attestor.0g.io"
export BLOCKCHAIN_URL="https://blockchain.0g.io"

echo "Starting agent-wrapper in demo mode..."
echo "  - Demo agent will be used"
echo "  - Mock servers will be used for external services"
echo "  - Internal HTTP server on :8080"
echo ""
echo "To initialize the agent, send a POST request:"
echo "  curl -X POST http://localhost:8080/_internal/init \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{"
echo "      \"sealId\": \"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef\","
echo "      \"tempKey\": \"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef\","
echo "      \"attestorURL\": \"https://attestor.0g.io\""
echo "    }'"
echo ""
echo "Or use the init-demo.sh script"
echo ""
echo "=========================================="
echo ""

# Run the wrapper
./wrapper
