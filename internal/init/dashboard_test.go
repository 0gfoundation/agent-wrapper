package init

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDashboard_LogAndRetrieve(t *testing.T) {
	server := New()

	// Simulate log messages like orchestrator would
	server.Log("Starting initialization flow...")
	server.Log("Step 1: Waiting for HTTP initialization...")
	time.Sleep(10 * time.Millisecond)
	server.Log("Step 2: Generating key pair...")
	server.Log("Public key generated: 0xabc123...")
	server.Log("Seal ID: 0xdef456...")
	server.Log("Initialization flow complete. Agent ready.")

	// Check log count
	count := server.GetLogCount()
	if count != 6 {
		t.Errorf("Expected 6 log lines, got %d", count)
	}

	// Test dashboard endpoint
	req := httptest.NewRequest("GET", "/_internal/dashboard", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	// Check status
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check content type
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Expected text/plain content type, got %s", ct)
	}

	// Check body contains our log messages
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Starting initialization flow") {
		t.Error("Dashboard body missing 'Starting initialization flow'")
	}
	if !strings.Contains(bodyStr, "Initialization flow complete") {
		t.Error("Dashboard body missing 'Initialization flow complete'")
	}
	if !strings.Contains(bodyStr, "0xabc123") {
		t.Error("Dashboard body missing '0xabc123'")
	}

	t.Logf("Dashboard output:\n%s", bodyStr)
}

func TestDashboard_EmptyBuffer(t *testing.T) {
	server := New()

	// Test dashboard with no logs
	req := httptest.NewRequest("GET", "/_internal/dashboard", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "no logs yet") {
		t.Errorf("Expected 'no logs yet' message, got: %s", bodyStr)
	}
}

func TestDashboard_LogSizeLimit(t *testing.T) {
	server := New()
	server.SetLogSize(5) // Set max 5 lines

	// Log more than the limit
	for i := 0; i < 10; i++ {
		server.Log("Log line %d", i)
	}

	// Should only keep last 5
	count := server.GetLogCount()
	if count != 5 {
		t.Errorf("Expected 5 log lines (limited), got %d", count)
	}

	// Check dashboard output
	req := httptest.NewRequest("GET", "/_internal/dashboard", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Should NOT contain "Log line 0" (trimmed)
	if strings.Contains(bodyStr, "Log line 0") {
		t.Error("Old logs should be trimmed")
	}

	// Should contain "Log line 5" (first kept)
	if !strings.Contains(bodyStr, "Log line 5") {
		t.Error("Expected 'Log line 5' in buffer")
	}

	// Should contain "Log line 9" (last)
	if !strings.Contains(bodyStr, "Log line 9") {
		t.Error("Expected 'Log line 9' in buffer")
	}
}

func TestDashboard_Clear(t *testing.T) {
	server := New()

	server.Log("Line 1")
	server.Log("Line 2")

	if server.GetLogCount() != 2 {
		t.Errorf("Expected 2 log lines, got %d", server.GetLogCount())
	}

	server.ClearLog()

	if server.GetLogCount() != 0 {
		t.Errorf("Expected 0 log lines after clear, got %d", server.GetLogCount())
	}

	// Dashboard should show "no logs yet" after clear
	req := httptest.NewRequest("GET", "/_internal/dashboard", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "no logs yet") {
		t.Errorf("Expected 'no logs yet' after clear, got: %s", bodyStr)
	}
}
