// Package proxy provides HTTP proxy functionality for forwarding requests to agent
//
// Security Model:
// - Proxy forwards external requests to agent on localhost:9000
// - Internal endpoints (/_internal/*, /a2a/*) are NOT proxied - handled by wrapper
// - All proxied responses are signed with agentSeal key for verification
// - agentSeal private key never leaves wrapper process memory
//
// Note: Agent process runs on same host and can directly access wrapper's localhost:8080.
// Internal endpoints do not expose sensitive data (no private keys), but this is a
// shared trust boundary within the TEE.
package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/0g-citizen-claw/agent-wrapper/internal/flow"
	"github.com/0g-citizen-claw/agent-wrapper/internal/sealed"
)

// Proxy handles HTTP proxying to the agent
type Proxy struct {
	orchestrator flow.StatusProvider
	sealedState  *sealed.State
	agentURL     *url.URL
}

// New creates a new proxy
func New(orchestrator flow.StatusProvider, sealedState *sealed.State) *Proxy {
	return &Proxy{
		orchestrator: orchestrator,
		sealedState:  sealedState,
	}
}

// updateAgentURL updates the agent URL
func (p *Proxy) updateAgentURL() error {
	port := p.orchestrator.GetAgentPort()
	agentURL, err := url.Parse(fmt.Sprintf("http://localhost:%s", port))
	if err != nil {
		return err
	}
	p.agentURL = agentURL
	return nil
}

// ServeHTTP implements http.Handler for proxying requests
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if flow is complete
	if !p.orchestrator.IsFlowComplete() {
		http.Error(w, "Agent not ready. Waiting for initialization.", http.StatusServiceUnavailable)
		return
	}

	// Update agent URL
	if err := p.updateAgentURL(); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		log.Printf("Error updating agent URL: %v", err)
		return
	}

	// Don't proxy internal endpoints
	if isInternalEndpoint(r.URL.Path) {
		http.Error(w, "Cannot proxy internal endpoints", http.StatusForbidden)
		return
	}

	// Build proxy request
	proxyReq, err := p.buildProxyRequest(r)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		log.Printf("Error building proxy request: %v", err)
		return
	}

	// Execute proxy request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Failed to reach agent", http.StatusBadGateway)
		log.Printf("Error proxying request: %v", err)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Error reading response", http.StatusBadGateway)
		log.Printf("Error reading response: %v", err)
		return
	}

	// Copy headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Add signature headers
	timestamp := time.Now().Unix()
	signature := p.signResponse(body, timestamp)

	w.Header().Set("X-Agent-Id", p.sealedState.GetAgentID())
	w.Header().Set("X-Seal-Id", p.sealedState.GetSealID())
	w.Header().Set("X-Timestamp", fmt.Sprintf("%d", timestamp))
	w.Header().Set("X-Signature", signature)

	// Write status and body
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// buildProxyRequest builds the proxy request
func (p *Proxy) buildProxyRequest(original *http.Request) (*http.Request, error) {
	// Read body if present
	var bodyReader io.Reader
	if original.Body != nil {
		bodyBytes, err := io.ReadAll(original.Body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Build proxy URL
	proxyURL := p.agentURL.ResolveReference(original.URL)

	// Create proxy request
	proxyReq, err := http.NewRequestWithContext(
		context.Background(),
		original.Method,
		proxyURL.String(),
		bodyReader,
	)
	if err != nil {
		return nil, err
	}

	// Copy headers (excluding hop-by-hop headers)
	for key, values := range original.Header {
		if !isHopByHopHeader(key) {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
	}

	return proxyReq, nil
}

// signResponse signs the response body
func (p *Proxy) signResponse(body []byte, timestamp int64) string {
	// Create signature content: agentId|sealId|timestamp|hash(body)
	hash := sha256.Sum256(body)
	content := fmt.Sprintf("%s|%s|%d|%s",
		p.sealedState.GetAgentID(),
		p.sealedState.GetSealID(),
		timestamp,
		hex.EncodeToString(hash[:]),
	)

	// Sign using agentSeal key
	signature := p.sealedState.SignWithAgentSealKey([]byte(content))

	return hex.EncodeToString(signature)
}

// isInternalEndpoint checks if path is an internal endpoint
func isInternalEndpoint(path string) bool {
	internalPaths := []string{
		"/_internal/health",
		"/_internal/ready",
		"/_internal/init",
		"/a2a/hello",
		"/a2a/info",
	}

	for _, internal := range internalPaths {
		if path == internal || len(path) > len(internal) && path[:len(internal)] == internal {
			return true
		}
	}

	return false
}

// isHopByHopHeader checks if header is hop-by-hop
func isHopByHopHeader(header string) bool {
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, h := range hopByHopHeaders {
		if header == h {
			return true
		}
	}

	return false
}

// Handler returns the http.Handler for the proxy
func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(p.ServeHTTP)
}
