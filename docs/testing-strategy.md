# Agent Wrapper Testing Strategy

## Test Pyramid

```
┌─────────────────────────────────────────────────────────────┐
│                        E2E Tests (5%)                        │
│  ─────────────────────────────────────────────────────────   │
│  Complete flow testing with real environment                │
│  - Full startup flow                                         │
│  - Mock server integration                                  │
├─────────────────────────────────────────────────────────────┤
│  Integration Tests (15%)                                     │
│  ─────────────────────────────────────────────────────────   │
│  Multi-module collaboration testing                          │
│  - HTTP Init + Orchestrator                                  │
│  - Proxy + Sealed State                                      │
├─────────────────────────────────────────────────────────────┤
│  Unit Tests (80%)                                            │
│  ─────────────────────────────────────────────────────────   │
│  Single function/method testing with external dependencies   │
│  fully mocked                                                │
└─────────────────────────────────────────────────────────────┘
```

## Module Test Coverage

| Module | Unit | Integration | E2E | Coverage |
|--------|------|-------------|-----|----------|
| `internal/init` | ✅ | ✅ | ✅ | 100% |
| `internal/sealed` | ✅ | ✅ | ✅ | 100% |
| `internal/attest` | ✅ (Mock) | ✅ | - | 100% |
| `internal/blockchain` | ✅ (Mock) | ✅ | - | 100% |
| `internal/storage` | ✅ (Mock) | ✅ | - | 100% |
| `internal/config` | ✅ | ✅ | - | 100% |
| `internal/framework` | ✅ | ✅ | - | 100% |
| `internal/process` | ✅ | ✅ | - | 100% |
| `internal/proxy` | ✅ | ✅ | ✅ | 95% |
| `internal/flow` | ✅ | ✅ | ✅ | 95% |
| `internal/mock` | ✅ | ✅ | - | 100% |

## Running Tests

### All Tests

```bash
make test
```

### Unit Tests Only

```bash
make test-unit
# or
go test ./internal/... -short
```

### Integration Tests

```bash
make test-integration
# or
go test ./internal/...
```

### E2E Tests

```bash
make test-e2e
# or
go test ./internal/flow/... -run E2E -v
```

### Demo Mode Tests

```bash
DEMO_MODE=true go test ./internal/flow/... -run Demo -v
```

### Coverage Report

```bash
make coverage
# or
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out -o coverage/index.html
open coverage/index.html
```

### Specific Module

```bash
# Run specific module tests
go test -v ./internal/init/...
go test -v -cover ./internal/framework/...

# Run specific test
go test -v ./internal/sealed/... -run TestGenerateKeyPair
```

## Test Organization

### Unit Tests

Located next to implementation files: `*_test.go`

```go
// internal/sealed/state_test.go
func TestGenerateKeyPair(t *testing.T) {
    t.Run("generates valid key pair", func(t *testing.T) {
        pubKey, privKey, err := sealed.GenerateKeyPair()

        assert.NoError(t, err)
        assert.Len(t, pubKey, 65) // Uncompressed P-256
        assert.Len(t, privKey, 32)
    })
}
```

### Integration Tests

Test multiple modules working together:

```go
// internal/flow/integration_test.go
func TestFlowInitialization(t *testing.T) {
    // Setup mock server
    mock := mock.NewServer()
    defer mock.Close()

    // Create orchestrator with mock URLs
    orch := flow.New(/* ... */)

    // Run flow
    err := orch.Run(context.Background())

    assert.NoError(t, err)
    assert.True(t, orch.IsFlowComplete())
}
```

### E2E Tests

Complete startup flow testing:

```go
// internal/flow/e2e_test.go
func TestE2E(t *testing.T) {
    if testing.Short() {
        t.Skip("E2E test")
    }

    // Full flow test with mock server
}
```

### Demo Mode Tests

Test demo mode specifically:

```go
// internal/flow/demo_test.go
func TestDemoMode(t *testing.T) {
    t.Setenv("DEMO_MODE", "true")

    // Run flow
    err := orch.Run(context.Background())

    assert.NoError(t, err)
    assert.True(t, orch.IsFlowComplete())
}
```

## Mock Server

The mock server (`internal/mock/`) provides HTTP endpoints for testing:

### Endpoints

| Path | Method | Response |
|------|--------|----------|
| `/mock/attest` | POST | `{token: "test-token"}` |
| `/mock/key` | GET | `{privateKey: "mock-key"}` |
| `/mock/agents/by-seal-id/{sealId}` | GET | `{agentId: "mock-agent-id"}` |
| `/mock/agents/{agentId}/intelligent-datas` | GET | `[{dataHash: "mock-hash"}]` |
| `/mock/config/{hash}` | GET | Encrypted bytes |

### Using Mock Server

```go
// Start mock server
mockServer := mock.NewServer()
defer mockServer.Close()

// Create clients with mock URL
client := attest.NewClient(&attest.Config{
    BaseURL: mockServer.URL(),
})
```

## Test Data

Test fixtures are located in the code:

```go
// Test data in test files
const testSealID = "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
const testTempKey = "0xabcd1234567890abcdef"
```

## Test Output

```
=== RUN   TestInitHandler_ValidRequest
--- PASS: TestInitHandler_ValidRequest (0.02s)
=== RUN   TestGenerateKeyPair
--- PASS: TestGenerateKeyPair (0.03s)
=== RUN   TestAttestClient_GetAgentSealKey
--- PASS: TestAttestClient_GetAgentSealKey (0.05s)
=== RUN   TestE2E_FullFlow
--- PASS: TestE2E_FullFlow (2.50s)
PASS
coverage: 85.3% of statements
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'
      - run: make test
      - run: make coverage
```

## Writing New Tests

### Template

```go
package yourmodule

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestYourFunction(t *testing.T) {
    t.Run("does something expected", func(t *testing.T) {
        // Arrange
        input := "test"

        // Act
        result := YourFunction(input)

        // Assert
        assert.Equal(t, "expected", result)
    })

    t.Run("handles error case", func(t *testing.T) {
        // Arrange
        input := ""

        // Act
        result, err := YourFunction(input)

        // Assert
        require.Error(t, err)
        assert.Empty(t, result)
    })
}
```

## Common Patterns

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
        {"too short", "ab", true},
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

### Setup/Teardown

```go
func TestWithSetup(t *testing.T) {
    // Setup
    server := NewTestServer()
    defer server.Close()

    // Test
    client := NewClient(server.URL)
    result, err := client.DoSomething()

    // Assert
    require.NoError(t, err)
    assert.NotNil(t, result)
}
```

### Environment Variables

```go
func TestWithEnv(t *testing.T) {
    // Save original value
    old := os.Getenv("TEST_VAR")
    defer os.Setenv("TEST_VAR", old)

    // Set test value
    t.Setenv("TEST_VAR", "test-value")

    // Test
    result := GetConfig()
    assert.Equal(t, "test-value", result)
}
```

## Debugging Tests

### Verbose Output

```bash
go test -v ./internal/...
```

### Specific Test

```bash
go test -v ./internal/sealed/... -run TestGenerateKeyPair
```

### With Race Detector

```bash
go test -race ./internal/...
```

### Stop on First Failure

```bash
go test -failfast ./internal/...
```

## Resources

- [Testify Assertions](https://github.com/stretchr/testify/blob/master/assert/assertions.go)
- [Go Testing Guide](https://go.dev/doc/tutorial/add-a-test)
- [Table Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
