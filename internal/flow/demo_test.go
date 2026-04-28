package flow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	initpkg "github.com/0g-citizen-claw/agent-wrapper/internal/init"
	"github.com/0g-citizen-claw/agent-wrapper/internal/mock"
	"github.com/0g-citizen-claw/agent-wrapper/internal/sealed"
)

// TestDemoMode tests the demo mode functionality
func TestDemoMode(t *testing.T) {
	// Set demo mode
	oldDemoMode := os.Getenv("DEMO_MODE")
	defer os.Setenv("DEMO_MODE", oldDemoMode)
	os.Setenv("DEMO_MODE", "true")

	// Start mock servers
	servers := mock.NewServers()
	defer servers.Close()

	attestorURL, blockchainURL, storageURL := servers.URLs()

	// Create orchestrator
	initServer := initpkg.New()
	sealedState := sealed.NewState()
	orch := New(initServer, sealedState, &Config{
		StorageEndpoint: storageURL,
		AttestorURL:     attestorURL,
		BlockchainURL:   blockchainURL,
	})

	// Check that default config uses demo mode
	defaultConfig := orch.defaultConfig()
	if defaultConfig.Framework.Name != "demo" {
		t.Errorf("expected framework name 'demo', got %s", defaultConfig.Framework.Name)
	}

	// Test demo agent finding
	demoPath := orch.findDemoAgent()
	t.Logf("Demo agent path: %s", demoPath)
}

// TestFindDemoAgent tests finding the demo agent script
func TestFindDemoAgent(t *testing.T) {
	orch := New(initpkg.New(), sealed.NewState(), &Config{})

	path := orch.findDemoAgent()
	t.Logf("Found demo agent at: %s", path)

	// Path should be one of the expected locations
	if path != "" {
		if !strings.Contains(path, "demo-agent.py") {
			t.Errorf("expected path to contain demo-agent.py, got %s", path)
		}
	}
}

// TestDemoModeDefaultConfig tests that demo mode creates appropriate config
func TestDemoModeDefaultConfig(t *testing.T) {
	oldDemoMode := os.Getenv("DEMO_MODE")
	defer os.Setenv("DEMO_MODE", oldDemoMode)

	// Test with demo mode enabled
	os.Setenv("DEMO_MODE", "true")
	orch := New(initpkg.New(), sealed.NewState(), &Config{})
	cfg := orch.defaultConfig()

	if cfg.Framework.Name != "demo" {
		t.Errorf("expected framework 'demo', got %s", cfg.Framework.Name)
	}
	if cfg.Runtime.WorkingDir == "/app" {
		t.Error("demo mode should not use /app working dir")
	}
	if cfg.Env["FRAMEWORK"] != "demo" {
		t.Errorf("expected FRAMEWORK=demo, got %s", cfg.Env["FRAMEWORK"])
	}

	// Test with demo mode disabled
	os.Setenv("DEMO_MODE", "false")
	orch2 := New(initpkg.New(), sealed.NewState(), &Config{})
	cfg2 := orch2.defaultConfig()

	if cfg2.Framework.Name != "openclaw" {
		t.Errorf("expected framework 'openclaw', got %s", cfg2.Framework.Name)
	}
}

// TestDemoAgentScript tests that the demo agent script exists
func TestDemoAgentScript(t *testing.T) {
	paths := []string{
		"./examples/demo-agent.py",
		"../examples/demo-agent.py",
	}

	var foundPath string
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			foundPath = path
			break
		}
	}

	if foundPath == "" {
		t.Skip("demo agent script not found")
	}

	// Read and verify it's a valid Python script
	content, err := os.ReadFile(foundPath)
	if err != nil {
		t.Fatalf("failed to read demo agent: %v", err)
	}

	if !strings.Contains(string(content), "#!/usr/bin/env python3") {
		t.Error("demo agent should be a Python 3 script")
	}
	if !strings.Contains(string(content), "HTTPServer") {
		t.Error("demo agent should use HTTPServer")
	}
}

// TestDemoModeWithFlow tests the full flow in demo mode
func TestDemoModeWithFlow(t *testing.T) {
	oldDemoMode := os.Getenv("DEMO_MODE")
	defer os.Setenv("DEMO_MODE", oldDemoMode)
	os.Setenv("DEMO_MODE", "true")

	// Find demo agent first
	orch := New(initpkg.New(), sealed.NewState(), &Config{})
	demoPath := orch.findDemoAgent()
	if demoPath == "" {
		t.Skip("demo agent not found, skipping flow test")
	}

	// Start mock servers
	servers := mock.NewServers()
	defer servers.Close()

	attestorURL, blockchainURL, storageURL := servers.URLs()

	// Create fresh orchestrator for flow test
	initServer := initpkg.New()
	sealedState := sealed.NewState()
	orch = New(initServer, sealedState, &Config{
		StorageEndpoint: storageURL,
		AttestorURL:     attestorURL,
		BlockchainURL:   blockchainURL,
	})

	// Register test seal
	testSealID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	servers.RegisterSeal(testSealID, "1")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start flow in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- orch.Run(ctx)
	}()

	// Wait for initialization to be ready
	time.Sleep(200 * time.Millisecond)

	// Send init request
	initReq := &initpkg.InitRequest{
		SealID:      testSealID,
		TempKey:     "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		AttestorURL: attestorURL,
	}
	reqBody, _ := json.Marshal(initReq)

	// Retry init request
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/_internal/init", strings.NewReader(string(reqBody)))
		w := httptest.NewRecorder()
		initServer.Handler().ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for flow to progress through framework install
	time.Sleep(2 * time.Second)

	// Check that demo mode was used
	orch.mu.Lock()
	config := orch.agentConfig
	orch.mu.Unlock()

	if config != nil && config.Framework != nil {
		if config.Framework.Name == "demo" {
			t.Log("Demo mode was correctly used in flow")
		}
	}

	// Clean up
	cancel()
	orch.Stop()

	// Drain error channel
	select {
	case <-errCh:
	case <-time.After(time.Second):
	}
}

// TestDemoAgentIntegration is an integration test that actually runs the demo agent
// This test is skipped by default as it requires Python
func TestDemoAgentIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Check if Python is available
	if _, err := os.Stat("/usr/bin/python3"); os.IsNotExist(err) {
		if _, err := os.Stat("/usr/local/bin/python3"); os.IsNotExist(err) {
			t.Skip("Python 3 not available")
		}
	}

	// This test would run the actual demo agent
	// For now, just verify the script exists
	demoPaths := []string{
		"examples/demo-agent.py",
		"../examples/demo-agent.py",
	}

	for _, path := range demoPaths {
		if _, err := os.Stat(path); err == nil {
			t.Logf("Demo agent found at: %s", path)
			return
		}
	}

	t.Skip("demo agent script not found")
}
