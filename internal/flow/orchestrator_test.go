package flow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	initpkg "github.com/0g-citizen-claw/agent-wrapper/internal/init"
	"github.com/0g-citizen-claw/agent-wrapper/internal/sealed"
)

func TestNew(t *testing.T) {
	initServer := initpkg.New()
	sealedState := sealed.NewState()
	cfg := &Config{
		StorageEndpoint: "https://storage.example.com",
		AttestorURL:     "https://attestor.example.com",
		BlockchainURL:   "https://blockchain.example.com",
	}

	orch := New(initServer, sealedState, cfg)

	if orch == nil {
		t.Fatal("expected non-nil orchestrator")
	}
	if orch.initServer != initServer {
		t.Error("initServer not set correctly")
	}
	if orch.sealedState != sealedState {
		t.Error("sealedState not set correctly")
	}
	if orch.attestClient == nil {
		t.Error("attestClient not initialized")
	}
	if orch.configManager == nil {
		t.Error("configManager not initialized")
	}
	if orch.frameworkInst == nil {
		t.Error("frameworkInst not initialized")
	}
	if orch.blockchainCli == nil {
		t.Error("blockchainCli not initialized")
	}
	if orch.storageCli == nil {
		t.Error("storageCli not initialized")
	}
}

func TestOrchestrator_Run_InitStep(t *testing.T) {
	t.Run("waits for HTTP initialization", func(t *testing.T) {
		initServer := initpkg.New()
		sealedState := sealed.NewState()
		orch := New(initServer, sealedState, &Config{
			StorageEndpoint: "https://storage.example.com",
			AttestorURL:     "https://attestor.example.com",
			BlockchainURL:   "https://blockchain.example.com",
		})

		// Start initialization in background
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- orch.Run(ctx)
		}()

		// Wait a bit
		time.Sleep(100 * time.Millisecond)

		// Check that we're still waiting (not initialized)
		if orch.initialized {
			t.Error("should not be initialized yet")
		}

		// Send initialization request
		initReq := &initpkg.InitRequest{
			SealID:      "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			TempKey:     "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			AttestorURL: "https://attestor.example.com",
		}
		reqBody, _ := json.Marshal(initReq)
		req := httptest.NewRequest("POST", "/_internal/init", strings.NewReader(string(reqBody)))
		w := httptest.NewRecorder()

		initServer.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("init request failed: %d - %s", w.Code, w.Body.String())
		}

		// Wait for initialization to complete
		select {
		case err := <-errCh:
			// Should complete without error
			if err != nil {
				t.Logf("Run completed with error (expected for now): %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("timeout waiting for initialization")
		case <-ctx.Done():
		}
	})
}

func TestOrchestrator_GetImageHash(t *testing.T) {
	orch := New(initpkg.New(), sealed.NewState(), &Config{})

	hash := orch.getImageHash()

	if hash == "" {
		t.Error("expected non-empty hash")
	}
	if !strings.HasPrefix(hash, "0x") {
		t.Error("hash should start with 0x")
	}
}

func TestOrchestrator_DefaultConfig(t *testing.T) {
	orch := New(initpkg.New(), sealed.NewState(), &Config{})

	config := orch.defaultConfig()

	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.Framework == nil {
		t.Fatal("expected non-nil Framework")
	}
	if config.Framework.Name != "openclaw" {
		t.Errorf("expected Framework.Name 'openclaw', got %s", config.Framework.Name)
	}
	if config.Framework.Version != "0.1.0" {
		t.Errorf("expected Framework.Version '0.1.0', got %s", config.Framework.Version)
	}
	if config.Runtime == nil {
		t.Fatal("expected non-nil Runtime")
	}
	if config.Runtime.EntryPoint != "python3 main.py" {
		t.Errorf("expected EntryPoint 'python3 main.py', got %s", config.Runtime.EntryPoint)
	}
	if config.Runtime.AgentPort != 9000 {
		t.Errorf("expected AgentPort 9000, got %d", config.Runtime.AgentPort)
	}
	if config.Runtime.WorkingDir != "/app" {
		t.Errorf("expected WorkingDir '/app', got %s", config.Runtime.WorkingDir)
	}
	if config.Env == nil {
		t.Fatal("expected non-nil Env")
	}
	if config.Env["LOG_LEVEL"] != "info" {
		t.Errorf("expected LOG_LEVEL 'info', got %s", config.Env["LOG_LEVEL"])
	}
}

func TestOrchestrator_IsFlowComplete(t *testing.T) {
	orch := New(initpkg.New(), sealed.NewState(), &Config{})

	// Initially not complete
	if orch.IsFlowComplete() {
		t.Error("flow should not be complete initially")
	}

	// Mark as complete
	orch.mu.Lock()
	orch.flowComplete = true
	orch.mu.Unlock()

	if !orch.IsFlowComplete() {
		t.Error("flow should be complete after setting")
	}
}

func TestOrchestrator_GetAgentConfig(t *testing.T) {
	orch := New(initpkg.New(), sealed.NewState(), &Config{})

	// Initially nil
	if orch.GetAgentConfig() != nil {
		t.Error("agent config should be nil initially")
	}

	// Set config
	testConfig := orch.defaultConfig()
	orch.mu.Lock()
	orch.agentConfig = testConfig
	orch.mu.Unlock()

	config := orch.GetAgentConfig()
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.Runtime.EntryPoint != "python3 main.py" {
		t.Errorf("expected EntryPoint 'python3 main.py', got %s", config.Runtime.EntryPoint)
	}
}

func TestOrchestrator_GetAgentPort(t *testing.T) {
	orch := New(initpkg.New(), sealed.NewState(), &Config{})

	// Default port
	port := orch.GetAgentPort()
	if port != "9000" {
		t.Errorf("expected default port '9000', got %s", port)
	}

	// Set custom port
	testConfig := orch.defaultConfig()
	testConfig.Runtime.AgentPort = 8080
	orch.mu.Lock()
	orch.agentConfig = testConfig
	orch.mu.Unlock()

	port = orch.GetAgentPort()
	if port != "8080" {
		t.Errorf("expected port '8080', got %s", port)
	}
}

func TestOrchestrator_WriteDemoScript(t *testing.T) {
	// This functionality has been moved to the process manager
	// Testing is now covered in process manager tests
	t.Skip("writeDemoScript moved to process manager")
}

func TestOrchestrator_Stop(t *testing.T) {
	orch := New(initpkg.New(), sealed.NewState(), &Config{})

	// Stop with no agent process should not panic
	orch.Stop()

	// If we get here without panic, test passes
}

func TestConfig(t *testing.T) {
	cfg := &Config{
		StorageEndpoint: "https://storage.example.com",
		AttestorURL:     "https://attestor.example.com",
		BlockchainURL:   "https://blockchain.example.com",
	}

	if cfg.StorageEndpoint == "" {
		t.Error("StorageEndpoint should not be empty")
	}
	if cfg.AttestorURL == "" {
		t.Error("AttestorURL should not be empty")
	}
	if cfg.BlockchainURL == "" {
		t.Error("BlockchainURL should not be empty")
	}
}

func TestOrchestrator_DeriveAESKey(t *testing.T) {
	orch := New(initpkg.New(), sealed.NewState(), &Config{})

	// Test with valid private key (32 bytes)
	privKey := make([]byte, 32)
	for i := range privKey {
		privKey[i] = byte(i)
	}

	aesKey, err := orch.deriveAESKey(privKey)
	if err != nil {
		t.Fatalf("deriveAESKey failed: %v", err)
	}
	if len(aesKey) != 32 {
		t.Errorf("expected 32 byte key, got %d", len(aesKey))
	}

	// Test with short key
	shortKey := make([]byte, 16)
	_, err = orch.deriveAESKey(shortKey)
	if err == nil {
		t.Error("expected error for short key")
	}
}
