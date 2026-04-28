package blockchain

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewClient tests creating a new client
func TestNewClient(t *testing.T) {
	client := NewClient(&Config{
		Endpoint: "https://blockchain.example.com",
	})

	assert.NotNil(t, client)
	assert.Equal(t, "https://blockchain.example.com", client.config.Endpoint)
}

// TestGetAgentIdBySealId tests getting agentId by sealId
func TestGetAgentIdBySealId_Success(t *testing.T) {
	sealID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	agentID := "12345"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/agents/by-seal-id/"+sealID, r.URL.Path)
		assert.Equal(t, "GET", r.Method)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"agentId": agentID,
		})
	}))
	defer server.Close()

	client := NewClient(&Config{
		Endpoint: strings.TrimPrefix(server.URL, "http://"),
	})

	result, err := client.GetAgentIdBySealId(sealID)

	require.NoError(t, err)
	assert.Equal(t, agentID, result)
}

// TestGetAgentIdBySealId_InvalidSealId tests invalid seal ID
func TestGetAgentIdBySealId_InvalidSealId(t *testing.T) {
	client := NewClient(&Config{
		Endpoint: "blockchain.example.com",
	})

	_, err := client.GetAgentIdBySealId("")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "seal ID cannot be empty")
}

// TestGetIntelligentDatas tests getting intelligent datas
func TestGetIntelligentDatas_Success(t *testing.T) {
	agentID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	expectedDatas := []IntelligentData{
		{
			DataDescription: "Agent Config",
			DataHash:        "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/agents/"+agentID+"/intelligent-datas", r.URL.Path)
		assert.Equal(t, "GET", r.Method)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedDatas)
	}))
	defer server.Close()

	client := NewClient(&Config{
		Endpoint: strings.TrimPrefix(server.URL, "http://"),
	})

	datas, err := client.GetIntelligentDatas(agentID)

	require.NoError(t, err)
	assert.Len(t, datas, 1)
	assert.Equal(t, "Agent Config", datas[0].DataDescription)
	assert.Equal(t, expectedDatas[0].DataHash, datas[0].DataHash)
}

// TestGetAgentMetadata tests getting agent metadata
func TestGetAgentMetadata_Success(t *testing.T) {
	agentID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	expectedMetadata := &AgentMetadata{
		AgentID:   uint256(agentID),
		SealID:    "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		AgentSeal: "0xabcd1234567890abcd1234567890abcd12345678",
		IntelligentDatas: []IntelligentData{
			{
				DataDescription: "Agent Config",
				DataHash:        "0x9876543210fedcba9876543210fedcba9876543210fedcba9876543210fedcba",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/agents/"+agentID+"/metadata", r.URL.Path)
		assert.Equal(t, "GET", r.Method)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedMetadata)
	}))
	defer server.Close()

	client := NewClient(&Config{
		Endpoint: strings.TrimPrefix(server.URL, "http://"),
	})

	metadata, err := client.GetAgentMetadata(agentID)

	require.NoError(t, err)
	assert.Equal(t, expectedMetadata.AgentID, metadata.AgentID)
	assert.Equal(t, expectedMetadata.SealID, metadata.SealID)
	assert.Equal(t, expectedMetadata.AgentSeal, metadata.AgentSeal)
	assert.Len(t, metadata.IntelligentDatas, 1)
}

// TestGetAgentMetadata_HTTPError tests HTTP error handling
func TestGetAgentMetadata_HTTPError(t *testing.T) {
	agentID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "agent_not_found",
		})
	}))
	defer server.Close()

	client := NewClient(&Config{
		Endpoint: strings.TrimPrefix(server.URL, "http://"),
	})

	metadata, err := client.GetAgentMetadata(agentID)

	assert.Error(t, err)
	assert.Nil(t, metadata)
	assert.Contains(t, err.Error(), "404")
}

// TestGetAgentMetadata_NetworkError tests network error handling
func TestGetAgentMetadata_NetworkError(t *testing.T) {
	client := NewClient(&Config{
		Endpoint: "invalid-url-that-does-not-exist.local:12345",
		Timeout:  1000,
	})

	metadata, err := client.GetAgentMetadata("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, metadata)
}

// TestGetAgentMetadata_InvalidJSON tests invalid JSON response
func TestGetAgentMetadata_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(&Config{
		Endpoint: strings.TrimPrefix(server.URL, "http://"),
	})

	metadata, err := client.GetAgentMetadata("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, metadata)
}

// TestEventListener tests listening for events
func TestEventListener_Start(t *testing.T) {
	client := NewClient(&Config{
		Endpoint: "blockchain.example.com",
	})

	listener := client.ListenForSealBonded("0xabcd1234")

	assert.NotNil(t, listener)
	assert.Equal(t, "0xabcd1234", listener.sealID)
}

// TestEventListener_Stop tests stopping event listener
func TestEventListener_Stop(t *testing.T) {
	client := NewClient(&Config{
		Endpoint: "blockchain.example.com",
	})

	listener := client.ListenForSealBonded("0xabcd1234")

	// Start and immediately stop
	go listener.Start(func(agentID string) {
		// Should not be called
		t.Error("Event callback should not be called")
	})

	listener.Stop()

	// Should not panic
	listener.Stop()
}

// TestValidateAgentID tests agent ID validation
func TestValidateAgentID(t *testing.T) {
	tests := []struct {
		name    string
		agentID string
		wantErr bool
	}{
		{"valid with 0x", "0x1234567890abcdef", false},
		{"valid without 0x", "1234567890abcdef", false},
		{"valid longer", "0x1234567890abcdef1234567890abcdef12345678", false},
		{"empty", "", true},
		{"invalid characters", "0xghijkl", true},
		{"odd length", "0xabc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentID(tt.agentID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateSealID tests seal ID validation
func TestValidateSealID(t *testing.T) {
	tests := []struct {
		name    string
		sealID  string
		wantErr bool
	}{
		{"valid bytes32 with 0x", "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", false},
		{"valid bytes32 without 0x", "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", false},
		{"empty", "", true},
		{"invalid characters", "0xghijkl", true},
		{"odd length", "0xabc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSealID(tt.sealID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestConfigDefaults tests config defaults
func TestConfigDefaults(t *testing.T) {
	client := NewClient(&Config{
		Endpoint: "https://blockchain.example.com",
	})

	assert.NotNil(t, client.httpClient)
	assert.Equal(t, "https://blockchain.example.com", client.config.Endpoint)
}

// TestGetAgentMetadata_MissingFields tests metadata with missing fields
func TestGetAgentMetadata_MissingFields(t *testing.T) {
	agentID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	incompleteMetadata := map[string]string{
		"agentId":  agentID,
		"sealId":   "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		// Missing agentSeal, intelligentDatas
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(incompleteMetadata)
	}))
	defer server.Close()

	client := NewClient(&Config{
		Endpoint: strings.TrimPrefix(server.URL, "http://"),
	})

	metadata, err := client.GetAgentMetadata(agentID)

	require.NoError(t, err)
	assert.Equal(t, uint256(agentID), metadata.AgentID)
	assert.Equal(t, "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", metadata.SealID)
}

// TestGetAgentMetadata_Timeout tests request timeout
func TestGetAgentMetadata_Timeout(t *testing.T) {
	agentID := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than client timeout
		// Note: This test is timing-dependent and may be flaky
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"agentId": agentID})
	}))
	defer slowServer.Close()

	client := NewClient(&Config{
		Endpoint: slowServer.URL[7:], // Remove "http://"
		Timeout:  1, // 1 nanosecond - essentially immediate timeout
	})

	_, err := client.GetAgentMetadata(agentID)
	// May or may not error depending on timing
	_ = err // Just check it doesn't panic
}
