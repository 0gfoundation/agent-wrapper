// Package process provides agent process management
// Handles starting, monitoring, and restarting agent processes
package process

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/0gfoundation/agent-wrapper/internal/config"
)

// Manager manages agent processes
type Manager struct {
	mu            sync.RWMutex
	cmd           *exec.Cmd
	config        *config.AgentConfig
	restartCount  int
	maxRestarts   int
	running       bool
	stopped       bool
	cancel        context.CancelFunc
	outputWriters []io.Writer
}

// Config holds configuration for the process manager
type Config struct {
	MaxRestarts    int           // Maximum number of restarts (0 = no limit)
	RestartDelay   time.Duration // Delay between restart attempts
	StopTimeout    time.Duration // Timeout for graceful shutdown
	StartTimeout   time.Duration // Timeout for process start
}

// NewManager creates a new process manager
func NewManager(cfg *Config) *Manager {
	if cfg == nil {
		cfg = &Config{
			MaxRestarts:  3,
			RestartDelay: 5 * time.Second,
			StopTimeout:  10 * time.Second,
			StartTimeout: 30 * time.Second,
		}
	}

	return &Manager{
		maxRestarts: cfg.MaxRestarts,
	}
}

// Status represents the process status
type Status struct {
	PID         int       `json:"pid"`
	Running     bool      `json:"running"`
	RestartCount int      `json:"restartCount"`
	Uptime      int64     `json:"uptime"`        // seconds
	StartTime   time.Time `json:"startTime"`
	ExitTime    time.Time `json:"exitTime,omitempty"`
	ExitError   string    `json:"exitError,omitempty"`
}

// Start starts the agent process with the given configuration
func (m *Manager) Start(ctx context.Context, agentConfig *config.AgentConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("process already running")
	}

	m.config = agentConfig
	m.stopped = false

	// Create context with timeout for start
	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	m.cancel = cancel

	// Parse entry point
	entryPoint := agentConfig.Runtime.EntryPoint
	cmdParts, err := parseEntryPoint(entryPoint)
	if err != nil {
		return fmt.Errorf("invalid entry point: %w", err)
	}

	// Create command
	cmd := exec.CommandContext(startCtx, cmdParts[0], cmdParts[1:]...)

	// Set working directory
	if agentConfig.Runtime.WorkingDir != "" {
		cmd.Dir = agentConfig.Runtime.WorkingDir
	}

	// Set up environment
	env := os.Environ()
	// Add agent port
	env = append(env, fmt.Sprintf("AGENT_PORT=%s", agentConfig.Runtime.GetAgentPort()))
	// Add custom env vars
	for k, v := range agentConfig.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	// Add inference config as env vars
	env = append(env, inferenceEnvVars(agentConfig.Inference)...)
	cmd.Env = env

	// Set up pipes for stdout/stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	m.cmd = cmd
	m.running = true

	// Start output readers
	go m.readOutput(stdout, "stdout")
	go m.readOutput(stderr, "stderr")

	// Wait a bit to ensure it started
	select {
	case <-time.After(500 * time.Millisecond):
	case <-startCtx.Done():
		return fmt.Errorf("start timeout")
	}

	// Check if process is still running
	if cmd.Process == nil {
		m.running = false
		return fmt.Errorf("process did not start")
	}

	log.Printf("Agent process started with PID %d", cmd.Process.Pid)

	// Start monitoring
	go m.monitor(startCtx)

	return nil
}

// monitor monitors the process and restarts if needed
func (m *Manager) monitor(ctx context.Context) {
	for {
		m.mu.RLock()
		cmd := m.cmd
		stopped := m.stopped
		m.mu.RUnlock()

		if stopped || cmd == nil {
			return
		}

		// Wait for process to exit
		err := cmd.Wait()

		m.mu.Lock()
		m.running = false
		exitErr := fmt.Sprintf("process exited: %v", err)
		shouldRestart := !m.stopped && (m.maxRestarts == 0 || m.restartCount < m.maxRestarts)
		m.mu.Unlock()

		log.Printf("Agent process %d exited: %v", cmd.Process.Pid, err)

		if !shouldRestart {
			return
		}

		// Restart after delay
		time.Sleep(5 * time.Second)

		m.mu.Lock()
		m.restartCount++

		if m.maxRestarts > 0 && m.restartCount >= m.maxRestarts {
			log.Printf("Max restarts (%d) reached, giving up", m.maxRestarts)
			m.mu.Unlock()
			return
		}

		log.Printf("Restarting agent (attempt %d/%d)", m.restartCount+1, m.maxRestarts)

		// Create new command
		entryPoint := m.config.Runtime.EntryPoint
		cmdParts, _ := parseEntryPoint(entryPoint)
		newCmd := exec.Command(cmdParts[0], cmdParts[1:]...)

		if m.config.Runtime.WorkingDir != "" {
			newCmd.Dir = m.config.Runtime.WorkingDir
		}

		// Set up environment
		env := os.Environ()
		env = append(env, fmt.Sprintf("AGENT_PORT=%s", m.config.Runtime.GetAgentPort()))
		for k, v := range m.config.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		// Add inference config as env vars
		env = append(env, inferenceEnvVars(m.config.Inference)...)
		newCmd.Env = env

		// Set up output pipes
		stdout, _ := newCmd.StdoutPipe()
		stderr, _ := newCmd.StderrPipe()

		if err := newCmd.Start(); err != nil {
			log.Printf("Failed to restart agent: %v", err)
			m.mu.Unlock()
			return
		}

		m.cmd = newCmd
		m.running = true
		m.mu.Unlock()

		// Start output readers for new process
		go m.readOutput(stdout, "stdout")
		go m.readOutput(stderr, "stderr")

		log.Printf("Agent restarted with PID %d", newCmd.Process.Pid)

		if exitErr != "" {
			log.Printf("Previous exit: %s", exitErr)
		}
	}
}

// readOutput reads and logs process output
func (m *Manager) readOutput(r io.Reader, name string) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			log.Printf("[agent %s] %s", name, string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

// Stop stops the agent process gracefully
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.stopped = true

	if m.cancel != nil {
		m.cancel()
	}

	if m.cmd != nil && m.cmd.Process != nil {
		log.Printf("Stopping agent process %d...", m.cmd.Process.Pid)

		// Try graceful shutdown first
		m.cmd.Process.Signal(syscall.SIGTERM)

		// Wait for graceful shutdown
		done := make(chan error, 1)
		go func() {
			_, err := m.cmd.Process.Wait()
			done <- err
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(10 * time.Second):
			// Force kill if timeout
			log.Printf("Agent did not stop gracefully, killing...")
			m.cmd.Process.Kill()
		}
	}

	m.running = false
	m.cmd = nil

	return nil
}

// Status returns the current status of the process
func (m *Manager) Status() *Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := &Status{
		Running:      m.running,
		RestartCount: m.restartCount,
	}

	if m.cmd != nil && m.cmd.Process != nil {
		status.PID = m.cmd.Process.Pid
	}

	return status
}

// IsRunning returns whether the process is currently running
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetPID returns the process ID if running
func (m *Manager) GetPID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Pid
	}
	return 0
}

// AddOutputWriter adds a writer for process output
func (m *Manager) AddOutputWriter(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputWriters = append(m.outputWriters, w)
}

// Signal sends a signal to the process
func (m *Manager) Signal(sig syscall.Signal) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.cmd == nil || m.cmd.Process == nil {
		return fmt.Errorf("process not running")
	}

	return m.cmd.Process.Signal(sig)
}

// parseEntryPoint parses the entry point command into command and args
func parseEntryPoint(entryPoint string) ([]string, error) {
	if entryPoint == "" {
		return nil, fmt.Errorf("entry point is empty")
	}

	// Simple shell-like parsing
	// Split by spaces, but respect quotes
	var parts []string
	var current string
	var inQuotes bool
	var quoteChar rune

	for _, r := range entryPoint {
		switch {
		case r == '\'' || r == '"':
			if !inQuotes {
				inQuotes = true
				quoteChar = r
			} else if r == quoteChar {
				inQuotes = false
				quoteChar = 0
			} else {
				current += string(r)
			}
		case r == ' ' && !inQuotes:
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		default:
			current += string(r)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	if len(parts) == 0 {
		return nil, fmt.Errorf("no command found in entry point")
	}

	return parts, nil
}

// inferenceEnvVars converts Inference config to environment variables
func inferenceEnvVars(inference *config.Inference) []string {
	if inference == nil {
		return nil
	}

	var env []string

	if inference.Provider != "" {
		env = append(env, fmt.Sprintf("INFERENCE_PROVIDER=%s", inference.Provider))
	}
	if inference.Model != "" {
		env = append(env, fmt.Sprintf("INFERENCE_MODEL=%s", inference.Model))
	}
	if inference.Endpoint != "" {
		env = append(env, fmt.Sprintf("INFERENCE_ENDPOINT=%s", inference.Endpoint))
	}
	if inference.APIKey != "" {
		env = append(env, fmt.Sprintf("INFERENCE_API_KEY=%s", inference.APIKey))
	}

	return env
}
