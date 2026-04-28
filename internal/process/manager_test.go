package process

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/0gfoundation/agent-wrapper/internal/config"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager(nil)

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.maxRestarts != 3 {
		t.Errorf("expected maxRestarts 3, got %d", mgr.maxRestarts)
	}

	customCfg := &Config{
		MaxRestarts:  5,
		RestartDelay: 10 * time.Second,
	}
	mgr = NewManager(customCfg)

	if mgr.maxRestarts != 5 {
		t.Errorf("expected maxRestarts 5, got %d", mgr.maxRestarts)
	}
}

func TestParseEntryPoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "simple command",
			input: "python3 main.py",
			want:  []string{"python3", "main.py"},
		},
		{
			name:  "command with flags",
			input: "python3 -u main.py --port 9000",
			want:  []string{"python3", "-u", "main.py", "--port", "9000"},
		},
		{
			name:  "command with quotes",
			input: `python3 main.py --name "my agent"`,
			want:  []string{"python3", "main.py", "--name", "my agent"},
		},
		{
			name:  "node with path",
			input: "node /app/index.js",
			want:  []string{"node", "/app/index.js"},
		},
		{
			name:    "empty entry point",
			input:   "",
			wantErr: true,
		},
		{
			name:  "single word",
			input: "python3",
			want:  []string{"python3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEntryPoint(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseEntryPoint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("parseEntryPoint() got %d parts, want %d", len(got), len(tt.want))
				}
				for i := range tt.want {
					if i < len(got) && got[i] != tt.want[i] {
						t.Errorf("parseEntryPoint()[%d] = %v, want %v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestManager_Start_InvalidCommand(t *testing.T) {
	mgr := NewManager(nil)
	ctx := context.Background()

	agentConfig := &config.AgentConfig{
		Runtime: &config.Runtime{
			EntryPoint: "nonexistent-command-xyz-123",
			AgentPort:  9000,
		},
	}

	err := mgr.Start(ctx, agentConfig)
	if err == nil {
		t.Error("expected error for invalid command")
	}
}

func TestManager_Start_SimpleCommand(t *testing.T) {
	mgr := NewManager(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a simple echo script
	tmpFile := "/tmp/test-echo-" + time.Now().Format("20060102150405") + ".sh"
	script := "#!/bin/sh\necho 'agent started'\nsleep 10\n"

	if err := os.WriteFile(tmpFile, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	defer os.Remove(tmpFile)

	agentConfig := &config.AgentConfig{
		Runtime: &config.Runtime{
			EntryPoint: tmpFile,
			AgentPort:  9000,
		},
		Env: map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	if err := mgr.Start(ctx, agentConfig); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	if !mgr.IsRunning() {
		t.Error("process should be running")
	}

	pid := mgr.GetPID()
	if pid == 0 {
		t.Error("expected non-zero PID")
	}

	// Stop the process
	if err := mgr.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Wait for it to actually stop
	time.Sleep(200 * time.Millisecond)

	if mgr.IsRunning() {
		t.Error("process should be stopped")
	}
}

func TestManager_Stop_NotRunning(t *testing.T) {
	mgr := NewManager(nil)

	// Stop when not running should not error
	if err := mgr.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestManager_Status(t *testing.T) {
	mgr := NewManager(nil)

	status := mgr.Status()
	if status.Running {
		t.Error("should not be running initially")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpFile := "/tmp/test-status-" + time.Now().Format("20060102150405") + ".sh"
	script := "#!/bin/sh\nsleep 5\n"
	os.WriteFile(tmpFile, []byte(script), 0755)
	defer os.Remove(tmpFile)

	agentConfig := &config.AgentConfig{
		Runtime: &config.Runtime{
			EntryPoint: tmpFile,
			AgentPort:  9000,
		},
	}

	mgr.Start(ctx, agentConfig)
	time.Sleep(100 * time.Millisecond)

	status = mgr.Status()
	if !status.Running {
		t.Error("should be running after start")
	}
	if status.PID == 0 {
		t.Error("expected non-zero PID")
	}

	mgr.Stop()
}

func TestManager_Signal(t *testing.T) {
	mgr := NewManager(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpFile := "/tmp/test-signal-" + time.Now().Format("20060102150405") + ".sh"
	// Simple sleep script
	script := `#!/bin/sh
sleep 30
`
	os.WriteFile(tmpFile, []byte(script), 0755)
	defer os.Remove(tmpFile)

	agentConfig := &config.AgentConfig{
		Runtime: &config.Runtime{
			EntryPoint: tmpFile,
			AgentPort:  9000,
		},
	}

	if err := mgr.Start(ctx, agentConfig); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Mark as stopped to prevent restart before signaling
	mgr.mu.Lock()
	mgr.stopped = true
	mgr.mu.Unlock()

	// Send SIGTERM - should not error
	if err := mgr.Signal(syscall.SIGTERM); err != nil {
		t.Errorf("Signal failed: %v", err)
	}

	// Use Stop to ensure clean shutdown instead of relying on signal behavior
	// (signal traps don't work consistently across platforms)
	mgr.Stop()

	// Wait a moment for cleanup
	time.Sleep(200 * time.Millisecond)

	if mgr.IsRunning() {
		t.Error("process should have stopped")
	}
}

func TestManager_GetPID_NotRunning(t *testing.T) {
	mgr := NewManager(nil)

	pid := mgr.GetPID()
	if pid != 0 {
		t.Errorf("expected PID 0 when not running, got %d", pid)
	}
}

func TestManager_AddOutputWriter(t *testing.T) {
	mgr := NewManager(nil)

	// Just ensure it doesn't panic
	mgr.AddOutputWriter(nil)
}

func TestConfig_Defaults(t *testing.T) {
	// nil config should use defaults
	mgr := NewManager(nil)

	if mgr.maxRestarts != 3 {
		t.Errorf("expected default maxRestarts 3, got %d", mgr.maxRestarts)
	}
}

func TestManager_DoubleStart(t *testing.T) {
	mgr := NewManager(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpFile := "/tmp/test-double-" + time.Now().Format("20060102150405") + ".sh"
	script := "#!/bin/sh\nsleep 5\n"
	os.WriteFile(tmpFile, []byte(script), 0755)
	defer os.Remove(tmpFile)

	agentConfig := &config.AgentConfig{
		Runtime: &config.Runtime{
			EntryPoint: tmpFile,
			AgentPort:  9000,
		},
	}

	if err := mgr.Start(ctx, agentConfig); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Second start should fail
	err := mgr.Start(ctx, agentConfig)
	if err == nil {
		t.Error("expected error on second start")
	}

	mgr.Stop()
}
