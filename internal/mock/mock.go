// Package mock provides in-memory mock servers for testing
// These servers simulate the external services (Attestor, Blockchain, Storage)
package mock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

func hasSuffix(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// Servers holds all mock servers
type Servers struct {
	Attestor   *httptest.Server
	Blockchain *httptest.Server
	Storage    *httptest.Server

	// Internal state
	mu               sync.Mutex
	agentSealKey     []byte
	configHash       string
	encryptedConfig  []byte
	sealToAgentID    map[string]string
	agentToIntelData map[string][]IntelligentData
}

// IntelligentData represents intelligent data (matches blockchain type)
type IntelligentData struct {
	DataDescription string `json:"dataDescription"`
	DataHash        string `json:"dataHash"`
}

// NewServers creates and starts all mock servers
func NewServers() *Servers {
	s := &Servers{
		sealToAgentID:    make(map[string]string),
		agentToIntelData: make(map[string][]IntelligentData),
	}

	// Generate fixed agent seal key for testing
	s.agentSealKey = make([]byte, 32)
	rand.Read(s.agentSealKey)

	// Start servers
	s.Attestor = s.startAttestor()
	s.Blockchain = s.startBlockchain()
	s.Storage = s.startStorage()

	return s
}

// startAttestor starts the mock attestation server
func (s *Servers) startAttestor() *httptest.Server {
	mux := http.NewServeMux()

	// New API: POST /v1/unseal
	mux.HandleFunc("/v1/unseal", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SealID    string `json:"seal_id"`
			PubKey    string `json:"pubkey"`
			ImageHash string `json:"image_hash"`
			Signature string `json:"signature"`
			Timestamp int64  `json:"ts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		// Validate signature format (0x + 130 hex chars)
		if len(req.Signature) != 132 || req.Signature[0:2] != "0x" {
			errResp := map[string]string{
				"code":    "INVALID_SIGNATURE",
				"message": "Signature must be 0x followed by 130 hex characters",
			}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(errResp)
			return
		}

		s.mu.Lock()
		key := s.agentSealKey
		s.mu.Unlock()

		// Return ECIES encrypted agent seal key
		resp := map[string]interface{}{
			"scheme":        "ecies-secp256k1-aes256gcm",
			"encrypted_key": hex.EncodeToString(key),
			"expires_at":    3600 + req.Timestamp, // 1 hour from now
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	return httptest.NewServer(mux)
}

// startBlockchain starts the mock blockchain server
func (s *Servers) startBlockchain() *httptest.Server {
	mux := http.NewServeMux()

	// Register a test seal -> agent mapping
	s.RegisterSeal("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", "1")

	mux.HandleFunc("/agents/by-seal-id/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract sealID from path
		sealID := r.URL.Path[len("/agents/by-seal-id/"):]
		if sealID == "" {
			http.Error(w, "sealID required", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		agentID, ok := s.sealToAgentID[sealID]
		s.mu.Unlock()

		if !ok {
			// Auto-register for any valid sealID
			s.RegisterSeal(sealID, "1")
			agentID = "1"
		}

		resp := map[string]string{
			"agentId": agentID,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/agents/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Path format: /agents/{agentID}/intelligent-datas
		// Extract agentID using string manipulation
		path := r.URL.Path
		prefix := "/agents/"
		suffix := "/intelligent-datas"

		if !hasSuffix(path, suffix) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Remove prefix and suffix
		agentID := path[len(prefix):]
		agentID = agentID[:len(agentID)-len(suffix)]

		s.mu.Lock()
		datas, ok := s.agentToIntelData[agentID]
		s.mu.Unlock()

		if !ok {
			datas = []IntelligentData{{
				DataDescription: "default config",
				DataHash:        s.GetConfigHash(),
			}}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(datas)
	})

	return httptest.NewServer(mux)
}

// startStorage starts the mock storage server
func (s *Servers) startStorage() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract hash from path
		hash := r.URL.Path[len("/config/"):]
		if hash == "" {
			http.Error(w, "hash required", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		encrypted := s.encryptedConfig
		s.mu.Unlock()

		if encrypted == nil {
			// Generate default encrypted config
			encrypted = s.GenerateEncryptedConfig()
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(encrypted)
	})

	return httptest.NewServer(mux)
}

// RegisterSeal registers a sealID -> agentID mapping
func (s *Servers) RegisterSeal(sealID, agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealToAgentID[sealID] = agentID
}

// RegisterIntelligentData registers intelligent data for an agent
func (s *Servers) RegisterIntelligentData(agentID string, datas []IntelligentData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentToIntelData[agentID] = datas
}

// SetEncryptedConfig sets the encrypted config to return
func (s *Servers) SetEncryptedConfig(encrypted []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.encryptedConfig = encrypted
}

// GetConfigHash returns the config hash
func (s *Servers) GetConfigHash() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configHash == "" {
		s.configHash = "0x" + hex.EncodeToString([]byte("default-config-hash"))
	}
	return s.configHash
}

// GenerateEncryptedConfig generates a default encrypted config
func (s *Servers) GenerateEncryptedConfig() []byte {
	s.mu.Lock()
	key := s.agentSealKey
	s.mu.Unlock()

	// Create a simple config (plaintext for mock)
	config := map[string]interface{}{
		"framework": map[string]string{
			"name":    "openclaw",
			"version": "0.1.0",
		},
		"runtime": map[string]interface{}{
			"entryPoint": "python3 main.py",
			"workingDir": "/app",
			"agentPort":  9000,
		},
		"env": map[string]string{
			"LOG_LEVEL": "info",
		},
	}

	jsonConfig, _ := json.Marshal(config)

	// For mock, just prepend the key as "nonce" (simulated format)
	// Real implementation would use AES-GCM
	result := append(key, jsonConfig...)
	s.mu.Lock()
	s.encryptedConfig = result
	s.mu.Unlock()

	return result
}

// Close closes all mock servers
func (s *Servers) Close() {
	if s.Attestor != nil {
		s.Attestor.Close()
	}
	if s.Blockchain != nil {
		s.Blockchain.Close()
	}
	if s.Storage != nil {
		s.Storage.Close()
	}
}

// URLs returns the base URLs of all servers
func (s *Servers) URLs() (attestor, blockchain, storage string) {
	return s.Attestor.URL, s.Blockchain.URL, s.Storage.URL
}

// GetAgentSealKey returns the agent seal key (for test assertions)
func (s *Servers) GetAgentSealKey() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agentSealKey
}

// SetConfigHash sets the config hash
func (s *Servers) SetConfigHash(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configHash = hash
}
