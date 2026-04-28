# Agent Wrapper

> A containerized service that enables any agent framework to run on 0G Citizen Claw infrastructure with built-in TEE security, dynamic framework installation, and response signing.

## What It Is

Agent Wrapper is a **pre-built container image** that runs inside TEE to manage agent lifecycle. It handles all the complex stuff:

- ✅ Remote attestation with TEE
- ✅ Secure key management
- ✅ Encrypted configuration fetching
- ✅ **Dynamic framework installation**
- ✅ Response signing
- ✅ Heartbeat monitoring
- ✅ Health checks

**Your agent framework doesn't need to know any of this exists.**

## Quick Start

### 1. Use the Official Image

```bash
docker pull 0g-citizen-claw/agent-wrapper:latest
```

### 2. Deploy to 0g-sandbox

```bash
sandbox start agent-wrapper:latest
```

### 3. Initialize via HTTP

```bash
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234...abcd",
    "tempKey": "0xabcd...",
    "attestorUrl": "https://attestor.example.com"
  }'
```

That's it. The wrapper will:
- Perform remote attestation
- Fetch your agent's metadata from the blockchain
- Dynamically install the required framework (openclaw, eliza, etc.)
- Start your agent
- Begin proxying and signing responses

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│                    0g-sandbox (TEE)                          │
│                                                              │
│  1. 启动官方镜像                                              │
│  2. HTTP 传入参数: {sealId, tempKey, attestorUrl}           │
│                         │                                    │
│                         ▼                                    │
│  ┌─────────────────────────────────────────────────────────┐│
│  │  Container                                              ││
│  │                                                          ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │  Agent Wrapper (Go)                                │││
│  │  │  1. Receive init params via HTTP                   │││
│  │  │  2. Remote attestation → Get key                   │││
│  │  │  3. Listen for sealId binding event                │││
│  │  │  4. Fetch metadata from blockchain                 │││
│  │  │  5. Fetch encrypted config from Storage            │││
│  │  │  6. Dynamically install framework (pip/npm)        │││
│  │  │  7. Start agent process                            │││
│  │  │  8. Proxy :8080 → :9000                            │││
│  │  │  9. Sign responses                                 │││
│  │  └────────────────────────────────────────────────────┘││
│  │                          │                              ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │  Agent Framework (Dynamically Installed)           │││
│  │  │  - OpenClaw / Eliza / Custom                       │││
│  │  │  - No awareness of wrapper                         │││
│  │  │  - Just handles HTTP requests                      │││
│  │  └────────────────────────────────────────────────────┘││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## HTTP Initialization

Instead of environment variables or command-line arguments, the wrapper accepts startup parameters via a simple HTTP endpoint:

**POST** `/_internal/init`

```json
{
  "sealId": "0x1234...abcd",
  "tempKey": "0xabcd...",
  "attestorUrl": "https://attestor.example.com"
}
```

Response:
```json
{
  "status": "sealed",
  "message": "Entered sealed state, waiting for attestation"
}
```

## On-Chain Metadata

Your agent's framework and version are registered on-chain:

```json
{
  "framework": "openclaw",
  "version": "0.1.0",
  "configHash": "0x5678...",
  "imageHash": "0x9abc..."
}
```

The wrapper reads this metadata and automatically installs the required framework.

## What Your Agent Sees

```python
# Your agent code (unchanged)
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route("/chat", methods=["POST"])
def chat():
    data = request.json
    # Process request...
    return jsonify({"response": "hello!"})

if __name__ == "__main__":
    app.run(port=9000)  # Wrapper forwards to this
```

**No wrapper imports, no signature code, no heartbeat logic.**

## What Clients Receive

```http
POST /chat HTTP/1.1
Host: your-agent.0g.io
Content-Type: application/json

{"message": "hello"}
```

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Agent-Id: 0x1234...
X-Signature: 0xabc... (ECDSA signature)
X-Timestamp: 1712787654

{"response": "hello!"}
```

The signature proves the response came from your TEE-protected agent.

## Supported Frameworks

Frameworks are dynamically installed based on on-chain metadata:

| Framework | Language | Installed Via |
|-----------|----------|---------------|
| OpenClaw | Python | `pip install openclaw=={version}` |
| Eliza | Node.js | `npm install @eliza/core@{version}` |
| Custom | Any | Custom install script |

## Architecture

```
External Request → Wrapper (:8080) → Agent (:9000)
                           │
                           ├─ Remote attestation
                           ├─ Fetches encrypted config
                           ├─ Manages private keys
                           ├─ Dynamically installs framework
                           ├─ Signs responses
                           └─ Sends heartbeats
```

## API Endpoints

### Internal (Control)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/_internal/init` | Initialize with startup params |
| GET | `/_internal/health` | Health check |
| GET | `/_internal/ready` | Readiness check |

### Proxy (Traffic)

| Method | Path | Description |
|--------|------|-------------|
| ANY | `/*` | Proxied to agent with signed response |

## Documentation

- [用户指南 (Usage Guide)](./docs/usage.md) - 部署和运行说明
- [Architecture Diagram](./docs/architecture.md) - Visual architecture overview
- [Design Document](./docs/design.md) - Architecture details
- [API Reference](./docs/api.md) - HTTP endpoints
- [Development Guide](./docs/development.md) - Contributing
- [CLAUDE.md](./CLAUDE.md) - For Claude Code

## Migration from Previous Design

If you were using the previous version of Agent Wrapper:

| Old Approach | New Approach |
|--------------|--------------|
| Build custom image with wrapper | Use official pre-built image |
| Pass params via environment variables | Pass params via HTTP API |
| Install framework in image | Framework installed dynamically |
| `ENTRYPOINT ["/usr/local/bin/wrapper", "--", "python", "main.py"]` | `ENTRYPOINT ["/usr/local/bin/wrapper"]` |

## License

MIT
