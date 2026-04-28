package sealed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewState tests creating a new sealed state
func TestNewState(t *testing.T) {
	state := NewState()

	assert.NotNil(t, state)
	assert.Equal(t, StatusWaitingInit, state.GetStatus())
	assert.Nil(t, state.GetPrivateKey())
	assert.Empty(t, state.GetSealID())
	assert.Empty(t, state.GetTempKey())
	assert.Empty(t, state.GetAttestorURL())
	assert.Empty(t, state.GetPublicKey())
}

// TestState_Initialize tests initializing the state
func TestState_Initialize(t *testing.T) {
	state := NewState()

	err := state.Initialize("0xseal123", "0xtmpkey123", "https://attestor.example.com")

	require.NoError(t, err)
	assert.Equal(t, StatusSealed, state.GetStatus())
	assert.Equal(t, "0xseal123", state.GetSealID())
	assert.Equal(t, "0xtmpkey123", state.GetTempKey())
	assert.Equal(t, "https://attestor.example.com", state.GetAttestorURL())
	assert.NotEmpty(t, state.GetPublicKey())
	assert.NotNil(t, state.GetPrivateKey())
}

// TestState_Initialize_AlreadyInitialized tests double initialization
func TestState_Initialize_AlreadyInitialized(t *testing.T) {
	state := NewState()

	// First initialization
	err := state.Initialize("0xseal123", "0xtmpkey123", "https://attestor.example.com")
	require.NoError(t, err)

	// Second initialization should fail
	err = state.Initialize("0xseal456", "0xtmpkey456", "https://attestor.example.com")
	assert.Error(t, err)
	assert.Equal(t, ErrAlreadyInitialized, err)
}

// TestState_SetStatus tests setting status
func TestState_SetStatus(t *testing.T) {
	state := NewState()

	state.SetStatus(StatusAttesting)
	assert.Equal(t, StatusAttesting, state.GetStatus())

	state.SetStatus(StatusReady)
	assert.Equal(t, StatusReady, state.GetStatus())
}

// TestState_SetAgentID tests setting agent ID
func TestState_SetAgentID(t *testing.T) {
	state := NewState()

	state.SetAgentID("0xagent123")
	assert.Equal(t, "0xagent123", state.GetAgentID())
}

// TestState_SetAgentSealKey tests setting agentSeal private key
func TestState_SetAgentSealKey(t *testing.T) {
	state := NewState()

	// Create a dummy private key
	dummyKey := make([]byte, 32)
	for i := range dummyKey {
		dummyKey[i] = byte(i)
	}

	state.SetAgentSealKey(dummyKey)
	assert.Equal(t, dummyKey, state.GetAgentSealKey())
}

// TestState_GetPrivateKey tests getting private key
func TestState_GetPrivateKey(t *testing.T) {
	state := NewState()

	err := state.Initialize("0xseal123", "0xtmpkey123", "https://attestor.example.com")
	require.NoError(t, err)

	privKey := state.GetPrivateKey()
	assert.NotNil(t, privKey)
}

// TestState_GenerateKeyPair tests key generation
func TestState_GenerateKeyPair(t *testing.T) {
	pubKey, privKey, err := GenerateKeyPair()

	require.NoError(t, err)
	assert.NotNil(t, pubKey)
	assert.NotNil(t, privKey)
	assert.NotEmpty(t, pubKey)
	assert.NotEmpty(t, privKey) // DER encoded private key
}

// TestState_GenerateKeyPair_MultipleTests tests multiple key generations
func TestState_GenerateKeyPair_MultipleTests(t *testing.T) {
	keys := make(map[string]bool)

	// Generate 100 keys and check they're unique
	for i := 0; i < 100; i++ {
		pubKey, _, err := GenerateKeyPair()
		require.NoError(t, err)
		assert.False(t, keys[pubKey], "Generated duplicate public key")
		keys[pubKey] = true
	}

	assert.Len(t, keys, 100)
}

// TestStatus_String tests status string representation
func TestStatus_String(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusWaitingInit, "waiting_init"},
		{StatusSealed, "sealed"},
		{StatusAttesting, "attesting"},
		{StatusGettingKey, "getting_key"},
		{StatusWaitingEvent, "waiting_event"},
		{StatusFetchingMetadata, "fetching_metadata"},
		{StatusFetchingConfig, "fetching_config"},
		{StatusInstallingFramework, "installing_framework"},
		{StatusStartingAgent, "starting_agent"},
		{StatusReady, "ready"},
		{StatusError, "error"},
		{Status(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

// TestState_IsValidTransition tests valid status transitions
func TestState_IsValidTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     Status
		to       Status
		expected bool
	}{
		{"waiting_init to sealed", StatusWaitingInit, StatusSealed, true},
		{"sealed to attesting", StatusSealed, StatusAttesting, true},
		{"attesting to getting_key", StatusAttesting, StatusGettingKey, true},
		{"getting_key to waiting_event", StatusGettingKey, StatusWaitingEvent, true},
		{"waiting_event to fetching_metadata", StatusWaitingEvent, StatusFetchingMetadata, true},
		{"fetching_metadata to fetching_config", StatusFetchingMetadata, StatusFetchingConfig, true},
		{"fetching_config to installing_framework", StatusFetchingConfig, StatusInstallingFramework, true},
		{"installing_framework to starting_agent", StatusInstallingFramework, StatusStartingAgent, true},
		{"starting_agent to ready", StatusStartingAgent, StatusReady, true},
		{"ready to error", StatusReady, StatusError, true},
		{"any to error", StatusSealed, StatusError, true},
		{"sealed to ready invalid", StatusSealed, StatusReady, false},
		{"waiting_init to ready invalid", StatusWaitingInit, StatusReady, false},
		{"ready to sealed invalid", StatusReady, StatusSealed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsValidTransition(tt.from, tt.to))
		})
	}
}

// TestState_TransitionTo tests transitioning status
func TestState_TransitionTo(t *testing.T) {
	state := NewState()

	err := state.TransitionTo(StatusSealed)
	assert.NoError(t, err)
	assert.Equal(t, StatusSealed, state.GetStatus())

	err = state.TransitionTo(StatusReady)
	assert.Error(t, err)
	assert.Equal(t, StatusSealed, state.GetStatus())
}

// TestState_SetError tests setting error state
func TestState_SetError(t *testing.T) {
	state := NewState()

	state.SetStatus(StatusSealed)
	state.SetError(assert.AnError)

	assert.Equal(t, StatusError, state.GetStatus())
	assert.Equal(t, assert.AnError, state.GetError())
}

// TestState_Clone tests cloning state
func TestState_Clone(t *testing.T) {
	state := NewState()
	state.Initialize("0xseal123", "0xtmpkey123", "https://attestor.example.com")
	state.SetAgentID("0xagent123")

	cloned := state.Clone()

	assert.Equal(t, state.GetStatus(), cloned.GetStatus())
	assert.Equal(t, state.GetSealID(), cloned.GetSealID())
	assert.Equal(t, state.GetAgentID(), cloned.GetAgentID())
	assert.Equal(t, state.GetPublicKey(), cloned.GetPublicKey())
}

// TestState_IsReady tests IsReady method
func TestState_IsReady(t *testing.T) {
	state := NewState()

	assert.False(t, state.IsReady())

	state.SetStatus(StatusReady)
	assert.True(t, state.IsReady())

	state.SetStatus(StatusError)
	assert.False(t, state.IsReady())
}

// TestState_HasAgentSealKey tests HasAgentSealKey method
func TestState_HasAgentSealKey(t *testing.T) {
	state := NewState()

	assert.False(t, state.HasAgentSealKey())

	dummyKey := make([]byte, 32)
	state.SetAgentSealKey(dummyKey)

	assert.True(t, state.HasAgentSealKey())
}

// TestState_GetConfigHash tests config hash
func TestState_GetConfigHash(t *testing.T) {
	state := NewState()

	state.SetConfigHash("0xconfig123")
	assert.Equal(t, "0xconfig123", state.GetConfigHash())
}

// TestState_GetConfig tests config data
func TestState_GetConfig(t *testing.T) {
	state := NewState()

	configData := []byte(`{"test": "config"}`)
	state.SetConfig(configData)

	assert.Equal(t, configData, state.GetConfig())
}

// TestState_GetFramework tests framework info
func TestState_GetFramework(t *testing.T) {
	state := NewState()

	state.SetFramework("openclaw")
	assert.Equal(t, "openclaw", state.GetFramework())

	state.SetVersion("0.1.0")
	assert.Equal(t, "0.1.0", state.GetVersion())
}
