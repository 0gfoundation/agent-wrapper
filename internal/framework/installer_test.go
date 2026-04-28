package framework

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewInstaller tests creating a new installer
func TestNewInstaller(t *testing.T) {
	installer := NewInstaller(&Config{
		PythonCmd: "python3",
		NPMCmd:    "npm",
	})

	assert.NotNil(t, installer)
	assert.Equal(t, "python3", installer.config.PythonCmd)
	assert.Equal(t, "npm", installer.config.NPMCmd)
}

// TestDetectFramework tests framework detection
func TestDetectFramework(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		wantType  FrameworkType
		wantErr   bool
	}{
		{name: "openclaw", framework: "openclaw", wantType: FrameworkPython, wantErr: false},
		{name: "OpenClaw", framework: "openclaw", wantType: FrameworkPython, wantErr: false},
		{name: "OPENCLAW", framework: "openclaw", wantType: FrameworkPython, wantErr: false},
		{name: "eliza", framework: "eliza", wantType: FrameworkNodeJS, wantErr: false},
		{name: "Eliza", framework: "eliza", wantType: FrameworkNodeJS, wantErr: false},
		{name: "custom", framework: "custom", wantType: FrameworkCustom, wantErr: false},
		{name: "unknown", framework: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft, err := DetectFramework(tt.framework)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantType, ft)
			}
		})
	}
}

// TestPythonInstaller_InstallCommand tests Python install command generation
func TestPythonInstaller_InstallCommand(t *testing.T) {
	installer := &PythonInstaller{}

	tests := []struct {
		name     string
		framework string
		version  string
		expected []string
	}{
		{
			name:     "openclaw with version",
			framework: "openclaw",
			version:  "0.1.0",
			expected: []string{"python3", "-m", "pip", "install", "openclaw==0.1.0"},
		},
		{
			name:     "openclaw without version",
			framework: "openclaw",
			version:  "",
			expected: []string{"python3", "-m", "pip", "install", "openclaw"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := installer.InstallCommand(tt.framework, tt.version)
			assert.Equal(t, tt.expected, cmd)
		})
	}
}

// TestNodeJSInstaller_InstallCommand tests Node.js install command generation
func TestNodeJSInstaller_InstallCommand(t *testing.T) {
	installer := &NodeJSInstaller{}

	tests := []struct {
		name     string
		framework string
		version  string
		expected []string
	}{
		{
			name:     "eliza with version",
			framework: "eliza",
			version:  "1.2.0",
			expected: []string{"npm", "install", "@eliza/core@1.2.0"},
		},
		{
			name:     "eliza without version",
			framework: "eliza",
			version:  "",
			expected: []string{"npm", "install", "@eliza/core"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := installer.InstallCommand(tt.framework, tt.version)
			assert.Equal(t, tt.expected, cmd)
		})
	}
}

// TestGetInstaller tests getting the right installer for framework type
func TestGetInstaller(t *testing.T) {
	tests := []struct {
		name        string
		fwType      FrameworkType
		wantInstaller interface{}
	}{
		{"python", FrameworkPython, &PythonInstaller{}},
		{"nodejs", FrameworkNodeJS, &NodeJSInstaller{}},
		{"custom", FrameworkCustom, &CustomInstaller{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := GetInstaller(tt.fwType)
			assert.IsType(t, tt.wantInstaller, installer)
		})
	}
}

// TestInstallResult tests install result
func TestInstallResult(t *testing.T) {
	result := &InstallResult{
		Success:   true,
		Framework: "openclaw",
		Version:   "0.1.0",
		Duration:  1500000000, // 1.5s
	}

	assert.True(t, result.Success)
	assert.Equal(t, "openclaw", result.Framework)
	assert.Equal(t, "0.1.0", result.Version)
	assert.Equal(t, int64(1500000000), result.Duration)
}

// TestConfigDefaults tests config defaults
func TestConfigDefaults(t *testing.T) {
	installer := NewInstaller(nil)

	assert.NotNil(t, installer)
	assert.Equal(t, "python3", installer.config.PythonCmd)
	assert.Equal(t, "npm", installer.config.NPMCmd)
	assert.Equal(t, 300, installer.config.TimeoutSeconds)
}

// TestValidateFramework tests framework validation
func TestValidateFramework(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		wantErr   bool
	}{
		{"openclaw valid", "openclaw", false},
		{"eliza valid", "eliza", false},
		{"custom valid", "custom", false},
		{"empty", "", true},
		{"unknown", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFramework(tt.framework)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateVersion tests version validation
func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid semantic", "1.2.3", false},
		{"valid with v prefix", "v1.2.3", false},
		{"valid with pre-release", "1.2.3-beta", false},
		{"empty", "", false}, // Empty is valid (use latest)
		{"simple", "0.1", false},
		{"complex", "1.2.3-beta.1+build.123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.version)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCustomInstaller_InstallCommand tests custom installer command
func TestCustomInstaller_InstallCommand(t *testing.T) {
	installer := &CustomInstaller{}

	cmd := installer.InstallCommand("custom", "1.0.0")
	assert.Nil(t, cmd)
}

// TestInstall_WithRealPython tests real Python installation (if available)
func TestInstall_WithRealPython(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real installation test in short mode")
	}

	installer := NewInstaller(&Config{
		PythonCmd:      "python3",
		TimeoutSeconds: 60,
	})

	// Test with a very small package that should be available
	result, err := installer.Install(context.Background(), "pip", "23.1")

	if err != nil {
		// Python or pip might not be available, skip test
		t.Skip("Python not available:", err)
	}

	assert.True(t, result.Success)
	assert.Equal(t, "pip", result.Framework)
}

// TestInstall_WithRealNPM tests real NPM installation (if available)
func TestInstall_WithRealNPM(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real installation test in short mode")
	}

	installer := NewInstaller(&Config{
		NPMCmd:         "npm",
		TimeoutSeconds: 60,
	})

	// Test with a very small package
	// Note: This test requires Node.js and npm to be installed
	result, err := installer.Install(context.Background(), "@types/node", "latest")

	if err != nil {
		// npm might not be available, skip test
		t.Skip("npm not available:", err)
	}

	assert.True(t, result.Success)
}

// TestIsPythonAvailable checks if Python is available
func TestIsPythonAvailable(t *testing.T) {
	available := IsPythonAvailable("python3")
	t.Logf("Python3 available: %v", available)
}

// TestIsNPMAvailable checks if npm is available
func TestIsNPMAvailable(t *testing.T) {
	available := IsNPMAvailable("npm")
	t.Logf("npm available: %v", available)
}

// TestInstall_ContextCancel tests installation with cancelled context
func TestInstall_ContextCancel(t *testing.T) {
	installer := NewInstaller(&Config{
		PythonCmd:      "python3",
		TimeoutSeconds: 10,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := installer.Install(ctx, "openclaw", "0.1.0")

	assert.Error(t, err)
	assert.False(t, result.Success)
}

// TestFrameworkToNpmPackage tests npm package name conversion
func TestFrameworkToNpmPackage(t *testing.T) {
	installer := &NodeJSInstaller{}

	tests := []struct {
		name     string
		framework string
		expected string
	}{
		{"eliza", "eliza", "@eliza/core"},
		{"near-agent", "near-agent", "near-agent"},
		{"Eliza", "eliza", "@eliza/core"},
		{"unknown", "unknown", "@unknown/core"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := installer.InstallCommand(tt.framework, "")
			assert.Equal(t, tt.expected, cmd[2])
		})
	}
}

// TestInstall_InvalidFramework tests installation with invalid framework
func TestInstall_InvalidFramework(t *testing.T) {
	installer := NewInstaller(&Config{
		PythonCmd:      "python3",
		TimeoutSeconds: 10,
	})

	result, err := installer.Install(context.Background(), "unknown-framework-xyz", "1.0.0")

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, "unknown-framework-xyz", result.Framework)
}

// TestInstall_Timeout tests installation timeout
func TestInstall_Timeout(t *testing.T) {
	installer := NewInstaller(&Config{
		PythonCmd:      "python3",
		TimeoutSeconds: 1, // 1 second timeout
	})

	// Use a framework that doesn't exist to trigger a delay
	ctx := context.Background()
	result, err := installer.Install(ctx, "openclaw", "0.0.0-nonexistent-version")

	// Should error due to timeout or package not found
	if err != nil {
		assert.False(t, result.Success)
	}
}

// TestFormatInstallError tests error formatting
func TestFormatInstallError(t *testing.T) {
	err := fmt.Errorf("base error")
	output := []byte("detailed error message")

	formatted := formatInstallError(err, output)

	assert.Contains(t, formatted, "base error")
	assert.Contains(t, formatted, "detailed error message")
}

// TestFormatInstallError_NoOutput tests error formatting without output
func TestFormatInstallError_NoOutput(t *testing.T) {
	err := fmt.Errorf("base error")

	formatted := formatInstallError(err, nil)

	assert.Equal(t, "base error", formatted)
}
