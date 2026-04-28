# Agent Wrapper - Agent Instructions

## Deployment Model

Agent Wrapper is a **transparent middleware** - downstream agent frameworks have ZERO knowledge of its existence.

```
┌─────────────────────────────────────────────────────────────┐
│                    0g-sandbox (TEE)                          │
│                                                              │
│  Environment Variables: SEAL_ID, TEMP_KEY, ATTESTOR_URL...  │
│                         │                                    │
│                         ▼                                    │
│  ┌─────────────────────────────────────────────────────────┐│
│  │  Container ENTRYPOINT: /usr/local/bin/wrapper            ││
│  │                                                          ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │  Agent Wrapper (Go)                               │││
│  │  │                                                     │││
│  │  │  1. Read environment variables                     │││
│  │  │  2. Attestor.Attest() → Get token                  │││
│  │  │  3. Blockchain.WaitForBinding(sealId) → agentId     │││
│  │  │  4. Attestor.GetKey(token) → agentSealPrivateKey   │││
│  │  │  5. Storage.FetchConfig() → encryptedConfig        │││
│  │  │  6. Decrypt(agentSealPrivateKey) → config          │││
│  │  │  7. Start Agent (exec entryPoint)                  │││
│  │  │  8. Start HTTP Proxy (:8080)                       │││
│  │  │  9. Start Heartbeat                                │││
│  │  └────────────────────────────────────────────────────┘││
│  │                                    │                    ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │  Downstream Agent (:9000)                          │││
│  │  │  - No awareness of wrapper                         │││
│  │  │  - Normal HTTP server                              │││
│  │  │  - Business logic as usual                         │││
│  │  └────────────────────────────────────────────────────┘││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## Downstream Developer Experience

### What They Do

1. **Add wrapper to Dockerfile**
```dockerfile
FROM python:3.11-slim
RUN wget -q https://.../wrapper-linux-amd64 -O /usr/local/bin/wrapper && chmod +x /usr/local/bin/wrapper
RUN pip install openclaw
COPY . /app
ENTRYPOINT ["/usr/local/bin/wrapper", "--", "python", "main.py"]
```

2. **Upload config to 0G Storage**
3. **Push image to registry**

### What They Don't Do

| ❌ Don't Need | ✅ Why |
|---------------|--------|
| Call wrapper CLI | Sandbox starts container |
| Modify agent code | Wrapper is transparent |
| Implement signing | Wrapper signs responses |
| Handle heartbeats | Wrapper sends them |
| Know about TEE | Wrapper handles everything |

## Module Responsibilities

### Bootstrap Module

**Purpose:** Orchestrate startup flow

**Flow:**
```
1. Parse environment variables (from sandbox)
2. Create clients: Attestor, Storage, Config
3. Run attestation flow
4. Get agentSeal private key
5. Fetch and decrypt config
6. Start downstream agent
7. Start HTTP proxy
8. Start heartbeat
```

**Key Functions:**
- `NewBootstrap(cfg)` - Create bootstrap instance
- `Initialize(params)` - Run full initialization
- `GetAgentID()` - Get agent ID
- `GetAgentSealKey()` - Get private key (internal only)
- `GetConfig()` - Get config manager

### Attestor Client

**Purpose:** Remote attestation and key delivery

**Endpoints:**
- `POST /attest` - Perform attestation, get token
- `GET /key?token=xxx` - Get agentSeal private key

**Key Functions:**
- `Attest(params)` - Get attestation token
- `GetAgentSealKey(token)` - Get private key (PEM)
- `BuildAttestURL()` - Build attest endpoint URL
- `BuildGetKeyURL()` - Build get key endpoint URL

### Storage Client

**Purpose:** Fetch encrypted configs from 0G Storage

**Endpoints:**
- `GET /file/{hash}` - Fetch file
- `GET /config/{hash}` - Fetch config

**Key Functions:**
- `FetchFile(hash)` - Fetch file by hash
- `FetchConfig(hash)` - Fetch config by hash
- `ValidateHash(hash)` - Validate hash format

### Config Manager

**Purpose:** Manage agent configuration

**Key Functions:**
- `FetchConfig(hash)` - Fetch and decrypt config
- `GetConfig()` - Get cached config
- `IsConfigLoaded()` - Check if loaded
- `ValidateConfig(cfg)` - Validate config schema

### Signer Module

**Purpose:** ECDSA cryptographic operations

**Key Functions:**
- `SignResponse(key, data)` - Sign response data
- `SignResponseWrapper(key, agentId, sealId, ts, data)` - Full signature
- `SignHeartbeat(key, sealId, timestamp)` - Sign heartbeat
- `VerifySignature(data, sig, pubKey)` - Verify signature
- `GetPublicKey(key)` - Get public key hex

### Proxy Handler (Future)

**Purpose:** HTTP proxy with response signing

**Flow:**
```
External Request → :8080 → Agent (:9000)
                      │
                      └─ Response
                          │
                          ├─ Sign(agentId, sealId, ts, data)
                          └─ Return with signature headers
```

### Heartbeat (Future)

**Purpose:** Periodic liveness signals

**Flow:**
```
Every 30s:
  1. Sign(sealId, timestamp)
  2. POST to Attestor /heartbeat
  3. Handle response
```

## Request/Response Flow

### External Request

```
Client → Wrapper :8080 → Agent :9000
        │              │
        │              └─ Process
        │                  │
        │                  └─ Response
        │                      │
        ├─ Sign(agentId, sealId, ts, response)
        │
        └─ SignedResponse {
              data: response,
              signature: ECDSA(...),
              agentId: ...,
              sealId: ...,
              timestamp: ...
            }
```

### Signature Format

```
content = agentId + "|" + sealId + "|" + timestamp + "|" + hash(data)
signature = ECDSA.Sign(agentSealKey, hash(content))
```

### Response Headers

```
X-Agent-Id: 0x1234567890abcdef...
X-Seal-Id: seal_abc123
X-Signature: 1a2b3c4d... (128 hex chars)
X-Timestamp: 1712787654
```

## Configuration Schema

```json
{
  "agentId": "0x1234...abcd",
  "entryPoint": "python main.py",
  "env": {
    "API_KEY": "sk-...",
    "MODEL": "gpt-4"
  },
  "resources": {
    "memoryMB": 2048,
    "cpu": 2
  }
}
```

## Security Model

### Trust Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│                        Untrusted                            │
│                  (External Internet)                        │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                     TEE Boundary (Gramine)                   │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐│
│  │  Wrapper (Trusted)                                    ││
│  │  - agentSeal key in memory only                       ││
│  │  - All signing in TEE                                ││
│  │  - Agent communication via localhost                 ││
│  └────────────────────────────────────────────────────────┘│
│                          │                                  │
│                          ▼                                  │
│  ┌────────────────────────────────────────────────────────┐│
│  │  Agent (Business Logic)                               ││
│  │  - No access to wrapper memory                        ││
│  │  - No access to private keys                         ││
│  │  - Normal HTTP server on :9000                        ││
│  └────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

### Key Isolation

- **agentSeal private key** - Only in Wrapper process memory
- **Agent process** - No access to Wrapper memory or keys
- **TEE protection** - Key material never leaves TEE

## Error Handling

| Error | Wrapper Action | Agent Impact |
|-------|---------------|-------------|
| Agent crash | Restart agent | Brief interruption |
| Network timeout | Retry request | Delayed response |
| Sign failure | Log error, continue | Unsigned response |
| Attestor down | Wait and retry | Delayed startup |

## Future Modules

- [ ] **Proxy Handler** - HTTP proxy with signing
- [ ] **Heartbeat** - Periodic liveness signals
- [ ] **Blockchain Client** - Event monitoring
- [ ] **Docker Client** - Agent container management

## Implementation Priority

1. ✅ **Signer** - Cryptographic operations
2. ✅ **Storage** - Config fetching
3. ✅ **Config** - Config management
4. ✅ **Attestor** - Remote attestation
5. ✅ **Bootstrap** - Orchestration
6. ⏳ **Proxy** - HTTP proxy (next)
7. ⏳ **Heartbeat** - Liveness signals
8. ⏳ **Blockchain** - Event monitoring
