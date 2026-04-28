// Package config provides configuration management including
// encryption, decryption, and validation.
package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidKeySize is returned when the key size is invalid
	ErrInvalidKeySize = errors.New("invalid key size: must be 32 bytes")
	// ErrInvalidData is returned when the encrypted data is invalid
	ErrInvalidData = errors.New("invalid encrypted data")
	// ErrMissingEntryPoint is returned when entry point is missing
	ErrMissingEntryPoint = errors.New("entryPoint is required")
	// ErrInvalidMemory is returned when memory value is invalid
	ErrInvalidMemory = errors.New("memoryMB must be greater than 0")
)

// ManagerConfig holds configuration for the config manager
type ManagerConfig struct {
	// StorageEndpoint is the 0G Storage endpoint
	StorageEndpoint string
}

// Manager handles config encryption/decryption
type Manager struct {
	config *ManagerConfig
}

// NewManager creates a new config manager
func NewManager(cfg *ManagerConfig) *Manager {
	if cfg == nil {
		cfg = &ManagerConfig{}
	}
	return &Manager{
		config: cfg,
	}
}

// AgentConfig represents the agent configuration stored in encrypted Storage
// This contains all information needed to run the agent that is NOT on-chain
type AgentConfig struct {
	Framework  *Framework              `json:"framework,omitempty"`
	Runtime    *Runtime                `json:"runtime,omitempty"`
	Inference  *Inference              `json:"inference,omitempty"`
	Persona    *Persona                `json:"persona,omitempty"`
	Env        map[string]string       `json:"env,omitempty"`
	Custom     map[string]interface{}  `json:"custom,omitempty"` // For future extensions
}

// Framework represents the framework configuration
type Framework struct {
	Name    string `json:"name"`              // e.g., "openclaw", "eliza"
	Version string `json:"version,omitempty"` // e.g., "0.1.0"
}

// Runtime represents runtime configuration
type Runtime struct {
	EntryPoint string `json:"entryPoint"`          // Command to start agent, e.g., "python3 main.py"
	WorkingDir string `json:"workingDir,omitempty"` // Working directory, e.g., "/app"
	AgentPort  int    `json:"agentPort,omitempty"`  // Port agent listens on, e.g., 9000
}

// Inference represents inference/compute configuration
type Inference struct {
	Provider string `json:"provider,omitempty"` // e.g., "0g-compute", "openai", "anthropic"
	Model    string `json:"model,omitempty"`    // e.g., "glm-4", "gpt-4", "claude-3"
	Endpoint string `json:"endpoint,omitempty"` // Custom endpoint URL
	APIKey   string `json:"apiKey,omitempty"`   // API key for the provider (encrypted)
}

// Persona represents the agent persona/configuration
type Persona struct {
	Name         string            `json:"name,omitempty"`         // Persona name
	SystemPrompt string            `json:"system_prompt,omitempty"` // System prompt for the agent
	Description  string            `json:"description,omitempty"`  // Description of the persona
	Settings     map[string]string `json:"settings,omitempty"`     // Additional persona settings
}

// Resources represents resource requirements (managed by 0g-sandbox, not in config)
// This is kept for backward compatibility but should not be in the encrypted config
type Resources struct {
	MemoryMB  int    `json:"memoryMB,omitempty"`
	CPU       int    `json:"cpu,omitempty"`
	AgentPort string `json:"agentPort,omitempty"` // Deprecated: Use Runtime.AgentPort instead
}

// SetDefaults sets default values for runtime
func (r *Runtime) SetDefaults() {
	if r.AgentPort == 0 {
		r.AgentPort = 9000 // Default port
	}
	if r.WorkingDir == "" {
		r.WorkingDir = "/app" // Default working directory
	}
}

// GetAgentPort returns the agent port as a string
func (r *Runtime) GetAgentPort() string {
	if r.AgentPort == 0 {
		return "9000"
	}
	return fmt.Sprintf("%d", r.AgentPort)
}

// DecryptConfig decrypts an encrypted configuration
func (m *Manager) DecryptConfig(encryptedData, key []byte) (*AgentConfig, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeySize
	}
	if len(encryptedData) == 0 {
		return nil, ErrInvalidData
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Check data length (nonce + ciphertext + tag)
	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, ErrInvalidData
	}

	// Extract nonce
	nonce := encryptedData[:nonceSize]
	ciphertext := encryptedData[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	// Parse JSON
	var config AgentConfig
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults for runtime
	if config.Runtime != nil {
		config.Runtime.SetDefaults()
	}

	return &config, nil
}

// EncryptConfig encrypts a configuration
func (m *Manager) EncryptConfig(config *AgentConfig, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeySize
	}

	// Marshal to JSON
	plaintext, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random IV
	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt and prepend IV
	ciphertext := gcm.Seal(iv, iv, plaintext, nil)

	return ciphertext, nil
}

// ValidateConfig validates an agent configuration
func ValidateConfig(config *AgentConfig) error {
	if config.Runtime == nil {
		return errors.New("runtime is required")
	}
	if config.Runtime.EntryPoint == "" {
		return ErrMissingEntryPoint
	}
	if config.Framework != nil && config.Framework.Name == "" {
		return errors.New("framework name cannot be empty")
	}
	return nil
}

// ParseConfig parses JSON configuration data
func ParseConfig(jsonData []byte) (*AgentConfig, error) {
	var config AgentConfig
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults for runtime
	if config.Runtime != nil {
		config.Runtime.SetDefaults()
	}

	return &config, nil
}

// ConfigToJSON converts a config to JSON
func ConfigToJSON(config *AgentConfig) ([]byte, error) {
	jsonData, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	return jsonData, nil
}

// GenerateKey generates a random 256-bit encryption key
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

// ConfigHash generates a hash of the configuration for verification
func ConfigHash(config *AgentConfig) string {
	jsonData, err := json.Marshal(config)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

// KeyFromHex converts a hex string to a key
func KeyFromHex(hexKey string) ([]byte, error) {
	// Strip 0x prefix if present
	hexStr := hexKey
	if len(hexStr) >= 2 && hexStr[0:2] == "0x" {
		hexStr = hexStr[2:]
	}

	key, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}

	if len(key) != 32 {
		return nil, ErrInvalidKeySize
	}

	return key, nil
}

// KeyToHex converts a key to hex string
func KeyToHex(key []byte) string {
	return "0x" + hex.EncodeToString(key)
}

// GetCustomField retrieves a custom field value
func (c *AgentConfig) GetCustomField(key string) (interface{}, bool) {
	if c.Custom == nil {
		return nil, false
	}
	val, ok := c.Custom[key]
	return val, ok
}

// SetCustomField sets a custom field value
func (c *AgentConfig) SetCustomField(key string, value interface{}) {
	if c.Custom == nil {
		c.Custom = make(map[string]interface{})
	}
	c.Custom[key] = value
}

// Merge merges another config into this one (other takes precedence)
func (c *AgentConfig) Merge(other *AgentConfig) {
	if other == nil {
		return
	}
	if other.Framework != nil {
		c.Framework = other.Framework
	}
	if other.Runtime != nil {
		c.Runtime = other.Runtime
	}
	if other.Inference != nil {
		c.Inference = other.Inference
	}
	if other.Persona != nil {
		c.Persona = other.Persona
	}
	if other.Env != nil {
		if c.Env == nil {
			c.Env = make(map[string]string)
		}
		for k, v := range other.Env {
			c.Env[k] = v
		}
	}
	if other.Custom != nil {
		if c.Custom == nil {
			c.Custom = make(map[string]interface{})
		}
		for k, v := range other.Custom {
			c.Custom[k] = v
		}
	}
}
