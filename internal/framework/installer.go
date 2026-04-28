// Package framework provides dynamic framework installation support
package framework

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// FrameworkType represents the type of framework (Python, Node.js, Custom)
type FrameworkType int

const (
	FrameworkPython FrameworkType = iota
	FrameworkNodeJS
	FrameworkCustom
)

// Config holds configuration for framework installation
type Config struct {
	// PythonCmd is the Python command to use (default: python3)
	PythonCmd string

	// NPMCmd is the npm command to use (default: npm)
	NPMCmd string

	// TimeoutSeconds is the installation timeout (default: 300)
	TimeoutSeconds int
}

// Installer handles framework installation
type Installer struct {
	config *Config
}

// NewInstaller creates a new framework installer
func NewInstaller(cfg *Config) *Installer {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.PythonCmd == "" {
		cfg.PythonCmd = "python3"
	}
	if cfg.NPMCmd == "" {
		cfg.NPMCmd = "npm"
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = 300
	}

	return &Installer{
		config: cfg,
	}
}

// InstallResult represents the result of an installation
type InstallResult struct {
	Success   bool
	Framework string
	Version   string
	Duration  int64
	Error     string
}

// Install installs a framework with the given version
func (i *Installer) Install(ctx context.Context, framework, version string) (*InstallResult, error) {
	startTime := time.Now()

	// Detect framework type
	fwType, err := DetectFramework(framework)
	if err != nil {
		return &InstallResult{
			Success:   false,
			Framework: framework,
			Version:   version,
			Error:     err.Error(),
		}, err
	}

	// Get installer for framework type
	installer := GetInstaller(fwType)

	// If custom installer, need install script
	if fwType == FrameworkCustom {
		return &InstallResult{
			Success:   false,
			Framework: framework,
			Version:   version,
			Error:     "custom framework requires install script",
		}, fmt.Errorf("custom framework not implemented")
	}

	// Get install command
	cmd := installer.InstallCommand(framework, version)
	if cmd == nil {
		return &InstallResult{
			Success:   false,
			Framework: framework,
			Version:   version,
			Error:     "no install command generated",
		}, fmt.Errorf("failed to generate install command")
	}

	// Create context with timeout
	timeout := time.Duration(i.config.TimeoutSeconds) * time.Second
	installCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute installation
	command := exec.CommandContext(installCtx, cmd[0], cmd[1:]...)
	output, err := command.CombinedOutput()

	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		return &InstallResult{
			Success:   false,
			Framework: framework,
			Version:   version,
			Duration:  duration,
			Error:     formatInstallError(err, output),
		}, err
	}

	return &InstallResult{
		Success:   true,
		Framework: framework,
		Version:   version,
		Duration:  duration,
	}, nil
}

// DetectFramework detects the framework type from the framework name
func DetectFramework(framework string) (FrameworkType, error) {
	if framework == "" {
		return FrameworkCustom, fmt.Errorf("empty framework name")
	}

	// Normalize to lowercase
	fwLower := strings.ToLower(framework)

	// Known Python frameworks
	pythonFrameworks := []string{"openclaw", "autogen", "langroid", "crewai", "pip", "langchain"}
	for _, fw := range pythonFrameworks {
		if strings.Contains(fwLower, fw) {
			return FrameworkPython, nil
		}
	}

	// Known Node.js frameworks
	nodeFrameworks := []string{"eliza", "near-agent", "terminal-gpt", "typescript"}
	for _, fw := range nodeFrameworks {
		if strings.Contains(fwLower, fw) {
			return FrameworkNodeJS, nil
		}
	}

	// Check if it's "custom"
	if fwLower == "custom" {
		return FrameworkCustom, nil
	}

	return FrameworkCustom, fmt.Errorf("unknown framework: %s", framework)
}

// FrameworkInstaller is the interface for framework installers
type FrameworkInstaller interface {
	InstallCommand(framework, version string) []string
}

// PythonInstaller installs Python frameworks
type PythonInstaller struct{}

// InstallCommand returns the pip install command
func (p *PythonInstaller) InstallCommand(framework, version string) []string {
	if version != "" {
		return []string{"python3", "-m", "pip", "install", fmt.Sprintf("%s==%s", framework, version)}
	}
	return []string{"python3", "-m", "pip", "install", framework}
}

// NodeJSInstaller installs Node.js frameworks
type NodeJSInstaller struct{}

// InstallCommand returns the npm install command
func (n *NodeJSInstaller) InstallCommand(framework, version string) []string {
	// Convert framework name to npm package name
	pkgName := frameworkToNpmPackage(framework)

	if version != "" {
		return []string{"npm", "install", fmt.Sprintf("%s@%s", pkgName, version)}
	}
	return []string{"npm", "install", pkgName}
}

// CustomInstaller handles custom frameworks
type CustomInstaller struct{}

// InstallCommand returns nil for custom frameworks
func (c *CustomInstaller) InstallCommand(framework, version string) []string {
	return nil
}

// GetInstaller returns the installer for a framework type
func GetInstaller(fwType FrameworkType) FrameworkInstaller {
	switch fwType {
	case FrameworkPython:
		return &PythonInstaller{}
	case FrameworkNodeJS:
		return &NodeJSInstaller{}
	case FrameworkCustom:
		return &CustomInstaller{}
	default:
		return &CustomInstaller{}
	}
}

// frameworkToNpmPackage converts a framework name to its npm package name
func frameworkToNpmPackage(framework string) string {
	fwLower := strings.ToLower(framework)

	// Known mappings
	packages := map[string]string{
		"eliza":   "@eliza/core",
		"near-agent": "near-agent",
	}

	if pkg, ok := packages[fwLower]; ok {
		return pkg
	}

	// Default: use @<framework>/core pattern
	return fmt.Sprintf("@%s/core", fwLower)
}

// ValidateFramework validates a framework name
func ValidateFramework(framework string) error {
	_, err := DetectFramework(framework)
	return err
}

// ValidateVersion validates a version string
func ValidateVersion(version string) error {
	if version == "" {
		return nil // Empty is valid (use latest)
	}
	// Basic semantic version validation
	// Versions like "1.2.3", "v1.2.3", "1.2.3-beta" are all valid
	if strings.HasPrefix(version, "v") {
		version = version[1:]
	}
	// Just check it's not empty and has some structure
	if len(version) == 0 {
		return fmt.Errorf("version cannot be empty")
	}
	return nil
}

// IsPythonAvailable checks if Python is available
func IsPythonAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// IsNPMAvailable checks if npm is available
func IsNPMAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// formatInstallError formats an installation error
func formatInstallError(err error, output []byte) string {
	if len(output) > 0 {
		return fmt.Sprintf("%s: %s", err, string(output))
	}
	return err.Error()
}
