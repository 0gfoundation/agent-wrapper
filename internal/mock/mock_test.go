package mock

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewServers(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	if servers.Attestor == nil {
		t.Error("Attestor server not created")
	}
	if servers.Blockchain == nil {
		t.Error("Blockchain server not created")
	}
	if servers.Storage == nil {
		t.Error("Storage server not created")
	}

	// Check URLs are not empty
	attestorURL, blockchainURL, storageURL := servers.URLs()
	if attestorURL == "" {
		t.Error("Attestor URL is empty")
	}
	if blockchainURL == "" {
		t.Error("Blockchain URL is empty")
	}
	if storageURL == "" {
		t.Error("Storage URL is empty")
	}
}

func TestAttestorServer_Unseal(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	req := map[string]interface{}{
		"seal_id":    "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"pubkey":     "0x" + strings.Repeat("ab", 64),
		"image_hash": "0x1234",
		"signature":  "0x" + strings.Repeat("12", 64) + "1b",
		"ts":         int64(1234567890),
	}
	body, _ := json.Marshal(req)

	resp, err := http.Post(servers.Attestor.URL+"/v1/unseal", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

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

func TestBlockchainServer_GetAgentIdBySealId(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	sealID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	// Register seal
	servers.RegisterSeal(sealID, "42")

	resp, err := http.Get(servers.Blockchain.URL + "/agents/by-seal-id/" + sealID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if result["agentId"] != "42" {
		t.Errorf("expected agentId 42, got %s", result["agentId"])
	}
}

func TestBlockchainServer_GetIntelligentDatas(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	agentID := "1"
	expectedDatas := []IntelligentData{
		{
			DataDescription: "test config",
			DataHash:        "0xabc123",
		},
	}

	// Set custom hash for default
	servers.SetConfigHash("0xabc123")

	servers.RegisterIntelligentData(agentID, expectedDatas)

	resp, err := http.Get(servers.Blockchain.URL + "/agents/" + agentID + "/intelligent-datas")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result []IntelligentData
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 data, got %d", len(result))
	}
	if result[0].DataHash != "0xabc123" {
		t.Errorf("expected hash 0xabc123, got %s", result[0].DataHash)
	}
}

func TestStorageServer_FetchConfig(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	configHash := "0x1234567890abcdef"

	resp, err := http.Get(servers.Storage.URL + "/config/" + configHash)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	// Should contain agent seal key (32 bytes) + config
	if len(data) < 32 {
		t.Errorf("expected at least 32 bytes, got %d", len(data))
	}
}

func TestServers_RegisterSeal(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	sealID := "0xABCD"
	agentID := "99"

	servers.RegisterSeal(sealID, agentID)

	resp, err := http.Get(servers.Blockchain.URL + "/agents/by-seal-id/" + sealID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["agentId"] != "99" {
		t.Errorf("expected agentId 99, got %s", result["agentId"])
	}
}

func TestServers_SetEncryptedConfig(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	expectedData := []byte("custom-encrypted-config")
	servers.SetEncryptedConfig(expectedData)

	resp, err := http.Get(servers.Storage.URL + "/config/0x1234")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(data, expectedData) {
		t.Errorf("config mismatch")
	}
}

func TestServers_GetConfigHash(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	hash := servers.GetConfigHash()
	if hash == "" {
		t.Error("hash is empty")
	}
	if len(hash) < 10 {
		t.Error("hash too short")
	}
}

func TestServers_SetConfigHash(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	customHash := "0x-custom-hash-123"
	servers.SetConfigHash(customHash)

	hash := servers.GetConfigHash()
	if hash != customHash {
		t.Errorf("expected %s, got %s", customHash, hash)
	}
}

func TestServers_GetAgentSealKey(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	key := servers.GetAgentSealKey()
	if len(key) != 32 {
		t.Errorf("expected 32 byte key, got %d", len(key))
	}
}

func TestServers_GenerateEncryptedConfig(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	encrypted := servers.GenerateEncryptedConfig()

	// Should contain key (32 bytes) + JSON config
	if len(encrypted) < 32 {
		t.Errorf("encrypted config too short: %d bytes", len(encrypted))
	}

	// Try to parse the config part (after the 32 byte key)
	configData := encrypted[32:]
	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Logf("config data: %s", string(configData))
		t.Logf("parse error (expected for mock format): %v", err)
	}
}

func TestBlockchainServer_AutoRegister(t *testing.T) {
	servers := NewServers()
	defer servers.Close()

	// Use a sealID that wasn't explicitly registered
	newSealID := "0xNEW" + hex.EncodeToString([]byte("unregistered"))

	resp, err := http.Get(servers.Blockchain.URL + "/agents/by-seal-id/" + newSealID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected auto-register to succeed, got status %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["agentId"] == "" {
		t.Error("agentId should be auto-generated")
	}
}
