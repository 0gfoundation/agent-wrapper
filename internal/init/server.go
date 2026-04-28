// Package init provides HTTP initialization endpoints for the agent wrapper.
package init

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultPort is the default port for the HTTP server
	DefaultPort = ":8080"

	// Version is the wrapper version
	Version = "0.2.0"
)

// Server handles HTTP initialization requests
type Server struct {
	// Version is the wrapper version
	Version string

	// startTime is when the server started
	startTime time.Time

	// stateMu protects state access
	stateMu sync.RWMutex

	// initialized indicates if init has been called
	initialized bool

	// initState holds the initialization state
	initState *InitState

	// server is the HTTP server
	server *http.Server

	// done channel for graceful shutdown
	done chan struct{}

	// logBuffer holds log lines for /dashboard
	logBuffer []string
	logMu     sync.RWMutex
	logSize   int32 // maximum log lines to keep (atomic for reads)
}

// InitState holds the initialization state
type InitState struct {
	// SealID is the seal identifier
	SealID string

	// TempKey is the temporary attestation key
	TempKey string

	// AttestorURL is the Attestor service URL
	AttestorURL string


	// Status is the current status
	Status string
}

// InitRequest is the request body for initialization
type InitRequest struct {
	SealID      string `json:"sealId"`
	TempKey     string `json:"tempKey"`
	AttestorURL string `json:"attestorUrl"`
}

// InitResponse is the response for successful initialization
type InitResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ErrorResponse is the error response format
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// HealthResponse is the health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  int64  `json:"uptime,omitempty"`
}

// ReadyResponse is the readiness check response
type ReadyResponse struct {
	Ready   bool   `json:"ready"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	AgentID string `json:"agentId,omitempty"`
	SealID  string `json:"sealId,omitempty"`
}

// New creates a new initialization server
func New() *Server {
	return &Server{
		Version:   Version,
		startTime: time.Now(),
		done:      make(chan struct{}),
		initState: &InitState{Status: "waiting_init"},
		logBuffer: make([]string, 0, 1000),
		logSize:   1000, // keep last 1000 log lines
	}
}

// Start starts the HTTP server on the specified port
func (s *Server) Start(port string) error {
	if port == "" {
		port = DefaultPort
	}

	s.server = &http.Server{
		Addr:    port,
		Handler: s.Handler(),
	}

	s.server.RegisterOnShutdown(func() {
		close(s.done)
	})

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// Handler returns the main HTTP handler
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Internal endpoints only
	// All other requests are handled by the proxy module at the main router level
	mux.HandleFunc("/_internal/init", s.handleInit)
	mux.HandleFunc("/_internal/health", s.handleHealth)
	mux.HandleFunc("/_internal/ready", s.handleReady)
	mux.HandleFunc("/_internal/dashboard", s.handleDashboard)

	return mux
}

// handleInit handles initialization requests
func (s *Server) handleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed")
		return
	}

	// Check if already initialized
	s.stateMu.Lock()
	if s.initialized {
		s.stateMu.Unlock()
		s.writeError(w, http.StatusConflict, "already_initialized", "Already initialized")
		return
	}
	s.stateMu.Unlock()

	// Parse request
	var req InitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON")
		return
	}

	// Validate request
	if err := validateRequest(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Store initialization state
	s.stateMu.Lock()
	s.initialized = true
	s.initState = &InitState{
		SealID:      req.SealID,
		TempKey:     req.TempKey,
		AttestorURL: req.AttestorURL,
		Status:      "sealed",
	}
	s.stateMu.Unlock()

	// Write response
	s.writeJSON(w, http.StatusOK, InitResponse{
		Status:  "sealed",
		Message: "Entered sealed state, waiting for attestation",
	})
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	s.writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "healthy",
		Version: s.Version,
		Uptime:  time.Since(s.startTime).Milliseconds(),
	})
}

// handleReady handles readiness check requests
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	resp := ReadyResponse{
		Ready: false,
	}

	if s.initState != nil {
		resp.State = s.initState.Status
	} else {
		resp.State = "waiting_init"
	}

	if s.initialized && s.initState != nil {
		resp.SealID = s.initState.SealID
		// Ready is true only when status is "ready"
		resp.Ready = s.initState.Status == "ready"
	} else {
		resp.Message = "Waiting for initialization"
	}

	if resp.Ready {
		s.writeJSON(w, http.StatusOK, resp)
	} else {
		s.writeJSON(w, http.StatusServiceUnavailable, resp)
	}
}

// handleDashboard returns the accumulated log output (like bootstrap's /dashboard)
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	// Return logs as plain text (like bootstrap)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, s.GetLogBuffer())
}

// GetState returns the current initialization state (thread-safe)
func (s *Server) GetState() *InitState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.initState
}

// SetStatus updates the current status (thread-safe)
func (s *Server) SetStatus(status string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.initState != nil {
		s.initState.Status = status
	}
}

// validateRequest validates the initialization request
func validateRequest(req *InitRequest) error {
	if err := ValidateSealId(req.SealID); err != nil {
		return fmt.Errorf("sealId: %w", err)
	}
	if err := ValidateTempKey(req.TempKey); err != nil {
		return fmt.Errorf("tempKey: %w", err)
	}
	if err := ValidateAttestorUrl(req.AttestorURL); err != nil {
		return fmt.Errorf("attestorUrl: %w", err)
	}
	return nil
}

// ValidateSealId validates a seal ID
func ValidateSealId(sealId string) error {
	if sealId == "" {
		return errors.New("is required and must be a valid hex string")
	}
	// Strip 0x prefix
	hexStr := strings.TrimPrefix(sealId, "0x")
	// Validate hex
	_, err := hex.DecodeString(hexStr)
	if err != nil {
		return fmt.Errorf("must be a valid hex string: %w", err)
	}
	// Check even length (full bytes)
	if len(hexStr)%2 != 0 {
		return errors.New("must be a valid hex string with even length")
	}
	return nil
}

// ValidateTempKey validates a temporary key
func ValidateTempKey(tempKey string) error {
	if tempKey == "" {
		return errors.New("is required and must be a valid hex string")
	}
	// Strip 0x prefix
	hexStr := strings.TrimPrefix(tempKey, "0x")
	// Validate hex
	_, err := hex.DecodeString(hexStr)
	if err != nil {
		return fmt.Errorf("must be a valid hex string: %w", err)
	}
	// Check minimum length (at least 24 bytes = 48 hex chars for secp256k1)
	if len(hexStr) < 48 {
		return errors.New("must be at least 48 hex characters (24 bytes)")
	}
	// Check even length
	if len(hexStr)%2 != 0 {
		return errors.New("must be a valid hex string with even length")
	}
	return nil
}

// ValidateAttestorUrl validates an attestor URL
func ValidateAttestorUrl(attestorURL string) error {
	if attestorURL == "" {
		return errors.New("is required")
	}
	// Basic URL validation
	if !strings.HasPrefix(attestorURL, "http://") && !strings.HasPrefix(attestorURL, "https://") {
		return errors.New("must be a valid URL starting with http:// or https://")
	}
	// Try to parse as URL
	u, err := parseURL(attestorURL)
	if err != nil {
		return fmt.Errorf("must be a valid URL: %w", err)
	}
	// Check host
	if u.Host == "" {
		return errors.New("must include a host")
	}
	return nil
}

// parseURL parses a URL (net/url is not used to reduce dependencies)
func parseURL(rawURL string) (*struct {
	Scheme string
	Host   string
	Path   string
}, error) {
	parts := strings.SplitN(rawURL, "://", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid URL format")
	}

	scheme := parts[0]
	rest := parts[1]

	// Split host and path
	hostPath := strings.SplitN(rest, "/", 2)
	host := hostPath[0]
	path := ""
	if len(hostPath) > 1 {
		path = "/" + hostPath[1]
	}

	// Handle port
	if _, port, err := net.SplitHostPort(host); err == nil {
		if port != "" {
			if _, err := strconv.ParseUint(port, 10, 16); err != nil {
				return nil, errors.New("invalid port")
			}
		}
	}

	return &struct {
		Scheme string
		Host   string
		Path   string
	}{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}, nil
}


// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, ErrorResponse{
		Error:   code,
		Message: message,
	})
}

// WaitUntilInitialized waits until the server is initialized or context is cancelled
func (s *Server) WaitUntilInitialized(ctx context.Context) (*InitState, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			s.stateMu.RLock()
			initialized := s.initialized
			s.stateMu.RUnlock()

			if initialized {
				return s.GetState(), nil
			}
		}
	}
}

// IsInitialized returns whether the server has been initialized
func (s *Server) IsInitialized() bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.initialized
}

// Log adds a formatted log message to the buffer (thread-safe, like bootstrap's logf)
// This is the primary logging method for orchestrator to record initialization progress.
func (s *Server) Log(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	// Also print to stdout
	fmt.Println(msg)

	s.logMu.Lock()
	defer s.logMu.Unlock()

	s.logBuffer = append(s.logBuffer, msg)

	// Trim to max size if needed
	maxSize := int(atomic.LoadInt32(&s.logSize))
	if len(s.logBuffer) > maxSize {
		// Keep only the most recent maxSize entries
		s.logBuffer = s.logBuffer[len(s.logBuffer)-maxSize:]
	}
}

// GetLogBuffer returns the accumulated log as a single string (thread-safe)
// Like bootstrap's currentLog() - returns all log lines joined by newlines.
func (s *Server) GetLogBuffer() string {
	s.logMu.RLock()
	defer s.logMu.RUnlock()

	if len(s.logBuffer) == 0 {
		return "(no logs yet)"
	}

	// Join with newlines and ensure trailing newline
	var sb strings.Builder
	for _, line := range s.logBuffer {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}

// SetLogSize sets the maximum number of log lines to keep (thread-safe)
// Default is 1000. Use 0 for unlimited (not recommended for long-running processes).
func (s *Server) SetLogSize(size int32) {
	atomic.StoreInt32(&s.logSize, size)
}

// GetLogSize returns the current maximum log buffer size.
func (s *Server) GetLogSize() int32 {
	return atomic.LoadInt32(&s.logSize)
}

// GetLogCount returns the current number of log lines in the buffer.
func (s *Server) GetLogCount() int {
	s.logMu.RLock()
	defer s.logMu.RUnlock()
	return len(s.logBuffer)
}

// ClearLog clears the log buffer (thread-safe).
func (s *Server) ClearLog() {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	s.logBuffer = make([]string, 0, 1000)
}
