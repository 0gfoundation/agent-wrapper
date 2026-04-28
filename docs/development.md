# Agent Wrapper Development Guide

## Getting Started

### Prerequisites

- Go 1.23 or later
- Docker 20+ (for local testing)
- Make

### Setting Up Development Environment

```bash
# Clone the repository
git clone https://github.com/0g-citizen-claw/agent-wrapper.git
cd agent-wrapper

# Install dependencies
go mod download

# Run tests
make test

# Build locally
make build
```

## Project Structure

```
agent-wrapper/
├── cmd/
│   └── wrapper/
│       └── main.go           # Entry point
├── internal/                 # Private packages
│   ├── attest/               # Attestor service client
│   ├── blockchain/           # Blockchain client
│   ├── config/               # Configuration management
│   ├── flow/                 # Initialization orchestration
│   ├── framework/            # Dynamic framework installation
│   ├── init/                 # HTTP initialization server
│   ├── mock/                 # Mock server for testing
│   ├── process/              # Agent process management
│   ├── proxy/                # HTTP proxy with signing
│   └── sealed/               # Sealed state management
├── pkg/                      # Public packages (currently empty)
├── docs/                     # Documentation
├── examples/                 # Example agents
│   └── demo-agent.py         # Demo agent for testing
├── Makefile                  # Build commands
├── Dockerfile               # Container build
├── docker-compose.yml       # Development compose
├── go.mod                   # Go module definition
└── README.md                # User documentation
```

## Module Details

### internal/init

HTTP server that receives initialization parameters.

**Files**:
- `server.go` - HTTP server implementation
- `server_test.go` - Unit tests

**Responsibilities**:
- Serve `/_internal/init` endpoint
- Serve `/_internal/health` endpoint
- Serve `/_internal/ready` endpoint
- Track initialization status

### internal/sealed

Thread-safe state management for sensitive data.

**Files**:
- `state.go` - State implementation
- `state_test.go` - Unit tests

**Responsibilities**:
- Store sealId, tempKey, attestorUrl
- Generate and store ECDSA key pair
- Store agentSeal private key
- Store agentId
- Provide thread-safe getters

### internal/attest

Client for Attestor service communication.

**Files**:
- `client.go` - Client implementation
- `client_test.go` - Unit tests with mock

**Responsibilities**:
- Perform remote attestation
- Retrieve agentSeal private key
- Handle authentication tokens

### internal/blockchain

Client for blockchain queries.

**Files**:
- `client.go` - Client implementation
- `client_test.go` - Unit tests with mock

**Responsibilities**:
- Query agentId by sealId
- Query IntelligentData array
- Parse blockchain responses

### internal/storage

Client for 0G Storage.

**Files**:
- `storage.go` - Client implementation
- `storage_test.go` - Unit tests with mock

**Responsibilities**:
- Fetch encrypted configuration
- Handle storage errors

### internal/config

Configuration management.

**Files**:
- `manager.go` - Config implementation
- `manager_test.go` - Unit tests

**Responsibilities**:
- Decrypt encrypted config
- Validate config structure
- Provide default config

### internal/framework

Dynamic framework installation.

**Files**:
- `installer.go` - Installer implementation
- `installer_test.go` - Unit tests

**Responsibilities**:
- Install Python packages (pip)
- Install Node.js packages (npm)
- Handle installation timeouts
- Report installation results

### internal/process

Agent process management.

**Files**:
- `manager.go` - Process manager
- `manager_test.go` - Unit tests

**Responsibilities**:
- Start agent process
- Stop agent process gracefully
- Monitor process health
- Auto-restart on crash

### internal/proxy

HTTP proxy with ECDSA signing.

**Files**:
- `proxy.go` - Proxy implementation
- `proxy_test.go` - Unit tests

**Responsibilities**:
- Proxy requests to agent
- Sign responses with ECDSA
- Add signature headers

### internal/flow

Initialization flow orchestration.

**Files**:
- `orchestrator.go` - Orchestrator implementation
- `orchestrator_test.go` - Unit tests
- `e2e_test.go` - End-to-end tests
- `demo_test.go` - Demo mode tests

**Responsibilities**:
- Coordinate all initialization steps
- Handle demo mode
- Manage state transitions

### internal/mock

HTTP mock server for testing.

**Files**:
- `mock.go` - Mock server
- `mock_test.go` - Mock tests

**Responsibilities**:
- Mock Attestor endpoints
- Mock Blockchain endpoints
- Mock Storage endpoints

## Development Workflow

### 1. Create a Feature Branch

```bash
git checkout -b feature/your-feature-name
```

### 2. Write Tests First (TDD)

Following Test-Driven Development:

```go
// internal/yourmodule/yourmodule_test.go
func TestYourFunction(t *testing.T) {
    t.Run("does something expected", func(t *testing.T) {
        // Arrange
        input := "test"

        // Act
        result := YourFunction(input)

        // Assert
        assert.Equal(t, "expected", result)
    })
}
```

### 3. Run Tests (Watch Them Fail)

```bash
go test ./internal/yourmodule/... -v
```

### 4. Implement Functionality

```go
// internal/yourmodule/yourmodule.go
func YourFunction(input string) string {
    return "expected"
}
```

### 5. Run Tests Again

```bash
go test ./internal/yourmodule/... -v
```

### 6. Commit and Push

```bash
git add .
git commit -m "feat: add your feature"
git push origin feature/your-feature-name
```

## Testing

### Unit Tests

```bash
# Run all unit tests
make test-unit

# Run specific package tests
go test ./internal/init/... -v

# Run with coverage
go test -cover ./internal/...

# Run with race detector
go test -race ./internal/...
```

### E2E Tests

```bash
# Run E2E tests (requires Docker)
make test-e2e

# Run specific E2E test
go test ./internal/flow/... -v -run E2E
```

### Demo Mode Tests

```bash
# Run demo mode tests
DEMO_MODE=true go test ./internal/flow/... -v -run Demo
```

## Building

### Local Build

```bash
# Build for current platform
make build

# Output: ./bin/wrapper
```

### Docker Build

```bash
# Build Docker image
make docker

# Run with demo mode
docker run -p 8080:8080 -e DEMO_MODE=true \
    0g-citizen-claw/agent-wrapper:latest
```

### Docker Compose

```bash
# Start all services
make compose-up

# View logs
docker-compose logs -f

# Stop services
make compose-down
```

## Common Patterns

### Creating a New Module

```bash
# Create directory
mkdir -p internal/newmodule

# Create test file
cat > internal/newmodule/newmodule_test.go << 'EOF'
package newmodule

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestNewFunction(t *testing.T) {
    t.Run("returns expected result", func(t *testing.T) {
        result := NewFunction()
        assert.Equal(t, "expected", result)
    })
}
EOF

# Create implementation file
cat > internal/newmodule/newmodule.go << 'EOF'
package newmodule

// NewFunction does something.
func NewFunction() string {
    return "expected"
}
EOF
```

### HTTP Client Creation

```go
type MyClient struct {
    baseURL    string
    httpClient *http.Client
}

func NewMyClient(baseURL string, timeout time.Duration) *MyClient {
    if timeout == 0 {
        timeout = 30 * time.Second
    }

    return &MyClient{
        baseURL: baseURL,
        httpClient: &http.Client{
            Timeout: timeout,
        },
    }
}
```

### Thread-Safe State

```go
type MyState struct {
    mu    sync.RWMutex
    value string
}

func (s *MyState) GetValue() string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.value
}

func (s *MyState) SetValue(value string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.value = value
}
```

## Debugging

### Local Debugging

```bash
# Run with Delve
dlv debug ./cmd/wrapper

# Run tests with Delve
dlv test ./internal/sealed/...
```

### Demo Mode

For quick testing without external services:

```bash
# Set demo mode
export DEMO_MODE=true
go run ./cmd/wrapper

# Or with Docker
docker run -p 8080:8080 -e DEMO_MODE=true \
    0g-citizen-claw/agent-wrapper:latest
```

### Logging

Set log level for debugging:

```bash
export LOG_LEVEL=debug
go run ./cmd/wrapper
```

## Code Style

### Formatting

```bash
# Format all Go files
go fmt ./...
```

### Linting

```bash
# Run golangci-lint (if installed)
golangci-lint run
```

## Release Process

### Version Bump

1. Update version in `cmd/wrapper/main.go`
2. Update documentation

```bash
# Build release
make build

# Build Docker image
make docker
```

## CI/CD

### Running CI Checks Locally

```bash
# Run all checks
make check
```

## Contributing

### Before Submitting a PR

1. **Run all tests**: `make test`
2. **Format code**: `go fmt ./...`
3. **Test demo mode**: `DEMO_MODE=true go test ./...`
4. **Update documentation**: If adding new features

### PR Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] E2E tests pass
- [ ] Demo mode tested

## Checklist
- [ ] Code follows project patterns
- [ ] All tests pass
- [ ] Documentation updated
```

## Resources

### Internal Documentation

- [README.md](../README.md) - Project overview
- [CLAUDE.md](../CLAUDE.md) - Claude Code instructions
- [architecture.md](./architecture.md) - Architecture diagrams
- [design.md](./design.md) - Design details
- [api.md](./api.md) - API documentation

### External Resources

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Testify Documentation](https://github.com/stretchr/testify)
