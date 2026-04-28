# Agent Wrapper Design Document

## Overview

Agent Wrapper is a **containerized service** that runs inside TEE (Trusted Execution Environment) to manage agent lifecycle for the 0G Citizen Claw infrastructure.

## Deployment Model

```
┌─────────────────────────────────────────────────────────────┐
│                    0g-sandbox (TEE Environment)              │
│                                                              │
│  Official pre-built image → Start instance                   │
│  Startup parameters via HTTP interface                       │
│                         │                                    │
│                         ▼                                    │
│  ┌─────────────────────────────────────────────────────────┐│
│  │              Container Instance                         ││
│  │                                                          ││
│  │  ENTRYPOINT: /usr/local/bin/wrapper                      ││
│  │                                                          ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │         Agent Wrapper (Go)                         │││
│  │  │                                                     │││
│  │  │  1. HTTP service listening on :8080                │││
│  │  │  2. Wait for POST /_internal/init                  │││
│  │  │  3. Enter sealed state, generate key pair          │││
│  │  │  4. Remote attestation → get agentSeal private key  │││
│  │  │  5. Query blockchain: sealId → agentId             │││
│  │  │  6. Fetch IntelligentData[] from blockchain        │││
│  │  │  7. Fetch encrypted config from 0G Storage         │││
│  │  │  8. Decrypt config using agentSealKey              │││
│  │  │  9. Dynamically install framework                  │││
│  │  │  10. Start HTTP proxy → :8080                      │││
│  │  │  11. Forward requests → Agent (:9000)              │││
│  │  │  12. Sign responses → ECDSA(agentSealKey)          │││
│  │  └────────────────────────────────────────────────────┘││
│  │                                    │                    ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │         Agent Framework (dynamically installed)    │││
│  │  │                                                     │││
│  │  │  HTTP Server :9000                                 │││
│  │  │  - OpenClaw / Eliza / Custom framework             │││
│  │  │  - Installed per blockchain metadata               │││
│  │  └────────────────────────────────────────────────────┘││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## Module Design

### internal/init - HTTP Initialization Server

**Purpose**: Receive initialization parameters via HTTP POST

**Endpoints**:
- `POST /_internal/init` - Receive {sealId, tempKey, attestorUrl}
- `GET /_internal/health` - Health check
- `GET /_internal/ready` - Ready check

**State**:
```go
type InitState struct {
    SealID      string
    TempKey     string
    AttestorURL string
}

type Server struct {
    mu          sync.RWMutex
    initialized bool
    state       InitState
    status      string
}
```

### internal/sealed - Sealed State Management

**Purpose**: Thread-safe storage for sensitive data

**State**:
```go
type State struct {
    mu            sync.RWMutex
    sealID        string
    tempKey       string
    publicKey     []byte
    privateKey    []byte
    agentSealKey  []byte
    agentID       string
}
```

**Methods**:
- `GenerateKeyPair()` - Generate ECDSA P-256 key pair
- `Initialize(sealID, tempKey, attestorURL)` - Initialize sealed state
- `SetAgentSealKey(key)` - Store agentSeal private key
- `SetAgentID(id)` - Store agent ID

### internal/attest - Attestor Service Client

**Purpose**: Remote attestation with Attestor service

**API**:
```go
type Client struct {
    baseURL    string
    httpClient *http.Client
    tempKey    string
}

func (c *Client) GetAgentSealKey(sealID, pubKey, imageHash string) ([]byte, string, error)
```

**Attestor Endpoints**:
- `POST /attest` - Perform attestation, get token
- `GET /key` - Get agentSeal private key using token

### internal/blockchain - Blockchain Client

**Purpose**: Query blockchain for agent metadata

**API**:
```go
type Client struct {
    endpoint   string
    httpClient *http.Client
}

type IntelligentData struct {
    DataDescription string
    DataHash        string
}

func (c *Client) GetAgentIdBySealId(sealId string) (string, error)
func (c *Client) GetIntelligentDatas(agentId string) ([]IntelligentData, error)
```

**Blockchain Endpoints**:
- `GET /agents/by-seal-id/{sealId}` - Get agentId
- `GET /agents/{agentId}/intelligent-datas` - Get IntelligentData array

### internal/storage - 0G Storage Client

**Purpose**: Fetch encrypted configuration

**API**:
```go
type Client struct {
    endpoint   string
    httpClient *http.Client
}

func (c *Client) FetchConfig(hash string) ([]byte, error)
```

### internal/config - Configuration Manager

**Purpose**: Decrypt and validate agent configuration

**AgentConfig Structure**:
```go
type AgentConfig struct {
    Framework *Framework `json:"framework"`
    Runtime   *Runtime   `json:"runtime"`
    Env       map[string]string `json:"env"`
}

type Framework struct {
    Name    string `json:"name"`
    Version string `json:"version"`
}

type Runtime struct {
    EntryPoint  string `json:"entryPoint"`
    WorkingDir  string `json:"workingDir"`
    AgentPort   int    `json:"agentPort"`
}
```

### internal/framework - Framework Installer

**Purpose**: Dynamically install agent frameworks

**API**:
```go
type Installer struct {
    pythonCmd  string
    npmCmd     string
    timeout    time.Duration
}

type InstallResult struct {
    Success  bool
    Duration int64
    Error    string
}

func (i *Installer) Install(ctx context.Context, name, version string) (*InstallResult, error)
```

**Supported Frameworks**:
| Name | Type | Install Command |
|------|------|-----------------|
| openclaw | Python | `pip install openclaw=={version}` |
| eliza | Node.js | `npm install @eliza/core@{version}` |
| demo | Skip | No installation |

### internal/process - Process Manager

**Purpose**: Start/stop agent process with monitoring

**API**:
```go
type Manager struct {
    mu      sync.RWMutex
    cmd     *exec.Cmd
    running bool
}

func (m *Manager) Start(ctx context.Context, config *config.AgentConfig) error
func (m *Manager) Stop()
func (m *Manager) IsRunning() bool
```

**Features**:
- Start agent with configured entry point
- Inject environment variables
- Capture stdout/stderr
- Auto-restart on crash (configurable)

### internal/proxy - HTTP Proxy Handler

**Purpose**: Proxy requests to agent with ECDSA signing

**API**:
```go
type Proxy struct {
    orchestrator StatusProvider
    sealedState  *sealed.State
}

func (p *Proxy) Handler() http.Handler
```

**Signature Format**:
```
signature = Sign(
    agentId + "|" +
    sealId + "|" +
    timestamp + "|" +
    hash(responseBody)
)
```

**Response Headers**:
- `X-Agent-Id` - Agent identifier
- `X-Seal-Id` - Seal identifier
- `X-Signature` - ECDSA signature (hex)
- `X-Timestamp` - Signature timestamp (Unix)

### internal/flow - Orchestrator

**Purpose**: Coordinate initialization flow

**Flow Sequence**:
```
1. Wait for HTTP init → Get {sealId, tempKey, attestorUrl}
2. Generate key pair → Initialize sealed state
3. Remote attestation → Get agentSealKey (skip in demo mode)
4. Query blockchain → Get agentId (skip in demo mode)
5. Fetch IntelligentData[] → Get configHash (skip in demo mode)
6. Fetch encrypted config from Storage (skip in demo mode)
7. Decrypt config using agentSealKey
8. Validate config → Use defaults if invalid
9. Install framework (skip if demo)
10. Start agent process
11. Set status = ready
```

### internal/mock - Mock Server

**Purpose**: HTTP mock server for testing

**Endpoints**:
- `POST /mock/attest` - Mock attestation
- `GET /mock/key` - Mock key endpoint
- `GET /mock/agents/by-seal-id/{sealId}` - Mock agentId query
- `GET /mock/agents/{agentId}/intelligent-datas` - Mock IntelligentData
- `GET /mock/config/{hash}` - Mock config fetch

## Contract Data Structure

### AgenticID Contract (ERC-7857 + ERC-8004 + Agentic)

```solidity
// On-chain data
struct AgentMetadata {
    uint256 agentId;              // ERC-8004 token ID
    bytes32 sealId;               // ERC-7857 Seal ID
    address agentSeal;            // Agent Seal contract
    IntelligentData[] intelligentDatas;
}

struct IntelligentData {
    string dataDescription;       // Description
    bytes32 dataHash;             // Storage hash
}
```

### Query Flow

```
sealId
  ↓
getAgentIdBySealId(sealId) → agentId (uint256)
  ↓
intelligentDatasOf(agentId) → IntelligentData[]
  ↓
IntelligentData[0].dataHash → Storage Key
  ↓
Storage.Get(dataHash) → Encrypted Config
  ↓
Decrypt(agentSealPrivateKey) → Agent Config
```

### What's NOT On-Chain

| Field | Location | Reason |
|-------|----------|--------|
| `framework` | Encrypted config | Sensitive, updatable |
| `version` | Encrypted config | Avoid gas costs |
| `entryPoint` | Encrypted config | Runtime parameter |
| `agentPort` | Encrypted config | Configurable |
| `env` | Encrypted config | API keys, secrets |

## Configuration

### HTTP Initialization

**POST** `/_internal/init`

```json
{
  "sealId": "0x1234...abcd",
  "tempKey": "0xabcd...",
  "attestorUrl": "https://attestor.example.com"
}
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `:8080` | HTTP listen port |
| `DEMO_MODE` | `false` | Skip external calls |
| `STORAGE_ENDPOINT` | `https://storage.0g.io` | Storage URL |
| `ATTESTOR_URL` | `https://attestor.0g.io` | Attestor URL |
| `BLOCKCHAIN_URL` | `https://blockchain.0g.io` | Blockchain URL |

## Error Handling

| Error Type | Wrapper Action | Agent Aware? |
|------------|----------------|--------------|
| Agent crash | Auto-restart | ✗ No |
| Framework install fail | Log warning, continue | ✗ No |
| Network timeout | Retry (3x) | ✗ No |
| Sign failure | Log error, continue | ✗ No |
| Attestor unavailable | Wait/retry | ✗ No |

## Performance Targets

| Metric | Target |
|--------|--------|
| Cold start | < 60s |
| Framework install | < 30s |
| Proxy latency | < 10ms |
| Sign time | < 5ms |
| Memory | < 100MB (Wrapper) |
| Agent start | < 5s |

## Implementation Status

### Completed Modules

| Module | Status | Coverage |
|--------|--------|----------|
| `internal/init` | ✅ Complete | 100% |
| `internal/sealed` | ✅ Complete | 100% |
| `internal/attest` | ✅ Complete + Mock | 100% |
| `internal/blockchain` | ✅ Complete + Mock | 100% |
| `internal/storage` | ✅ Complete + Mock | 100% |
| `internal/config` | ✅ Complete | 100% |
| `internal/framework` | ✅ Complete | 100% |
| `internal/process` | ✅ Complete | 100% |
| `internal/proxy` | ✅ Complete | 95% |
| `internal/flow` | ✅ Complete | 95% |
| `internal/mock` | ✅ Complete | 100% |

### Pending Features

| Feature | Priority |
|---------|----------|
| Heartbeat mechanism | Medium |
| Config hot-reload | Low |
| Metrics/observability | Medium |
| Private key memory protection | High |

## Version

Current version: **0.2.0**
