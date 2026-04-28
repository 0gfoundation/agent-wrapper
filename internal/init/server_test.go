package init

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitHandler_ValidRequest tests successful initialization
func TestInitHandler_ValidRequest(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	reqBody := `{
		"sealId": "0x1234567890abcdef",
		"tempKey": "0xabcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd",
		"attestorUrl": "https://attestor.example.com"
	}`

	resp := srv.Post("/_internal/init", reqBody)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result InitResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "sealed", result.Status)
	assert.NotEmpty(t, result.Message)
}

// TestInitHandler_MissingSealId tests missing sealId
func TestInitHandler_MissingSealId(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	reqBody := `{
		"tempKey": "0xabcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd",
		"attestorUrl": "https://attestor.example.com"
	}`

	resp := srv.Post("/_internal/init", reqBody)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "invalid_request", result.Error)
	assert.Contains(t, result.Message, "sealId")
}

// TestInitHandler_InvalidSealId tests invalid sealId format
func TestInitHandler_InvalidSealId(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	reqBody := `{
		"sealId": "not-a-valid-hex",
		"tempKey": "0xabcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd",
		"attestorUrl": "https://attestor.example.com"
	}`

	resp := srv.Post("/_internal/init", reqBody)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "invalid_request", result.Error)
}

// TestInitHandler_MissingTempKey tests missing tempKey
func TestInitHandler_MissingTempKey(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	reqBody := `{
		"sealId": "0x1234567890abcdef",
		"attestorUrl": "https://attestor.example.com"
	}`

	resp := srv.Post("/_internal/init", reqBody)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "invalid_request", result.Error)
	assert.Contains(t, result.Message, "tempKey")
}

// TestInitHandler_InvalidTempKey tests invalid tempKey format
func TestInitHandler_InvalidTempKey(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	reqBody := `{
		"sealId": "0x1234567890abcdef",
		"tempKey": "not-a-valid-hex",
		"attestorUrl": "https://attestor.example.com"
	}`

	resp := srv.Post("/_internal/init", reqBody)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "invalid_request", result.Error)
}

// TestInitHandler_MissingAttestorUrl tests missing attestorUrl
func TestInitHandler_MissingAttestorUrl(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	reqBody := `{
		"sealId": "0x1234567890abcdef",
		"tempKey": "0xabcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd"
	}`

	resp := srv.Post("/_internal/init", reqBody)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "invalid_request", result.Error)
	assert.Contains(t, result.Message, "attestorUrl")
}

// TestInitHandler_InvalidJSON tests invalid JSON request
func TestInitHandler_InvalidJSON(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	resp := srv.Post("/_internal/init", "invalid json")

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestInitHandler_AlreadyInitialized tests duplicate initialization
func TestInitHandler_AlreadyInitialized(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	reqBody := `{
		"sealId": "0x1234567890abcdef",
		"tempKey": "0xabcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd",
		"attestorUrl": "https://attestor.example.com"
	}`

	// First request should succeed
	resp1 := srv.Post("/_internal/init", reqBody)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// Second request should fail
	resp2 := srv.Post("/_internal/init", reqBody)
	assert.Equal(t, http.StatusConflict, resp2.StatusCode)

	var result ErrorResponse
	err := json.NewDecoder(resp2.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "already_initialized", result.Error)
}

// TestHealthHandler tests health check endpoint
func TestHealthHandler(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	resp := srv.Get("/_internal/health")

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result HealthResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "healthy", result.Status)
	assert.NotEmpty(t, result.Version)
}

// TestReadyHandler_BeforeInit tests ready check before initialization
func TestReadyHandler_BeforeInit(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	resp := srv.Get("/_internal/ready")

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var result ReadyResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.False(t, result.Ready)
	assert.Equal(t, "waiting_init", result.State)
}

// TestReadyHandler_AfterInit tests ready check after initialization
func TestReadyHandler_AfterInit(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	// First initialize
	reqBody := `{
		"sealId": "0x1234567890abcdef",
		"tempKey": "0xabcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd",
		"attestorUrl": "https://attestor.example.com"
	}`
	srv.Post("/_internal/init", reqBody)

	// Check ready
	resp := srv.Get("/_internal/ready")

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var result ReadyResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.False(t, result.Ready) // Not ready until full bootstrap
	assert.Equal(t, "sealed", result.State)
}

// TestValidateSealId tests sealId validation
func TestValidateSealId(t *testing.T) {
	tests := []struct {
		name    string
		sealId  string
		wantErr bool
	}{
		{"valid with 0x prefix", "0x1234567890abcdef", false},
		{"valid without 0x prefix", "1234567890abcdef", false},
		{"valid longer", "0x1234567890abcdef1234567890abcdef12345678", false},
		{"empty", "", true},
		{"invalid characters", "0xghijkl", true},
		{"odd length", "0xabc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSealId(tt.sealId)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateTempKey tests tempKey validation
func TestValidateTempKey(t *testing.T) {
	tests := []struct {
		name    string
		tempKey string
		wantErr bool
	}{
		{"valid with 0x prefix", "0xabcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd", false},
		{"valid without 0x prefix", "abcd1234567890abcd1234567890abcd1234567890abcd1234567890abcd", false},
		{"empty", "", true},
		{"invalid characters", "0xghijkl", true},
		{"too short", "0xabcd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTempKey(tt.tempKey)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateAttestorUrl tests attestorUrl validation
func TestValidateAttestorUrl(t *testing.T) {
	tests := []struct {
		name       string
		attestorUrl string
		wantErr    bool
	}{
		{"valid https", "https://attestor.example.com", false},
		{"valid http", "http://attestor.example.com", false},
		{"valid with port", "https://attestor.example.com:8080", false},
		{"valid with path", "https://attestor.example.com/v1", false},
		{"empty", "", true},
		{"invalid format", "not-a-url", true},
		{"missing scheme", "attestor.example.com", true},
		{"invalid port", "https://attestor.example.com:abc", true},
		{"localhost", "http://localhost:8080", false},
		{"ip address", "http://127.0.0.1:8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAttestorUrl(tt.attestorUrl)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestServer_IsInitialized tests IsInitialized method
func TestServer_IsInitialized(t *testing.T) {
	s := New()

	assert.False(t, s.IsInitialized())

	// Simulate initialization
	s.stateMu.Lock()
	s.initialized = true
	s.stateMu.Unlock()

	assert.True(t, s.IsInitialized())
}

// TestServer_SetStatus tests SetStatus method
func TestServer_SetStatus(t *testing.T) {
	s := New()

	s.SetStatus("test_status")

	assert.Equal(t, "test_status", s.GetState().Status)
}

// TestServer_WaitUntilInitialized tests WaitUntilInitialized
func TestServer_WaitUntilInitialized(t *testing.T) {
	s := New()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should timeout when not initialized
	_, err := s.WaitUntilInitialized(ctx)
	assert.Error(t, err)
}

// TestServer_WaitUntilInitialized_Success tests successful wait
func TestServer_WaitUntilInitialized_Success(t *testing.T) {
	s := New()

	// Initialize in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.stateMu.Lock()
		s.initialized = true
		s.stateMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	state, err := s.WaitUntilInitialized(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, state)
}



// TestInitHandler_ValidWithRealTempKey tests with realistic temp key
func TestInitHandler_ValidWithRealTempKey(t *testing.T) {
	srv := NewTestServer(t)
	defer srv.Close()

	// 64 hex chars = 32 bytes (secp256k1 private key size)
	tempKey := "0x" + strings.Repeat("ab", 32) // 64 hex chars

	reqBody := `{
		"sealId": "0x1234567890abcdef1234567890abcdef12345678",
		"tempKey": "` + tempKey + `",
		"attestorUrl": "https://attestor.example.com"
	}`

	resp := srv.Post("/_internal/init", reqBody)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result InitResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "sealed", result.Status)
}

// TestHealthHandler_WithVersion tests health check includes version
func TestHealthHandler_WithVersion(t *testing.T) {
	s := New()
	s.Version = "1.2.3"

	server := httptest.NewServer(s.Handler())
	defer server.Close()

	// Give some time for uptime to accumulate
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get(server.URL + "/_internal/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	var result HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "1.2.3", result.Version)
	assert.GreaterOrEqual(t, result.Uptime, int64(10))
}

// TestServer is a test helper for HTTP server testing
type TestServer struct {
	server *httptest.Server
	t      *testing.T
}

// NewTestServer creates a new test server
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()
	s := &Server{
		Version: "test",
	}
	ts := httptest.NewServer(s.Handler())
	return &TestServer{server: ts, t: t}
}

// Close closes the test server
func (s *TestServer) Close() {
	s.server.Close()
}

// Get performs a GET request
func (s *TestServer) Get(path string) *http.Response {
	req, err := http.NewRequest("GET", s.server.URL+path, nil)
	require.NoError(s.t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(s.t, err)
	return resp
}

// Post performs a POST request
func (s *TestServer) Post(path string, body string) *http.Response {
	req, err := http.NewRequest("POST", s.server.URL+path, strings.NewReader(body))
	require.NoError(s.t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(s.t, err)
	return resp
}
