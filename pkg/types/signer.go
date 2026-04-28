package types

// SignerConfig holds signer configuration
type SignerConfig struct {
	// Algorithm to use for signing
	Algorithm string // Default: "ecdsa-secp256k1"

	// Optional: Pre-configured private key (for testing only)
	PrivateKeyHex string // DO NOT USE IN PRODUCTION
}

// NewSignerConfig creates a default signer configuration
func NewSignerConfig() *SignerConfig {
	return &SignerConfig{
		Algorithm: "ecdsa-secp256k1",
	}
}
