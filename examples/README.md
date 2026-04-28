# Agent Wrapper Examples

This directory contains example agents and scripts for testing the agent-wrapper.

## Demo Agent

The `demo-agent.py` is a simple HTTP server that simulates an AI agent for testing purposes.

### Features

- HTTP server on configurable port (default: 9000)
- Health check endpoint: `GET /health`
- Chat endpoint: `POST /chat`
- Responds with agent metadata from environment variables

### Running the Demo

#### Quick Start

```bash
# Terminal 1: Start the wrapper in demo mode
./examples/run-demo.sh

# Terminal 2: Initialize the agent
./examples/init-demo.sh

# Terminal 3: Test the agent
curl http://localhost:9000/health
curl -X POST http://localhost:9000/chat -H "Content-Type: application/json" -d '{"message":"hello"}'
```

#### Manual Steps

1. Build the wrapper:
```bash
go build -o wrapper ./cmd/wrapper/
```

2. Set demo mode environment variable:
```bash
export DEMO_MODE=true
export PORT=8080
```

3. Start the wrapper:
```bash
./wrapper
```

4. Initialize the agent (in another terminal):
```bash
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    "tempKey": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    "attestorURL": "https://attestor.0g.io"
  }'
```

5. Check the agent health:
```bash
curl http://localhost:9000/health
```

## Demo Mode

Demo mode (`DEMO_MODE=true`) enables the following:

- Uses the demo agent Python script instead of production entry point
- Skips framework installation (no pip install needed)
- Works outside of `/app` directory
- Simplified config for local testing

## Production vs Demo Mode

| Feature | Demo Mode | Production |
|---------|-----------|------------|
| Agent Script | `demo-agent.py` | Config entry point |
| Working Dir | Current dir | `/app` |
| Framework Install | Skipped | Full pip/npm install |
| External Services | Mocked (internal) | Real Attestor/Blockchain/Storage |

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DEMO_MODE` | Enable demo mode | `false` |
| `PORT` | Wrapper HTTP port | `8080` |
| `AGENT_PORT` | Agent HTTP port | `9000` |
| `STORAGE_ENDPOINT` | 0G Storage URL | `https://storage.0g.io` |
| `ATTESTOR_URL` | Attestor service URL | `https://attestor.0g.io` |
| `BLOCKCHAIN_URL` | Blockchain service URL | `https://blockchain.0g.io` |

## Creating a Custom Agent

To create your own agent:

1. Create an HTTP server that listens on `AGENT_PORT`
2. Implement a `/health` endpoint for health checks
3. Add any custom endpoints for your agent's functionality
4. Update the config to point to your entry point

Example agent structure:
```python
import os
from http.server import HTTPServer, BaseHTTPRequestHandler

class MyAgentHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"status":"healthy"}')

if __name__ == '__main__':
    port = int(os.environ.get('AGENT_PORT', 9000))
    server = HTTPServer(('0.0.0.0', port), MyAgentHandler)
    server.serve_forever()
```
