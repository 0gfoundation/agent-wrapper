# Agent Wrapper - Claude Instructions

This file contains project-specific instructions for Claude Code working on the Agent Wrapper module.

## Project Context

Agent Wrapper is an **independent Go module** that runs inside TEE (Trusted Execution Environment) to manage agent lifecycle for the 0G Citizen Claw infrastructure.

**Key characteristics:**
- Framework-agnostic agent wrapper
- Security-critical (handles private keys, TEE attestation)
- Self-contained single binary deployment
- Independent Git repository with own CI/CD

## Technology Stack

- **Language:** Go 1.21+
- **Cryptography:**
  - ECDSA with secp256k1 curve (go-ethereum/crypto) - for Ethereum compatibility
  - ECIES encryption/decryption (go-ethereum/crypto/ecies)
  - Keccak256 hashing (go-ethereum/crypto)
  - AES-256-GCM for data encryption
- **Blockchain:** go-ethereum for RPC and event scanning
- **Testing:** testify
- **Deployment:** Docker container, runs inside Gramine TEE

## Project Structure

```
agent-wrapper/
├── cmd/wrapper/          # Entry point (main.go)
├── internal/             # Private packages (no external imports)
│   ├── attest/          # Attestor service client (/provision, /status)
│   ├── blockchain/      # Blockchain client (metadata, ITransferred events)
│   ├── config/          # Configuration management (encryption/decryption)
│   ├── flow/            # Initialization flow orchestrator
│   ├── framework/       # Framework installation (OpenClaw, etc.)
│   ├── init/            # HTTP initialization server
│   ├── process/         # Agent process management
│   ├── proxy/           # HTTP proxy handler (A2A endpoints)
│   ├── sealed/          # Sealed state management (keys, status)
│   └── storage/         # 0G Storage client (config, file download)
├── pkg/                 # Public packages
│   └── types/           # Shared types
├── api/                 # API definitions
├── docs/                # Documentation
├── configs/             # Configuration schemas
└── test/                # Integration/E2E tests
```

## Development Workflow

### TDD is Mandatory

**All new code must follow Test-Driven Development:**
1. Write failing test first (RED)
2. Watch it fail for the expected reason
3. Write minimal code to pass (GREEN)
4. Refactor while keeping tests passing

**No production code without a failing test first.**

### Module Implementation Order

When implementing new features, follow this order:
1. **Tests first** - Write comprehensive tests in `*_test.go`
2. **Implementation** - Implement to pass tests
3. **Documentation** - Update relevant docs

### Running Tests

```bash
# Run all tests
go test ./...

# Run specific module tests with coverage
go test -cover ./internal/signer/...

# Run with verbose output
go test -v ./internal/...
```

## Coding Standards

### Go Conventions
- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Exported functions must have documentation comments
- Package comments at top of each file

### Error Handling
- Always wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Never ignore errors from external services
- Log errors at appropriate levels

### Security Considerations
- **Private keys never leave TEE memory** - No logging, no serialization
- Validate all external inputs
- Use constant-time comparison for sensitive data
- Zero out sensitive data after use

### Naming Conventions
- Interfaces: Simple names (`Storage`, `Attestor`)
- Implementations: Descriptive names (`StorageClient`, `AttestorClient`)
- Test functions: `Test<FunctionName>_<Scenario>`
- Test sub-tests: Use `t.Run()` for related cases

## Module Guidelines

### Attest Module (`internal/attest/`)
- Attestor service client for `/provision` and `/status` endpoints
- Uses secp256k1 ECDSA for signing (Keccak256 + V=27/28)
- ECIES decryption for agentSeal key and sealed keys
- Supports TEE key management for attestation requests

### Blockchain Module (`internal/blockchain/`)
- Queries agent metadata from blockchain service
- Scans ITransferred events for sealed keys
- Two-phase scan strategy: head window first, then backward chunked scan
- Returns intelligent data list and sealed key mappings

### Config Module (`internal/config/`)
- AES-256-GCM encryption/decryption for configurations
- Validates configuration schema
- Returns structured `AgentConfig` with framework, runtime, inference settings

### Flow Module (`internal/flow/`)
- Orchestrates initialization flow
- Coordinates Attest, Blockchain, Storage, and Config modules
- Handles intelligent data processing (download + decrypt)
- Reports status to Attestor after bootstrap

### Storage Module (`internal/storage/`)
- 0G Storage integration
- `FetchConfig()` - retrieves encrypted agent config
- `DownloadFile()` - downloads files by hash (for intelligent data)
- Validates hash format (hexadecimal only)

### Sealed Module (`internal/sealed/`)
- Thread-safe state management for wrapper lifecycle
- Stores TEE key pair, agentSeal key, agent ID
- Status tracking with valid transitions
- Signing with agentSeal key (secp256k1)

### Proxy Module (`internal/proxy/`)
- HTTP proxy for agent requests
- Signs responses using agentSeal key
- A2A (agent-to-agent) endpoint support

## Common Patterns

### HTTP Client Creation

```go
timeout := cfg.Timeout
if timeout == 0 {
    timeout = 30 * time.Second
}

client := &http.Client{
    Timeout: timeout,
    Transport: &http.Transport{
        Proxy: nil, // Disable proxy to avoid connection issues
    },
}
```

### Validation Pattern

```go
func ValidateX(x *X) error {
    if x.Field == "" {
        return errors.New("field is required")
    }
    return nil
}
```

### Thread-Safe State

```go
type Example struct {
    mu    sync.RWMutex
    value string
}

func (e *Example) GetValue() string {
    e.mu.RLock()
    defer e.mu.RUnlock()
    return e.value
}
```

### Cryptographic Patterns

**Signing with Keccak256 + secp256k1 (Ethereum-compatible):**
```go
import (
    ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

// Create message
message := fmt.Sprintf("StatusReport:0x%s:%s:%s", sealID, status, errorDetail)

// Keccak256 hash
hash := ethcrypto.Keccak256([]byte(message))

// Sign with secp256k1 private key
signature, err := ethcrypto.Sign(hash, privKey)

// Normalize V to 27/28 (Ethereum ecrecover convention)
signature[64] += 27

// Encode as hex string
sigHex := "0x" + hex.EncodeToString(signature)
```

**ECIES Decryption (secp256k1):**
```go
import (
    ethecies "github.com/ethereum/go-ethereum/crypto/ecies"
)

// Convert ECDSA to ECIES private key
eciesPrivKey := ethecies.ImportECDSA(privKey)

// Decrypt sealed key -> data key
plaintext, err := eciesPrivKey.Decrypt(ciphertext, nil, nil)
```

## Initialization Flow

The agent wrapper follows this initialization sequence:

1. **HTTP Init** - Receive sealID, tempKey, attestorURL via POST /init
2. **Key Generation** - Generate secp256k1 key pair for signing
3. **Provision** - Call Attestor `/provision` to get encrypted agentSeal key
4. **Decrypt** - ECIES decrypt agentSeal key using TEE private key
5. **Query Agent** - Get agentID from blockchain by sealID
6. **Get Sealed Keys** - Scan ITransferred events for sealed keys
7. **Process Data** - Download and decrypt intelligent data
8. **Install Framework** - Install agent framework (OpenClaw, etc.)
9. **Start Agent** - Launch agent process
10. **Report Status** - Send "ready" status to Attestor

### Intelligent Data Decryption Flow

```
ITransferred Event → sealedKey (per dataHash)
        ↓
Download encrypted file from Storage (by dataHash)
        ↓
ECIES decrypt: sealedKey + agentSealPriv → dataKey (32 bytes)
        ↓
AES-GCM decrypt: encryptedFile + dataKey → plaintext config
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DEMO_MODE` | Set to "true" for demo mode (skips external calls) |
| `IMAGE_HASH` | Container image hash (set by TEE runtime) |
| `RPC_URL` | Ethereum RPC URL for ITransferred event scanning |
| `CONTRACT_ADDR` | AgenticID contract address |
| `AGENTIC_ID_CONTRACT` | Alternative env var for contract address |

## Testing Patterns

### Table-Driven Tests

```go
func TestValidateInput(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid input", "valid", false},
        {"empty input", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateInput(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateInput() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Skipping Integration Tests

```go
func TestIntegration(t *testing.T) {
    t.Skip("integration test - requires actual endpoint")
}
```

## Dependencies

**Current:**
- `github.com/ethereum/go-ethereum` - Ethereum cryptography (secp256k1, ECIES, Keccak256) and RPC client
- `github.com/stretchr/testify` - Testing assertions

**When adding dependencies:**
1. Check if standard library suffices first
2. Prefer stable, well-maintained packages
3. Run `go mod tidy` after adding
4. Update `go.sum` in commit

## Build and Release

```bash
# Build locally
make build

# Build for Linux deployment
make build-linux

# Build Docker image
make docker

# Run all checks
make check
```

## CI/CD

- GitHub Actions workflow: `.github/workflows/ci.yml`
- Runs on: push, pull_request
- Checks: Go tests, formatting, linting

## Common Tasks

### Adding a New Module

1. Create directory under `internal/`
2. Write tests first (`*_test.go`)
3. Implement functionality
4. Add to this document if it establishes new patterns

### Updating Dependencies

```bash
go get -u ./...
go mod tidy
```

### Debugging Tests

```bash
# Verbose with count
go test -v -count=1 ./internal/module/

# With race detector
go test -race ./internal/module/
```

## Important Notes

- **This is an independent project** - Changes should not reference parent monorepo
- **TEE environment** - Code runs inside Gramine with restricted system calls
- **Security first** - Always consider security implications
- **Keep it simple** - Prefer straightforward solutions over abstractions

## Related Documentation

- [README.md](./README.md) - User-facing documentation
- [docs/design.md](./docs/design.md) - Architecture and design
- [docs/api.md](./docs/api.md) - API documentation
- [docs/development.md](./docs/development.md) - Development guide
