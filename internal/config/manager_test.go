package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewManager tests creating a new config manager
func TestNewManager(t *testing.T) {
	manager := NewManager(&ManagerConfig{
		StorageEndpoint: "https://storage.example.com",
	})

	assert.NotNil(t, manager)
	assert.Equal(t, "https://storage.example.com", manager.config.StorageEndpoint)
}

// TestAgentConfig tests agent config structure
func TestAgentConfig(t *testing.T) {
	config := &AgentConfig{
		Framework: &Framework{
			Name:    "openclaw",
			Version: "0.1.0",
		},
		Runtime: &Runtime{
			EntryPoint: "python main.py",
			WorkingDir: "/app",
			AgentPort:  9000,
		},
		Env: map[string]string{
			"API_KEY": "sk-12345",
		},
	}

	assert.Equal(t, "openclaw", config.Framework.Name)
	assert.Equal(t, "0.1.0", config.Framework.Version)
	assert.Equal(t, "python main.py", config.Runtime.EntryPoint)
	assert.Equal(t, "/app", config.Runtime.WorkingDir)
	assert.Equal(t, 9000, config.Runtime.AgentPort)
	assert.Equal(t, "sk-12345", config.Env["API_KEY"])
}

// TestEncryptDecrypt_RoundTrip tests encryption and decryption
func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	manager := NewManager(&ManagerConfig{})

	// Create test config
	config := &AgentConfig{
		Framework: &Framework{
			Name:    "openclaw",
			Version: "0.1.0",
		},
		Runtime: &Runtime{
			EntryPoint: "python main.py",
			AgentPort:  9000,
		},
		Env: map[string]string{
			"API_KEY": "sk-12345",
		},
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i) // Fixed test key
	}

	// Encrypt
	encrypted, err := manager.EncryptConfig(config, key)
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)

	// Decrypt
	decrypted, err := manager.DecryptConfig(encrypted, key)
	require.NoError(t, err)

	assert.Equal(t, config.Framework.Name, decrypted.Framework.Name)
	assert.Equal(t, config.Framework.Version, decrypted.Framework.Version)
	assert.Equal(t, config.Runtime.EntryPoint, decrypted.Runtime.EntryPoint)
	assert.Equal(t, config.Env["API_KEY"], decrypted.Env["API_KEY"])
}

// TestDecryptConfig_InvalidKey tests decryption with invalid key
func TestDecryptConfig_InvalidKey(t *testing.T) {
	manager := NewManager(&ManagerConfig{})

	config := &AgentConfig{
		Runtime: &Runtime{
			EntryPoint: "python main.py",
		},
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	// Encrypt with one key
	encrypted, err := manager.EncryptConfig(config, key)
	require.NoError(t, err)

	// Try to decrypt with different key
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = byte(255 - i)
	}

	decrypted, err := manager.DecryptConfig(encrypted, wrongKey)
	assert.Error(t, err)
	assert.Nil(t, decrypted)
}

// TestDecryptConfig_WrongKeySize tests decryption with wrong key size
func TestDecryptConfig_WrongKeySize(t *testing.T) {
	manager := NewManager(&ManagerConfig{})

	config := &AgentConfig{
		Runtime: &Runtime{
			EntryPoint: "python main.py",
		},
	}
	key := make([]byte, 32)

	encrypted, _ := manager.EncryptConfig(config, key)

	wrongKey := make([]byte, 16)
	decrypted, err := manager.DecryptConfig(encrypted, wrongKey)

	assert.Error(t, err)
	assert.Nil(t, decrypted)
	assert.Equal(t, ErrInvalidKeySize, err)
}

// TestDecryptConfig_EmptyData tests decryption with empty data
func TestDecryptConfig_EmptyData(t *testing.T) {
	manager := NewManager(&ManagerConfig{})

	key := make([]byte, 32)
	config, err := manager.DecryptConfig([]byte{}, key)

	assert.Error(t, err)
	assert.Nil(t, config)
}

// TestDecryptConfig_ShortData tests decryption with too short data
func TestDecryptConfig_ShortData(t *testing.T) {
	manager := NewManager(&ManagerConfig{})

	key := make([]byte, 32)
	shortData := make([]byte, 8) // Shorter than AES block size

	config, err := manager.DecryptConfig(shortData, key)

	assert.Error(t, err)
	assert.Nil(t, config)
}

// TestEncryptConfig_InvalidKey tests encryption with invalid key
func TestEncryptConfig_InvalidKey(t *testing.T) {
	manager := NewManager(&ManagerConfig{})

	config := &AgentConfig{
		Runtime: &Runtime{
			EntryPoint: "python main.py",
		},
	}
	key := make([]byte, 16) // Wrong key size

	encrypted, err := manager.EncryptConfig(config, key)

	assert.Error(t, err)
	assert.Nil(t, encrypted)
	assert.Equal(t, ErrInvalidKeySize, err)
}

// TestValidateConfig_Valid tests config validation
func TestValidateConfig_Valid(t *testing.T) {
	config := &AgentConfig{
		Framework: &Framework{
			Name: "openclaw",
		},
		Runtime: &Runtime{
			EntryPoint: "python main.py",
		},
	}

	err := ValidateConfig(config)
	assert.NoError(t, err)
}

// TestValidateConfig_MissingRuntime tests validation without runtime
func TestValidateConfig_MissingRuntime(t *testing.T) {
	config := &AgentConfig{
		Framework: &Framework{
			Name: "openclaw",
		},
	}

	err := ValidateConfig(config)
	assert.Error(t, err)
}

// TestValidateConfig_MissingEntryPoint tests validation without entry point
func TestValidateConfig_MissingEntryPoint(t *testing.T) {
	config := &AgentConfig{
		Framework: &Framework{
			Name: "openclaw",
		},
		Runtime: &Runtime{
			EntryPoint: "",
		},
	}

	err := ValidateConfig(config)
	assert.Error(t, err)
	assert.Equal(t, ErrMissingEntryPoint, err)
}

// TestValidateConfig_EmptyFrameworkName tests validation with empty framework name
func TestValidateConfig_EmptyFrameworkName(t *testing.T) {
	config := &AgentConfig{
		Framework: &Framework{
			Name: "",
		},
		Runtime: &Runtime{
			EntryPoint: "python main.py",
		},
	}

	err := ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "framework name")
}

// TestParseConfig_Success tests parsing JSON config
func TestParseConfig_Success(t *testing.T) {
	jsonConfig := `{
		"framework": {
			"name": "openclaw",
			"version": "0.1.0"
		},
		"runtime": {
			"entryPoint": "python main.py",
			"workingDir": "/app",
			"agentPort": 9000
		},
		"env": {
			"API_KEY": "sk-12345"
		}
	}`

	config, err := ParseConfig([]byte(jsonConfig))

	require.NoError(t, err)
	assert.Equal(t, "openclaw", config.Framework.Name)
	assert.Equal(t, "0.1.0", config.Framework.Version)
	assert.Equal(t, "python main.py", config.Runtime.EntryPoint)
	assert.Equal(t, "/app", config.Runtime.WorkingDir)
	assert.Equal(t, 9000, config.Runtime.AgentPort)
	assert.Equal(t, "sk-12345", config.Env["API_KEY"])
}

// TestParseConfig_InvalidJSON tests parsing invalid JSON
func TestParseConfig_InvalidJSON(t *testing.T) {
	jsonConfig := `{invalid json}`

	config, err := ParseConfig([]byte(jsonConfig))

	assert.Error(t, err)
	assert.Nil(t, config)
}

// TestParseConfig_EmptyEnv tests parsing config with empty env
func TestParseConfig_EmptyEnv(t *testing.T) {
	jsonConfig := `{
		"runtime": {
			"entryPoint": "python main.py",
			"agentPort": 9000
		}
	}`

	config, err := ParseConfig([]byte(jsonConfig))

	require.NoError(t, err)
	assert.Equal(t, "python main.py", config.Runtime.EntryPoint)
	assert.Nil(t, config.Env) // Env should be nil when not provided
}

// TestParseConfig_WithDefaults tests that defaults are set
func TestParseConfig_WithDefaults(t *testing.T) {
	jsonConfig := `{
		"runtime": {
			"entryPoint": "python main.py"
		}
	}`

	config, err := ParseConfig([]byte(jsonConfig))

	require.NoError(t, err)
	assert.Equal(t, "python main.py", config.Runtime.EntryPoint)
	assert.Equal(t, 9000, config.Runtime.AgentPort) // Default
	assert.Equal(t, "/app", config.Runtime.WorkingDir) // Default
}

// TestConfigToJSON tests converting config to JSON
func TestConfigToJSON(t *testing.T) {
	config := &AgentConfig{
		Framework: &Framework{
			Name:    "openclaw",
			Version: "0.1.0",
		},
		Runtime: &Runtime{
			EntryPoint: "python main.py",
		},
		Env: map[string]string{
			"API_KEY": "sk-12345",
		},
	}

	jsonData, err := ConfigToJSON(config)

	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "openclaw")
	assert.Contains(t, string(jsonData), "python main.py")
	assert.Contains(t, string(jsonData), "API_KEY")
}

// TestGenerateKey tests generating an encryption key
func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()

	require.NoError(t, err)
	assert.Len(t, key, 32) // 256 bits
}

// TestGenerateKey_MultipleTests tests multiple key generations
func TestGenerateKey_MultipleTests(t *testing.T) {
	keys := make(map[string]bool)

	// Generate 10 keys and check they're unique
	for i := 0; i < 10; i++ {
		key, err := GenerateKey()
		require.NoError(t, err)
		keyStr := string(key)
		assert.False(t, keys[keyStr], "Generated duplicate key")
		keys[keyStr] = true
	}

	assert.Len(t, keys, 10)
}

// TestRuntime_Defaults tests runtime defaults
func TestRuntime_Defaults(t *testing.T) {
	runtime := &Runtime{}
	runtime.SetDefaults()

	assert.Equal(t, 9000, runtime.AgentPort)
	assert.Equal(t, "/app", runtime.WorkingDir)
}

// TestRuntime_GetAgentPort tests getting agent port as string
func TestRuntime_GetAgentPort(t *testing.T) {
	runtime := &Runtime{AgentPort: 8080}
	assert.Equal(t, "8080", runtime.GetAgentPort())

	runtime.AgentPort = 0
	assert.Equal(t, "9000", runtime.GetAgentPort()) // Default
}

// TestConfigHash tests config hash generation
func TestConfigHash(t *testing.T) {
	config := &AgentConfig{
		Runtime: &Runtime{
			EntryPoint: "python main.py",
		},
	}

	hash1 := ConfigHash(config)
	hash2 := ConfigHash(config)

	assert.Equal(t, hash1, hash2) // Same config should produce same hash

	// Change config
	config.Runtime.EntryPoint = "node index.js"
	hash3 := ConfigHash(config)

	assert.NotEqual(t, hash1, hash3) // Different config should produce different hash
}

// TestManagerConfigDefaults tests manager config defaults
func TestManagerConfigDefaults(t *testing.T) {
	manager := NewManager(nil)

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.config)
}

// TestKeyFromHex tests converting hex string to key
func TestKeyFromHex(t *testing.T) {
	// Valid key - 64 hex chars = 32 bytes
	hexKey := "0x" + strings.Repeat("00", 32)
	key, err := KeyFromHex(hexKey)
	assert.NoError(t, err)
	assert.Len(t, key, 32)

	// Invalid hex
	_, err = KeyFromHex("0xghijkl")
	assert.Error(t, err)

	// Wrong length
	_, err = KeyFromHex("0x0011")
	assert.Error(t, err)
}

// TestKeyToHex tests converting key to hex string
func TestKeyToHex(t *testing.T) {
	key := make([]byte, 32)
	hexStr := KeyToHex(key)

	assert.Equal(t, "0x", hexStr[0:2])
	assert.Len(t, hexStr, 66) // 0x + 64 hex chars
}

// TestEncryptConfig_SameConfigDifferentCipherText tests that encryption produces different ciphertext each time
func TestEncryptConfig_SameConfigDifferentCipherText(t *testing.T) {
	manager := NewManager(&ManagerConfig{})

	config := &AgentConfig{
		Runtime: &Runtime{
			EntryPoint: "python main.py",
		},
	}
	key := make([]byte, 32)

	encrypted1, err := manager.EncryptConfig(config, key)
	require.NoError(t, err)

	encrypted2, err := manager.EncryptConfig(config, key)
	require.NoError(t, err)

	// Ciphertexts should be different due to random IV
	assert.NotEqual(t, encrypted1, encrypted2)
}
