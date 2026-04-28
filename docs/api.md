# Agent Wrapper API Documentation

## Overview

Agent Wrapper provides HTTP endpoints for initialization, health checks, and proxying requests to agent frameworks.

**Base URL:** `http://localhost:8080`

Default port is configurable via `PORT` environment variable.

## Table of Contents

- [Internal Endpoints](#internal-endpoints)
- [Proxy Endpoints](#proxy-endpoints)
- [Error Responses](#error-responses)
- [Configuration](#configuration)

## Internal Endpoints

### POST /_internal/init

Initialize the wrapper with startup parameters.

**Request Body:**

```json
{
  "sealId": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
  "tempKey": "0xabcd1234567890abcdef",
  "attestorUrl": "https://attestor.0g.io"
}
```

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sealId` | string | Yes | Seal identifier (64 hex characters) |
| `tempKey` | string | Yes | Temporary attestation key (hex string) |
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
  "error": "invalid_request",
  "message": "Seal ID is required and must be a valid hex string"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Initialization started |
| 400 | Invalid parameters |
| 409 | Already initialized |
| 500 | Server error |

---

### GET /_internal/health

Check if the wrapper process is running.

**Response:**

```json
{
  "status": "healthy",
  "version": "0.2.0"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Wrapper is running |

---

### GET /_internal/ready

Check if the agent is ready to handle requests.

**Response (Ready):**

```json
{
  "ready": true,
  "agentId": "demo-agent-1234",
  "sealId": "0x1234...abcd",
  "framework": "openclaw",
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

**Possible States:**

| State | Description |
|-------|-------------|
| `waiting_init` | Waiting for POST /_internal/init |
| `sealed` | Sealed state, waiting for attestation |
| `attesting` | Performing remote attestation |
| `querying_agent` | Querying agentId from blockchain |
| `fetching_config` | Fetching configuration |
| `installing_framework` | Installing agent framework |
| `starting_agent` | Starting agent process |
| `ready` | Ready to serve requests |

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Check performed (see `ready` field) |
| 503 | Agent not ready |

## Proxy Endpoints

All other HTTP requests are proxied to the agent framework.

**Methods:** `GET`, `POST`, `PUT`, `DELETE`, `PATCH`, etc.

**Path:** `/*` (catch-all)

### Request

All headers and request bodies are forwarded to the agent framework.

```bash
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello!"}'
```

### Response

The response includes the agent's response plus signature headers:

| Header | Description |
|--------|-------------|
| `X-Agent-Id` | Agent identifier (hex) |
| `X-Seal-Id` | Seal identifier (hex) |
| `X-Signature` | ECDSA signature (128 hex characters) |
| `X-Timestamp` | Signature timestamp (Unix timestamp) |

**Example Response:**

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Agent-Id: 0x1234567890abcdef1234567890abcdef12345678
X-Seal-Id: 0x1234...abcd
X-Signature: 1a2b3c4d5e6f...
X-Timestamp: 1712787654

{
  "response": "Hello! How can I help you?"
}
```

### Signature Format

The signature is an ECDSA signature covering:

```
signature = Sign(
    agentId + "|" +
    sealId + "|" +
    timestamp + "|" +
    sha256(responseBody)
)
```

**Components:**

| Component | Description |
|-----------|-------------|
| `agentId` | Agent identifier from blockchain |
| `sealId` | Seal identifier from initialization |
| `timestamp` | Unix timestamp of signing |
| `sha256(responseBody)` | SHA-256 hash of response body |

### Verifying Signatures

```python
import hashlib
import ecdsa
from ecdsa import BadSignatureError

def verify_response(response_body, agent_id, seal_id, timestamp, signature_hex, public_key_hex):
    # Compute content hash
    data_hash = hashlib.sha256(response_body.encode()).hexdigest()
    content = f"{agent_id}|{seal_id}|{timestamp}|{data_hash}"
    content_hash = hashlib.sha256(content.encode()).digest()

    # Parse signature (64 bytes: R + S)
    signature_bytes = bytes.fromhex(signature_hex)

    # Verify using ECDSA
    vk = ecdsa.VerifyingKey.from_string(bytes.fromhex(public_key_hex[2:]))
    try:
        return vk.verify_digest(signature_bytes, content_hash)
    except BadSignatureError:
        return False
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 2xx | Success (forwarded from agent) |
| 4xx | Client error (forwarded from agent) |
| 5xx | Server error (wrapper or agent) |

## Error Responses

All error responses follow this format:

```json
{
  "error": "ERROR_CODE",
  "message": "Human readable error message"
}
```

### Error Codes

| Code | Description |
|------|-------------|
| `INVALID_REQUEST` | Malformed request |
| `ALREADY_INITIALIZED` | Wrapper already initialized |
| `AGENT_NOT_READY` | Agent is not ready |
| `PROXY_ERROR` | Error proxying to agent |
| `SIGNATURE_ERROR` | Error signing response |
| `ATTESTATION_ERROR` | Attestation failed |
| `CONFIG_ERROR` | Configuration error |
| `FRAMEWORK_INSTALL_ERROR` | Framework installation failed |
| `AGENT_START_ERROR` | Agent failed to start |

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | No | `:8080` | HTTP listen port |
| `DEMO_MODE` | No | `false` | Skip external service calls |
| `LOG_LEVEL` | No | `info` | Log level (debug/info/warn/error) |
| `STORAGE_ENDPOINT` | No | `https://storage.0g.io` | Storage service URL |
| `ATTESTOR_URL` | No | `https://attestor.0g.io` | Attestor service URL |
| `BLOCKCHAIN_URL` | No | `https://blockchain.0g.io` | Blockchain service URL |

### Startup Parameters

Startup parameters are provided via HTTP POST to `/_internal/init` (see above).

## Examples

### Initialization

```bash
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234567890abcdef",
    "tempKey": "0xabcd...",
    "attestorUrl": "https://attestor.0g.io"
  }'
```

### Health Check

```bash
curl http://localhost:8080/_internal/health
```

### Readiness Check

```bash
curl http://localhost:8080/_internal/ready
```

### Proxy Request

```bash
curl http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello!"}'
```

### Demo Mode

```bash
docker run -e DEMO_MODE=true -p 8080:8080 \
    0g-citizen-claw/agent-wrapper:latest
```

## Timeouts

| Operation | Default Timeout |
|-----------|-----------------|
| Proxy to agent | 30 seconds |
| Health check | 5 seconds |
| Ready check | 5 seconds |
| Initialization | 600 seconds (10 minutes) |

## Versioning

API follows semantic versioning. Current version: **v0.2.0**

### Changelog

#### v0.2.0
- Added `POST /_internal/init` endpoint
- Added demo mode support
- Removed command-line parameter support
- Added dynamic framework installation

#### v0.1.0
- Initial release with environment variable configuration
