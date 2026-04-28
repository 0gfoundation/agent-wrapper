# Agent Wrapper Documentation

Welcome to the Agent Wrapper documentation for the 0G Citizen Claw infrastructure.

## Quick Links

| Document | Description |
|----------|-------------|
| [Architecture](./architecture.md) | Complete architecture with Mermaid diagrams |
| [Design](./design.md) | Detailed design and implementation status |
| [API Reference](./api.md) | HTTP endpoints and usage |
| [User Guide](./usage.md) | Deployment and usage instructions |
| [Development Guide](./development.md) | Contributing and workflows |
| [Testing](./testing-strategy.md) | Testing approach and guidelines |

## Project Overview

Agent Wrapper is a **containerized service** that runs inside TEE (Trusted Execution Environment) to manage agent lifecycle for the 0G Citizen Claw infrastructure.

### Key Features

- **Containerized Deployment** - Pre-built Docker image, ready to use
- **Framework Agnostic** - Support any agent framework through dynamic installation
- **Security First** - Private keys never leave TEE memory
- **HTTP-based Control** - Simple REST API for initialization
- **Observable** - Clear logging and health endpoints

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    0g-sandbox (TEE Environment)                  │
│                                                                   │
│  Official Pre-built Image                                         │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                    Agent Wrapper (Go)                        ││
│  │                                                              ││
│  │  ┌────────────┐  ┌─────────────┐  ┌─────────────┐          ││
│  │  │ HTTP Init  │  │ Framework   │  │ Proxy       │          ││
│  │  │ & Control  │  │ Manager     │  │ Handler     │          ││
│  │  └────────────┘  └─────────────┘  └─────────────┘          ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

## Module Structure

```
internal/
├── attest/       # Attestor service client
├── blockchain/   # Blockchain client (agentId, intelligentDatas)
├── config/       # Configuration encryption/decryption
├── flow/         # Initialization orchestration
├── framework/    # Dynamic framework installation
├── init/         # HTTP initialization server
├── mock/         # Mock server for testing
├── process/      # Agent process management
├── proxy/        # HTTP proxy with signing
└── sealed/       # Sealed state management
```

## On-Chain vs Off-Chain Data

### On-Chain (Blockchain - ERC-7857 + ERC-8004 + Agentic)

| Data | Type | Description |
|------|------|-------------|
| `agentId` | uint256 | ERC-8004 Agentic Token ID |
| `sealId` | bytes32 | ERC-7857 Seal ID |
| `agentSeal` | address | Agent Seal contract address |
| `intelligentDatas[]` | array | Array of IntelligentData |
| `dataHash` | bytes32 | Hash of encrypted config in Storage |

### Off-Chain (0G Storage - Encrypted)

| Data | Description |
|------|-------------|
| `framework.name` | Framework name (openclaw/demo/eliza) |
| `framework.version` | Framework version |
| `runtime.entryPoint` | Command to start agent |
| `runtime.workingDir` | Working directory |
| `runtime.agentPort` | Agent port |
| `env` | Environment variables |

### Runtime (TEE Internal)

| Data | Description |
|------|-------------|
| `imageHash` | Container image hash (from environment) |
| `agentSealKey` | Private key for signing (never leaves TEE) |

## Getting Started

### Prerequisites

- Docker 20+ (for containerized deployment)
- Access to 0g-sandbox TEE environment
- Agent registered on-chain

### Quick Start

**1. Pull the official image**

```bash
docker pull 0g-citizen-claw/agent-wrapper:latest
```

**2. Start in 0g-sandbox**

```bash
sandbox start agent-wrapper:latest
```

**3. Initialize via HTTP**

```bash
curl -X POST http://localhost:8080/_internal/init \
  -H "Content-Type: application/json" \
  -d '{
    "sealId": "0x1234...abcd",
    "tempKey": "0xabcd...",
    "attestorUrl": "https://attestor.example.com"
  }'
```

## Startup Sequence

1. **Container Start** - Start official pre-built image
2. **HTTP Initialization** - Provide parameters via `/_internal/init`
3. **Sealed State** - Enter sealed state, generate key pair
4. **Remote Attestation** - Execute remote attestation (skip in demo mode)
5. **Get Key** - Get agentSeal private key from Attestor
6. **Query AgentID** - Query agentId by sealId from blockchain (skip in demo mode)
7. **Fetch IntelligentData** - Get intelligent data array (skip in demo mode)
8. **Fetch Config** - Pull encrypted config from 0G Storage (skip in demo mode)
9. **Decrypt Config** - Decrypt config using agentSealKey
10. **Install Framework** - Dynamically install agent framework
11. **Start Agent** - Start agent process
12. **Ready** - Ready to serve requests

## API Endpoints

### Internal Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/_internal/init` | Initialize wrapper with startup parameters |
| GET | `/_internal/health` | Health check |
| GET | `/_internal/ready` | Ready check (agent is running) |

### Proxy Endpoints

| Method | Path | Description |
|--------|------|-------------|
| ANY | `/*` | Proxy to agent with automatic signing |

## Dynamic Framework Support

Wrapper supports dynamic installation of the following frameworks:

| Framework | Language | Installation Method |
|-----------|----------|---------------------|
| OpenClaw | Python | `pip install openclaw=={version}` |
| Eliza | Node.js | `npm install @eliza/core@{version}` |
| Custom | Any | Custom installation script |

## Demo Mode

For development and testing, Agent Wrapper supports a demo mode that skips external service calls:

```bash
docker run -e DEMO_MODE=true 0g-citizen-claw/agent-wrapper:latest
```

In demo mode:
- Remote attestation is skipped
- Mock agentId is generated
- Default configuration is used
- Built-in demo agent is started

## Testing

```bash
# Run all tests
make test

# Run unit tests
make test-unit

# Run integration tests
make test-integration

# Run E2E tests
make test-e2e
```

## Additional Resources

- [Root README](../README.md) - Project overview
- [CLAUDE.md](../CLAUDE.md) - Claude Code instructions
- [examples/](../examples/) - Example agents and scripts

## License

MIT
