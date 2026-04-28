# Agent Wrapper User Guide

Agent Wrapper is a containerized service running in TEE (Trusted Execution Environment) that manages agent lifecycle for 0G Citizen Claw.

## Table of Contents

- [Quick Start](#quick-start)
- [Demo Mode](#demo-mode)
- [Production Mode](#production-mode)
- [HTTP Initialization](#http-initialization)
- [Deployment](#deployment)
- [API Endpoints](#api-endpoints)
- [Health Checks](#health-checks)
- [Troubleshooting](#troubleshooting)

## Quick Start

### Demo Mode (Fastest)

For testing and development:

```bash
# Run with demo mode
docker run -p 8080:8080 -e DEMO_MODE=true \
    0g-citizen-claw/agent-wrapper:latest

# Wait for ready status
curl http://localhost:8080/_internal/ready

# Send a request
curl http://localhost:8080/chat -d '{"message":"hello"}'
```

### Production Mode

For production deployment:

```bash
# Pull the image
docker pull 0g-citizen-claw/agent-wrapper:latest

# Start container
docker run -d -p 8080:8080 \
    --name my-agent \
    0g-citizen-claw/agent-wrapper:latest

# Initialize with your parameters
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234...abcd",
    "tempKey": "0xabcd...",
    "attestorUrl": "https://attestor.example.com"
  }'
```

## Demo Mode

Demo mode skips all external service calls for quick testing:

```bash
# Enable demo mode
export DEMO_MODE=true

# Or with Docker
docker run -e DEMO_MODE=true -p 8080:8080 \
    0g-citizen-claw/agent-wrapper:latest
```

### What Demo Mode Does

| Step | Production | Demo Mode |
|------|-----------|-----------|
| Key generation | ✅ Real | ✅ Real |
| Remote attestation | ✅ Real | ⏭️ Skipped |
| Query blockchain | ✅ Real | ⏭️ Skipped (mock agentId) |
| Fetch config | ✅ Real | ⏭️ Skipped (default config) |
| Framework install | ✅ Real | ⏭️ Skipped |
| Agent start | ✅ Configured | ✅ Demo agent |

### Demo Agent

The built-in demo agent is a simple Python HTTP server:

```python
# examples/demo-agent.py
# Responds to /chat with echo
```

## Production Mode

### Initialization Flow

```
1. Container starts
   └─ HTTP server listening on :8080

2. Send POST /_internal/init
   ├─ sealId (from 0g-sandbox)
   ├─ tempKey (your attestation key)
   └─ attestorUrl (attestation service)

3. Remote attestation
   ├─ Generate key pair
   ├─ Call Attestor.Attest()
   └─ Get agentSeal private key

4. Query blockchain
   ├─ Get agentId from sealId
   └─ Get IntelligentData[]

5. Fetch configuration
   ├─ Get encrypted config from Storage
   └─ Decrypt with agentSealKey

6. Install framework
   └─ pip/npm install from metadata

7. Start agent
   └─ Launch configured agent process

8. Ready to serve
```

## HTTP Initialization

### POST /_internal/init

**Request:**

```json
{
  "sealId": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
  "tempKey": "0xabcd1234...",
  "attestorUrl": "https://attestor.0g.io"
}
```

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sealId` | string | Yes | Seal identifier (64 hex chars) |
| `tempKey` | string | Yes | Temporary attestation key (hex) |
| `attestorUrl` | string | Yes | Attestor service URL |

**Response (Success):**

```json
{
  "status": "sealed",
  "message": "Entered sealed state, waiting for attestation"
}
```

**Response (Error):**

```json
{
  "error": "invalid_seal_id",
  "message": "Seal ID must be a valid hex string"
}
```

## Deployment

### Docker

```bash
# Pull image
docker pull 0g-citizen-claw/agent-wrapper:latest

# Run container
docker run -d \
  --name my-agent \
  -p 8080:8080 \
  -e STORAGE_ENDPOINT=https://storage.0g.io \
  -e ATTESTOR_URL=https://attestor.0g.io \
  -e BLOCKCHAIN_URL=https://blockchain.0g.io \
  0g-citizen-claw/agent-wrapper:latest

# Initialize
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "your-seal-id",
    "tempKey": "your-temp-key",
    "attestorUrl": "https://attestor.0g.io"
  }'
```

### Kubernetes

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: wrapper-config
data:
  STORAGE_ENDPOINT: "https://storage.0g.io"
  ATTESTOR_URL: "https://attestor.0g.io"
  BLOCKCHAIN_URL: "https://blockchain.0g.io"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agent-wrapper
spec:
  replicas: 1
  selector:
    matchLabels:
      app: agent-wrapper
  template:
    metadata:
      labels:
        app: agent-wrapper
    spec:
      containers:
      - name: wrapper
        image: 0g-citizen-claw/agent-wrapper:latest
        ports:
        - containerPort: 8080
          name: proxy
        envFrom:
        - configMapRef:
            name: wrapper-config
        livenessProbe:
          httpGet:
            path: /_internal/health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /_internal/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: agent-wrapper
spec:
  selector:
    app: agent-wrapper
  ports:
  - port: 8080
    targetPort: 8080
    name: proxy
  type: LoadBalancer
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `:8080` | HTTP listen port |
| `DEMO_MODE` | `false` | Enable demo mode |
| `LOG_LEVEL` | `info` | Log level (debug/info/warn/error) |
| `STORAGE_ENDPOINT` | `https://storage.0g.io` | Storage service URL |
| `ATTESTOR_URL` | `https://attestor.0g.io` | Attestor service URL |
| `BLOCKCHAIN_URL` | `https://blockchain.0g.io` | Blockchain service URL |

## API Endpoints

### Internal Endpoints

#### GET /_internal/health

Check if the wrapper process is running.

```bash
curl http://localhost:8080/_internal/health
```

**Response:**

```json
{
  "status": "healthy",
  "version": "0.2.0"
}
```

#### GET /_internal/ready

Check if the agent is ready to handle requests.

```bash
curl http://localhost:8080/_internal/ready
```

**Response (Ready):**

```json
{
  "ready": true,
  "agentId": "demo-agent-1234...",
  "sealId": "0x1234...",
  "framework": "demo",
  "version": "0.1.0"
}
```

**Response (Not Ready):**

```json
{
  "ready": false,
  "state": "initializing",
  "message": "Waiting for attestation"
}
```

#### POST /_internal/init

Initialize the wrapper (see [HTTP Initialization](#http-initialization)).

### Proxy Endpoints

All other requests are proxied to the agent framework with automatic signing.

```bash
curl http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello!"}'
```

**Response Headers:**

| Header | Description |
|--------|-------------|
| `X-Agent-Id` | Agent identifier |
| `X-Seal-Id` | Seal identifier |
| `X-Signature` | ECDSA signature (hex) |
| `X-Timestamp` | Unix timestamp |

## Health Checks

### Liveness Probe

```bash
curl http://localhost:8080/_internal/health
```

Returns HTTP 200 if the wrapper is running.

### Readiness Probe

```bash
curl http://localhost:8080/_internal/ready
```

Returns HTTP 200 if the agent is ready, HTTP 503 if still initializing.

## Troubleshooting

### Container exits immediately

**Problem:** Container exits without starting

**Solution:** Check logs
```bash
docker logs <container-id>
```

### Initialization timeout

**Problem:** `/_internal/ready` returns timeout

**Solution:** Check each service
```bash
# Check Attestor is reachable
curl https://attestor.0g.io/health

# Check Blockchain is reachable
curl https://blockchain.0g.io/health

# Check Storage is reachable
curl https://storage.0g.io/health
```

### Framework installation fails

**Problem:** Framework install step fails

**Solution:** Check framework name and version in your on-chain metadata

```bash
# Test pip install manually
docker exec <container-id> pip install openclaw==0.1.0
```

### Agent not responding

**Problem:** Proxy returns errors

**Solution:** Check if agent is running
```bash
# Check agent process
docker exec <container-id> ps aux | grep python

# Check agent port
docker exec <container-id> netstat -tlnp | grep 9000
```

### Debug mode

Enable detailed logging:

```bash
docker run -e LOG_LEVEL=debug -p 8080:8080 \
    0g-citizen-claw/agent-wrapper:latest
```

## Startup Logs

### Successful Demo Mode Startup

```
2024/04/23 10:00:00 Agent Wrapper v0.2.0 starting...
2024/04/23 10:00:00 HTTP server listening on :8080
2024/04/23 10:00:00 Waiting for initialization via POST /_internal/init
2024/04/23 10:00:05 Demo mode enabled - skipping external services
2024/04/23 10:00:05 Using demo configuration
2024/04/23 10:00:05 Skipping framework installation (demo mode)
2024/04/23 10:00:05 Starting demo agent...
2024/04/23 10:00:06 Agent started on port 9000
2024/04/23 10:00:06 Initialization flow complete. Agent ready.
```

### Successful Production Mode Startup

```
2024/04/23 10:00:00 Agent Wrapper v0.2.0 starting...
2024/04/23 10:00:00 HTTP server listening on :8080
2024/04/23 10:00:00 Waiting for initialization via POST /_internal/init
2024/04/23 10:00:05 Received init request for sealId: 0x1234...
2024/04/23 10:00:05 Public key generated: 0xabcd...
2024/04/23 10:00:05 Performing remote attestation...
2024/04/23 10:00:06 Attestation successful
2024/04/23 10:00:06 Querying agentId from sealId...
2024/04/23 10:00:07 Agent ID: 0x5678...
2024/04/23 10:00:07 Fetching intelligent datas...
2024/04/23 10:00:08 Found 1 intelligent data(s)
2024/04/23 10:00:08 Fetching encrypted config from Storage...
2024/04/23 10:00:09 Config decrypted successfully
2024/04/23 10:00:09 Installing framework: openclaw==0.1.0
2024/04/23 10:00:35 Framework installed
2024/04/23 10:00:35 Starting agent process...
2024/04/23 10:00:36 Agent started on port 9000
2024/04/23 10:00:36 Initialization flow complete. Agent ready.
```

## Next Steps

- [API Documentation](./api.md) - Detailed API reference
- [Architecture](./architecture.md) - Architecture diagrams
- [Design](./design.md) - Implementation details
- [Development](./development.md) - Contributing guide
