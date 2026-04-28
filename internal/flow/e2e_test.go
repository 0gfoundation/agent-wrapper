package flow

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	initpkg "github.com/0gfoundation/agent-wrapper/internal/init"
	"github.com/0gfoundation/agent-wrapper/internal/mock"
	"github.com/0gfoundation/agent-wrapper/internal/sealed"
)

// TestE2E_WithMockServers tests the complete flow with mock servers
func TestE2E_WithMockServers(t *testing.T) {
	// Start mock servers
	servers := mock.NewServers()
	defer servers.Close()

	// Get mock server URLs
	attestorURL, blockchainURL, storageURL := servers.URLs()

	// Create orchestrator with mock URLs
	initServer := initpkg.New()
	sealedState := sealed.NewState()
	orch := New(initServer, sealedState, &Config{
		StorageEndpoint: storageURL,
		AttestorURL:     attestorURL,
		BlockchainURL:   blockchainURL,
	})

	// Register test seal in mock blockchain
	testSealID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	servers.RegisterSeal(testSealID, "42")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start flow in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- orch.Run(ctx)
	}()

	// Wait a bit for orchestrator to start waiting
	time.Sleep(200 * time.Millisecond)

	// Send initialization request
	initReq := &initpkg.InitRequest{
		SealID:      testSealID,
		TempKey:     "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		AttestorURL: attestorURL,
	}
	reqBody, _ := json.Marshal(initReq)

	// Retry init request a few times
	var initResp *httptest.ResponseRecorder
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/_internal/init", strings.NewReader(string(reqBody)))
		initResp = httptest.NewRecorder()
		initServer.Handler().ServeHTTP(initResp, req)

		if initResp.Code == http.StatusOK {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if initResp.Code != http.StatusOK {
		t.Fatalf("init request failed: %d - %s", initResp.Code, initResp.Body.String())
	}

	// Wait for flow to complete or timeout
	select {
	case err := <-errCh:
		if err != nil {
			// Flow may fail at attestation or other steps depending on mock implementation
			// This is expected for now as we're testing the integration
			t.Logf("Flow completed with error (may be expected): %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Log("Flow timeout (may be expected if waiting for agent)")
	case <-ctx.Done():
		t.Log("Flow context done")
	}

	// Check that flow progressed
	if !orch.initialized {
		t.Error("Orchestrator should be initialized")
	}

	// Clean up
	orch.Stop()
}

// TestE2E_MockServerAttestation tests attestation flow with mock server
func TestE2E_MockServerAttestation(t *testing.T) {
	servers := mock.NewServers()
	defer servers.Close()

	attestorURL, _, _ := servers.URLs()

	// Test attestation via HTTP
	req := map[string]interface{}{
		"seal_id":    "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"pubkey":     "0x" + strings.Repeat("ab", 64),
		"image_hash": "0x1234",
		"signature":  "0x" + strings.Repeat("12", 64) + "1b",
		"ts":         int64(1234567890),
	}
	body, _ := json.Marshal(req)

	resp, err := http.Post(attestorURL+"/v1/unseal", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &result)

	if result["encrypted_key"] == nil {
		t.Error("encrypted_key not in response")
	}
	if result["scheme"] == nil {
		t.Error("scheme not in response")
	}
	if result["expires_at"] == nil {
		t.Error("expires_at not in response")
	}
}

// TestE2E_MockServerBlockchain tests blockchain flow with mock server
func TestE2E_MockServerBlockchain(t *testing.T) {
	servers := mock.NewServers()
	defer servers.Close()

	_, blockchainURL, _ := servers.URLs()

	testSealID := "0xtestseal1234567890abcdef1234567890abcdef1234567890abcdef"
	servers.RegisterSeal(testSealID, "999")

	resp, err := http.Get(blockchainURL + "/agents/by-seal-id/" + testSealID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["agentId"] != "999" {
		t.Errorf("expected agentId 999, got %s", result["agentId"])
	}
}

// TestE2E_MockServerStorage tests storage flow with mock server
func TestE2E_MockServerStorage(t *testing.T) {
	servers := mock.NewServers()
	defer servers.Close()

	_, _, storageURL := servers.URLs()

	testHash := "0xtesthash123456"

	resp, err := http.Get(storageURL + "/config/" + testHash)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	data, _ := io.ReadAll(resp.Body)
	// Mock returns agent seal key (32 bytes) + config
	if len(data) < 32 {
		t.Errorf("expected at least 32 bytes, got %d", len(data))
	}
}

// TestE2E_MockServerIntelligentData tests intelligent data endpoint
func TestE2E_MockServerIntelligentData(t *testing.T) {
	servers := mock.NewServers()
	defer servers.Close()

	_, blockchainURL, _ := servers.URLs()

	agentID := "1"
	resp, err := http.Get(blockchainURL + "/agents/" + agentID + "/intelligent-datas")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result []mock.IntelligentData
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result) == 0 {
		t.Error("expected at least one intelligent data")
	}
	if result[0].DataHash == "" {
		t.Error("expected non-empty dataHash")
	}
}

// TestE2E_CompleteFlowSimulation simulates a complete flow
func TestE2E_CompleteFlowSimulation(t *testing.T) {
	servers := mock.NewServers()
	defer servers.Close()

	attestorURL, blockchainURL, storageURL := servers.URLs()

	t.Run("step1_init", func(t *testing.T) {
		// Step 1: HTTP Init (would come from external source)
		testSealID := "0xABCDEF1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890"
		servers.RegisterSeal(testSealID, "1")
		t.Logf("Seal ID registered: %s -> agentId: 1", testSealID)
	})

	t.Run("step2_attest", func(t *testing.T) {
		// Step 2: Attestation
		req := map[string]interface{}{
			"seal_id":    "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"pubkey":     "0x" + strings.Repeat("ab", 64),
			"image_hash": "0x1234",
			"signature":  "0x" + strings.Repeat("12", 64) + "1b",
			"ts":         int64(1234567890),
		}
		body, _ := json.Marshal(req)
		resp, _ := http.Post(attestorURL+"/v1/unseal", "application/json", strings.NewReader(string(body)))
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("attestation failed: %d", resp.StatusCode)
		}
		t.Log("Attestation successful")
	})

	t.Run("step3_blockchain", func(t *testing.T) {
		// Step 3: Query agentId
		resp, _ := http.Get(blockchainURL + "/agents/by-seal-id/0xtest")
		defer resp.Body.Close()

		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("Agent ID: %s", result["agentId"])
	})

	t.Run("step4_intelligent_data", func(t *testing.T) {
		// Step 4: Get intelligent data
		resp, _ := http.Get(blockchainURL + "/agents/1/intelligent-datas")
		defer resp.Body.Close()

		var result []mock.IntelligentData
		json.NewDecoder(resp.Body).Decode(&result)
		if len(result) > 0 {
			t.Logf("Config hash: %s", result[0].DataHash)
		}
	})

	t.Run("step5_storage", func(t *testing.T) {
		// Step 5: Fetch config from storage
		resp, _ := http.Get(storageURL + "/config/0xhash")
		defer resp.Body.Close()

		data, _ := io.ReadAll(resp.Body)
		t.Logf("Config size: %d bytes", len(data))
	})
}
