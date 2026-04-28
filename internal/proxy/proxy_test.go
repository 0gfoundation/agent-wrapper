package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/0g-citizen-claw/agent-wrapper/internal/flow"
	"github.com/0g-citizen-claw/agent-wrapper/internal/sealed"
)

// Compile-time interface check
var _ flow.StatusProvider = (*mockOrchestrator)(nil)

// mockOrchestrator is a mock implementation of flow.StatusProvider
type mockOrchestrator struct {
	flowComplete bool
	agentPort    string
}

func (m *mockOrchestrator) IsFlowComplete() bool {
	return m.flowComplete
}

func (m *mockOrchestrator) GetAgentPort() string {
	return m.agentPort
}

// getTestServerPort extracts the port number from a test server URL
func getTestServerPort(serverURL string) string {
	u, err := strconv.Atoi(serverURL[strings.LastIndex(serverURL, ":")+1:])
	if err != nil {
		return "8080"
	}
	return strconv.Itoa(u)
}

func TestNew(t *testing.T) {
	orch := &mockOrchestrator{flowComplete: true, agentPort: "9000"}
	sealedState := sealed.NewState()

	proxy := New(orch, sealedState)

	if proxy == nil {
		t.Fatal("expected non-nil proxy")
	}
	if proxy.orchestrator == nil {
		t.Error("orchestrator not set correctly")
	}
	if proxy.sealedState != sealedState {
		t.Error("sealedState not set correctly")
	}
}

func TestServeHTTP(t *testing.T) {
	t.Run("returns 503 when flow not complete", func(t *testing.T) {
		orch := &mockOrchestrator{flowComplete: false}
		sealedState := sealed.NewState()
		proxy := New(orch, sealedState)

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		proxy.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
		}
	})

	t.Run("blocks internal endpoints", func(t *testing.T) {
		orch := &mockOrchestrator{flowComplete: true}
		sealedState := sealed.NewState()
		proxy := New(orch, sealedState)

		internalPaths := []string{
			"/_internal/health",
			"/_internal/ready",
			"/_internal/init",
			"/_internal/health/sub",
			"/a2a/hello",
			"/a2a/info",
			"/a2a/hello/sub",
		}

		for _, path := range internalPaths {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			proxy.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("path %s: expected status %d, got %d", path, http.StatusForbidden, w.Code)
			}
		}
	})

	t.Run("proxies request to agent and adds signature headers", func(t *testing.T) {
		// Create a mock agent server
		agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/chat" {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"response": "hello"}`))
		}))
		defer agentServer.Close()

		// Initialize sealed state
		sealedState := sealed.NewState()
		if err := sealedState.Initialize("test-seal", "temp-key", "https://attestor.example.com"); err != nil {
			t.Fatalf("failed to initialize sealed state: %v", err)
		}
		sealedState.SetAgentID("test-agent-id")
		// Set mock agentSeal key (32 bytes)
		mockAgentSealKey := make([]byte, 32)
		for i := range mockAgentSealKey {
			mockAgentSealKey[i] = byte(i)
		}
		sealedState.SetAgentSealKey(mockAgentSealKey)

		// Create proxy with mock orchestrator
		port := getTestServerPort(agentServer.URL)
		orch := &mockOrchestrator{flowComplete: true, agentPort: port}
		proxy := New(orch, sealedState)

		// Make request through proxy
		req := httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message": "test"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		proxy.ServeHTTP(w, req)

		// Check response
		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		body := w.Body.String()
		if body != `{"response": "hello"}` {
			t.Errorf("expected body '{\"response\": \"hello\"}', got %s", body)
		}

		// Check signature headers
		agentID := w.Header().Get("X-Agent-Id")
		if agentID != "test-agent-id" {
			t.Errorf("expected X-Agent-Id 'test-agent-id', got %s", agentID)
		}

		sealID := w.Header().Get("X-Seal-Id")
		if sealID != "test-seal" {
			t.Errorf("expected X-Seal-Id 'test-seal', got %s", sealID)
		}

		timestamp := w.Header().Get("X-Timestamp")
		if timestamp == "" {
			t.Error("expected X-Timestamp header")
		}

		signature := w.Header().Get("X-Signature")
		if signature == "" {
			t.Error("expected X-Signature header")
		}
	})

	t.Run("returns 502 when agent is unreachable", func(t *testing.T) {
		orch := &mockOrchestrator{flowComplete: true, agentPort: "9999"} // Non-existent port
		sealedState := sealed.NewState()
		proxy := New(orch, sealedState)

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		proxy.ServeHTTP(w, req)

		if w.Code != http.StatusBadGateway {
			t.Errorf("expected status %d, got %d", http.StatusBadGateway, w.Code)
		}
	})

	t.Run("forwards request body and headers", func(t *testing.T) {
		receivedBody := ""
		receivedHeaders := http.Header{}

		agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedBody = string(body)
			receivedHeaders = r.Header.Clone()

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok": true}`))
		}))
		defer agentServer.Close()

		sealedState := sealed.NewState()
		sealedState.Initialize("test-seal", "temp-key", "https://attestor.example.com")
		sealedState.SetAgentID("test-agent-id")
		// Set mock agentSeal key (32 bytes)
		mockAgentSealKey := make([]byte, 32)
		for i := range mockAgentSealKey {
			mockAgentSealKey[i] = byte(i)
		}
		sealedState.SetAgentSealKey(mockAgentSealKey)

		port := getTestServerPort(agentServer.URL)
		orch := &mockOrchestrator{flowComplete: true, agentPort: port}
		proxy := New(orch, sealedState)

		req := httptest.NewRequest("POST", "/endpoint", strings.NewReader(`test body`))
		req.Header.Set("X-Custom-Header", "custom-value")
		req.Header.Set("Connection", "close") // Hop-by-hop header
		w := httptest.NewRecorder()

		proxy.ServeHTTP(w, req)

		if receivedBody != "test body" {
			t.Errorf("expected body 'test body', got %s", receivedBody)
		}

		if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
			t.Error("custom header not forwarded")
		}

		if receivedHeaders.Get("Connection") != "" {
			t.Error("hop-by-hop header should not be forwarded")
		}
	})
}

func TestIsInternalEndpoint(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/_internal/health", true},
		{"/_internal/ready", true},
		{"/_internal/init", true},
		{"/_internal/health/sub", true},
		{"/_internal/ready?query=1", true},
		{"/a2a/hello", true},
		{"/a2a/info", true},
		{"/a2a/hello/sub", true}, // Prefix matching
		{"/a2a/info/detail", true},
		{"/api/chat", false},
		{"/health", false},
		{"/internal/test", false}, // Missing underscore prefix
		{"/a2a/other", false},     // Non-A2A endpoint
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isInternalEndpoint(tt.path)
			if result != tt.expected {
				t.Errorf("path %s: expected %v, got %v", tt.path, tt.expected, result)
			}
		})
	}
}

func TestIsHopByHopHeader(t *testing.T) {
	tests := []struct {
		header   string
		expected bool
	}{
		{"Connection", true},
		{"Keep-Alive", true},
		{"Proxy-Authenticate", true},
		{"Proxy-Authorization", true},
		{"Te", true},
		{"Trailers", true},
		{"Transfer-Encoding", true},
		{"Upgrade", true},
		{"Content-Type", false},
		{"Authorization", false},
		{"X-Custom-Header", false},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			result := isHopByHopHeader(tt.header)
			if result != tt.expected {
				t.Errorf("header %s: expected %v, got %v", tt.header, tt.expected, result)
			}
		})
	}
}

func TestHandler(t *testing.T) {
	orch := &mockOrchestrator{flowComplete: true}
	sealedState := sealed.NewState()
	proxy := New(orch, sealedState)

	handler := proxy.Handler()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	// Verify it implements http.Handler
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should fail since there's no actual agent running
	// but we're just testing that the handler works
	if w.Code == 0 {
		t.Error("handler should have written a response")
	}
}

func TestSignResponse(t *testing.T) {
	t.Run("produces valid signature", func(t *testing.T) {
		sealedState := sealed.NewState()
		if err := sealedState.Initialize("test-seal", "temp-key", "https://attestor.example.com"); err != nil {
			t.Fatalf("failed to initialize sealed state: %v", err)
		}
		sealedState.SetAgentID("test-agent-id")
		// Set mock agentSeal key (32 bytes)
		mockAgentSealKey := make([]byte, 32)
		for i := range mockAgentSealKey {
			mockAgentSealKey[i] = byte(i)
		}
		sealedState.SetAgentSealKey(mockAgentSealKey)

		orch := &mockOrchestrator{flowComplete: true}
		proxy := New(orch, sealedState)

		body := []byte(`{"test": "data"}`)
		timestamp := int64(1234567890)

		signature := proxy.signResponse(body, timestamp)

		if signature == "" {
			t.Error("expected non-empty signature")
		}

		// Signature should be hex encoded (128 chars for 64 bytes)
		if len(signature) != 128 {
			t.Errorf("expected signature length 128, got %d", len(signature))
		}
	})

	t.Run("verifies signature can be verified", func(t *testing.T) {
		sealedState := sealed.NewState()
		if err := sealedState.Initialize("test-seal", "temp-key", "https://attestor.example.com"); err != nil {
			t.Fatalf("failed to initialize sealed state: %v", err)
		}
		sealedState.SetAgentID("test-agent-id")
		// Set mock agentSeal key (32 bytes)
		mockAgentSealKey := make([]byte, 32)
		for i := range mockAgentSealKey {
			mockAgentSealKey[i] = byte(i)
		}
		sealedState.SetAgentSealKey(mockAgentSealKey)

		orch := &mockOrchestrator{flowComplete: true}
		proxy := New(orch, sealedState)

		body := []byte(`{"test": "data"}`)
		timestamp := int64(1234567890)

		signature := proxy.signResponse(body, timestamp)

		// Decode signature from hex
		sigBytes, err := hex.DecodeString(signature)
		if err != nil {
			t.Fatalf("failed to decode signature: %v", err)
		}

		// Create the signed content
		hash := sha256.Sum256(body)
		content := fmt.Sprintf("%s|%s|%d|%x",
			sealedState.GetAgentID(),
			sealedState.GetSealID(),
			timestamp,
			hash,
		)

		// Verify signature
		if !sealedState.VerifySignatureWithAgentSealKey([]byte(content), sigBytes) {
			t.Error("signature should be verifiable")
		}
	})
}
