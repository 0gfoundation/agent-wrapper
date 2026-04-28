// Package sealed manages the sealed state of the agent wrapper,
// including key generation and lifecycle status.
package sealed

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sync"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

var (
	// ErrAlreadyInitialized is returned when trying to initialize an already initialized state
	ErrAlreadyInitialized = errors.New("already initialized")
	// ErrInvalidTransition is returned for invalid status transitions
	ErrInvalidTransition = errors.New("invalid status transition")
)

// Status represents the current state of the wrapper
type Status int

const (
	StatusWaitingInit Status = iota
	StatusSealed
	StatusAttesting
	StatusGettingKey
	StatusWaitingEvent
	StatusFetchingMetadata
	StatusFetchingConfig
	StatusInstallingFramework
	StatusStartingAgent
	StatusReady
	StatusError
)

// String returns the string representation of the status
func (s Status) String() string {
	switch s {
	case StatusWaitingInit:
		return "waiting_init"
	case StatusSealed:
		return "sealed"
	case StatusAttesting:
		return "attesting"
	case StatusGettingKey:
		return "getting_key"
	case StatusWaitingEvent:
		return "waiting_event"
	case StatusFetchingMetadata:
		return "fetching_metadata"
	case StatusFetchingConfig:
		return "fetching_config"
	case StatusInstallingFramework:
		return "installing_framework"
	case StatusStartingAgent:
		return "starting_agent"
	case StatusReady:
		return "ready"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// State holds the sealed state during the wrapper lifecycle
type State struct {
	mu sync.RWMutex

	// Status is the current status
	status Status

	// sealID is the seal identifier
	sealID string

	// tempKey is the temporary attestation key
	tempKey string

	// attestorURL is the attestor service URL
	attestorURL string

	// publicKey is the generated public key (hex)
	publicKey string

	// privateKey is the generated private key for signing
	privateKey *ecdsa.PrivateKey

	// agentID is the agent ID from blockchain
	agentID string

	// agentSealKey is the agentSeal private key from Attestor
	agentSealKey []byte

	// configHash is the configuration hash
	configHash string

	// config is the decrypted configuration
	config []byte

	// framework is the agent framework name
	framework string

	// version is the framework version
	version string

	// err is the current error if any
	err error
}

// NewState creates a new sealed state
func NewState() *State {
	return &State{
		status: StatusWaitingInit,
	}
}

// Initialize initializes the state with the given parameters
func (s *State) Initialize(sealID, tempKey, attestorURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != StatusWaitingInit {
		return ErrAlreadyInitialized
	}

	// Generate key pair
	pubKeyHex, privKeyECDSA, err := generateKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	s.sealID = sealID
	s.tempKey = tempKey
	s.attestorURL = attestorURL
	s.publicKey = pubKeyHex
	s.privateKey = privKeyECDSA
	s.status = StatusSealed

	return nil
}

// GetStatus returns the current status
func (s *State) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// SetStatus updates the current status
func (s *State) SetStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

// TransitionTo transitions to a new status if valid
func (s *State) TransitionTo(newStatus Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !IsValidTransition(s.status, newStatus) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, s.status, newStatus)
	}

	s.status = newStatus
	return nil
}

// GetSealID returns the seal ID
func (s *State) GetSealID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sealID
}

// GetTempKey returns the temporary key
func (s *State) GetTempKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tempKey
}

// GetAttestorURL returns the attestor URL
func (s *State) GetAttestorURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.attestorURL
}

// GetPublicKey returns the public key (hex)
func (s *State) GetPublicKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publicKey
}

// GetPrivateKey returns the private key
func (s *State) GetPrivateKey() *ecdsa.PrivateKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.privateKey
}

// GetPrivateKeyBytes returns the private key as bytes (for attest client)
func (s *State) GetPrivateKeyBytes() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.privateKey == nil {
		return nil
	}
	// Convert ECDSA private key to bytes (D parameter)
	dBytes := s.privateKey.D.Bytes()
	// Pad to 32 bytes
	result := make([]byte, 32)
	copy(result[32-len(dBytes):], dBytes)
	return result
}

// GetAgentID returns the agent ID
func (s *State) GetAgentID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agentID
}

// SetAgentID sets the agent ID
func (s *State) SetAgentID(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentID = agentID
}

// GetAgentSealKey returns the agentSeal private key
func (s *State) GetAgentSealKey() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agentSealKey
}

// SetAgentSealKey sets the agentSeal private key
func (s *State) SetAgentSealKey(key []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentSealKey = key
}

// HasAgentSealKey returns true if agentSeal key is set
func (s *State) HasAgentSealKey() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.agentSealKey) > 0
}

// GetConfigHash returns the configuration hash
func (s *State) GetConfigHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configHash
}

// SetConfigHash sets the configuration hash
func (s *State) SetConfigHash(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configHash = hash
}

// GetConfig returns the decrypted configuration
func (s *State) GetConfig() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// SetConfig sets the decrypted configuration
func (s *State) SetConfig(config []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
}

// GetFramework returns the framework name
func (s *State) GetFramework() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.framework
}

// SetFramework sets the framework name
func (s *State) SetFramework(framework string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.framework = framework
}

// GetVersion returns the framework version
func (s *State) GetVersion() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// SetVersion sets the framework version
func (s *State) SetVersion(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version = version
}

// GetError returns the current error if any
func (s *State) GetError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

// SetError sets an error and updates status to Error
func (s *State) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
	s.status = StatusError
}

// IsReady returns true if the state is ready
func (s *State) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status == StatusReady
}

// Clone creates a shallow copy of the state (for safe external access)
func (s *State) Clone() *State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &State{
		status:      s.status,
		sealID:      s.sealID,
		publicKey:   s.publicKey,
		agentID:     s.agentID,
		configHash:  s.configHash,
		framework:   s.framework,
		version:     s.version,
		err:         s.err,
	}
}

// Sign signs the given data using ECDSA
// Returns the signature as R+S concatenated (64 bytes)
func (s *State) Sign(data []byte) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.privateKey == nil {
		return nil
	}

	// Hash the data
	hash := sha256.Sum256(data)

	// Sign the hash
	r, s_sig, err := ecdsa.Sign(rand.Reader, s.privateKey, hash[:])
	if err != nil {
		return nil
	}

	// Encode R and S as 32-byte big endian integers
	rBytes := r.Bytes()
	sBytes := s_sig.Bytes()

	// Pad to 32 bytes each
	rPadded := make([]byte, 32)
	sPadded := make([]byte, 32)
	copy(rPadded[32-len(rBytes):], rBytes)
	copy(sPadded[32-len(sBytes):], sBytes)

	// Concatenate R and S
	signature := append(rPadded, sPadded...)
	return signature
}

// VerifySignature verifies a signature using the public key
func (s *State) VerifySignature(data, signature []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.privateKey == nil {
		return false
	}

	// Hash the data
	hash := sha256.Sum256(data)

	// Parse signature
	if len(signature) != 64 {
		return false
	}

	r := new(big.Int).SetBytes(signature[:32])
	s_sig := new(big.Int).SetBytes(signature[32:])

	// Verify
	return ecdsa.Verify(&s.privateKey.PublicKey, hash[:], r, s_sig)
}

// SignWithAgentSealKey signs the given data using agentSeal private key
// This is used for signing responses to owners/A2A calls
// The agentSealKey is a 32-byte secp256k1 private key
// Returns the signature as R+S concatenated (64 bytes)
func (s *State) SignWithAgentSealKey(data []byte) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.agentSealKey) == 0 {
		return nil
	}

	// Use go-ethereum's secp256k1 curve for signing
	// agentSealKey is a 32-byte secp256k1 private key (from ECIES decryption)
	privKey, err := ethcrypto.ToECDSA(s.agentSealKey)
	if err != nil {
		return nil
	}

	// Hash the data with Keccak256 (Ethereum standard)
	hash := ethcrypto.Keccak256(data)

	// Sign the hash with secp256k1
	// crypto.Sign returns a 65-byte signature with V normalized to 27/28
	signature, err := ethcrypto.Sign(hash, privKey)
	if err != nil {
		return nil
	}

	// Return only R+S (64 bytes), excluding V
	return signature[:64]
}

// VerifySignatureWithAgentSealKey verifies a signature using agentSeal public key
// The signature should be 64 bytes (R+S concatenated)
func (s *State) VerifySignatureWithAgentSealKey(data, signature []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.agentSealKey) == 0 {
		return false
	}
	if len(signature) != 64 {
		return false
	}

	// Derive public key from agentSealKey using secp256k1
	privKey, err := ethcrypto.ToECDSA(s.agentSealKey)
	if err != nil {
		return false
	}

	// Hash the data with Keccak256 (Ethereum standard)
	hash := ethcrypto.Keccak256(data)

	// Parse signature (R+S)
	r := new(big.Int).SetBytes(signature[:32])
	s_sig := new(big.Int).SetBytes(signature[32:])

	// Verify using secp256k1
	return ecdsa.Verify(&privKey.PublicKey, hash, r, s_sig)
}

// generateKeyPair generates a new ECDSA key pair using P-256 curve
// Returns (publicKeyHex, privateKeyECDSA, error)
func generateKeyPair() (string, *ecdsa.PrivateKey, error) {
	// Generate private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Encode public key to hex
	pubKeyBytes := elliptic.Marshal(privKey.Curve, privKey.PublicKey.X, privKey.PublicKey.Y)
	pubKeyHex := "0x" + hex.EncodeToString(pubKeyBytes)

	return pubKeyHex, privKey, nil
}

// GenerateKeyPair generates a new ECDSA key pair using P-256 curve
// Returns (publicKeyHex, privateKeyBytes, error)
func GenerateKeyPair() (string, []byte, error) {
	// Generate private key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Encode public key to hex
	pubKeyBytes := elliptic.Marshal(privKey.Curve, privKey.PublicKey.X, privKey.PublicKey.Y)
	pubKeyHex := "0x" + hex.EncodeToString(pubKeyBytes)

	// Encode private key to bytes
	privKeyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	return pubKeyHex, privKeyBytes, nil
}

// ParsePrivateKey parses a PEM-encoded private key
func ParsePrivateKey(pemData []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse EC private key: %w", err)
	}

	return key, nil
}

// EncodePrivateKeyToPEM encodes a private key to PEM format
func EncodePrivateKeyToPEM(privKey *ecdsa.PrivateKey) ([]byte, error) {
	privKeyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	block := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privKeyBytes,
	}

	return pem.EncodeToMemory(block), nil
}

// IsValidTransition checks if a status transition is valid
func IsValidTransition(from, to Status) bool {
	// Can always transition to error
	if to == StatusError {
		return true
	}

	// Define valid transitions
	validTransitions := map[Status][]Status{
		StatusWaitingInit:       {StatusSealed},
		StatusSealed:            {StatusAttesting},
		StatusAttesting:         {StatusGettingKey},
		StatusGettingKey:        {StatusWaitingEvent},
		StatusWaitingEvent:      {StatusFetchingMetadata},
		StatusFetchingMetadata:  {StatusFetchingConfig},
		StatusFetchingConfig:    {StatusInstallingFramework},
		StatusInstallingFramework: {StatusStartingAgent},
		StatusStartingAgent:     {StatusReady},
		StatusReady:             {}, // No transitions from ready
		StatusError:             {}, // No transitions from error
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, allowedStatus := range allowed {
		if allowedStatus == to {
			return true
		}
	}

	return false
}
