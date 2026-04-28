package attest

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	ethecies "github.com/ethereum/go-ethereum/crypto/ecies"
)

func TestNewClient(t *testing.T) {
	t.Run("creates client with config", func(t *testing.T) {
		cfg := &Config{
			BaseURL: "https://attestor.example.com",
			Timeout: 60 * time.Second,
		}

		client := NewClient(cfg)

		if client == nil {
			t.Fatal("expected non-nil client")
		}
		if client.baseURL != cfg.BaseURL {
			t.Errorf("expected baseURL %s, got %s", cfg.BaseURL, client.baseURL)
		}
		if client.httpClient.Timeout != cfg.Timeout {
			t.Errorf("expected timeout %v, got %v", cfg.Timeout, client.httpClient.Timeout)
		}
	})

	t.Run("uses default timeout when not specified", func(t *testing.T) {
		cfg := &Config{
			BaseURL: "https://attestor.example.com",
		}

		client := NewClient(cfg)

		expectedTimeout := 30 * time.Second
		if client.httpClient.Timeout != expectedTimeout {
			t.Errorf("expected default timeout %v, got %v", expectedTimeout, client.httpClient.Timeout)
		}
	})
}

func TestSetTEEPrivateKey(t *testing.T) {
	client := NewClient(&Config{BaseURL: "https://example.com"})

	// Test with hex string (placeholder for actual private key)
	privKeyHex := "a1b2c3d4e5f67890a1b2c3d4e5f67890a1b2c3d4e5f67890a1b2c3d4e5f67890"

	err := client.SetTEEPrivateKeyFromHex(privKeyHex)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if client.teePrivKey == nil {
		t.Error("expected teePrivKey to be set")
	}
}

func TestUnseal(t *testing.T) {
	t.Run("successful unseal", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/unseal" {
				t.Errorf("expected path /v1/unseal, got %s", r.URL.Path)
			}
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}

			// Parse request body
			var req UnsealRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request: %v", err)
			}

			// Validate required fields
			if req.SealID == "" {
				t.Error("expected seal_id in request")
			}
			if req.PubKey == "" {
				t.Error("expected pubkey in request")
			}
			if req.Signature == "" {
				t.Error("expected signature in request")
			}
			if req.Timestamp == 0 {
				t.Error("expected timestamp in request")
			}

			resp := UnsealResponse{
				Scheme:      "ecies-secp256k1-aes256gcm",
				EncryptedKey: "48656c6c6f", // "Hello" in hex (placeholder)
				ExpiresAt:   time.Now().Unix() + 3600,
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient(&Config{BaseURL: server.URL})
		// Set a mock private key (for signature creation)
		client.teePrivKey = newECDSAMock()

		resp, err := client.Unseal("seal-123", "0xpubkey", "0ximagehash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Scheme != "ecies-secp256k1-aes256gcm" {
			t.Errorf("expected scheme 'ecies-secp256k1-aes256gcm', got %s", resp.Scheme)
		}
		if resp.EncryptedKey != "48656c6c6f" {
			t.Errorf("expected encryptedKey '48656c6c6f', got %s", resp.EncryptedKey)
		}
	})

	t.Run("server returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			errResp := ErrorResponse{
				Code:    "INVALID_SIGNATURE",
				Message: "Signature verification failed",
			}
			json.NewEncoder(w).Encode(errResp)
		}))
		defer server.Close()

		client := NewClient(&Config{BaseURL: server.URL})
		client.teePrivKey = newECDSAMock()

		_, err := client.Unseal("seal-123", "0xpubkey", "0ximagehash")
		if err == nil {
			t.Error("expected error for unauthorized request")
		}
	})

	t.Run("network error", func(t *testing.T) {
		client := NewClient(&Config{
			BaseURL: "http://localhost:9999", // Non-existent server
			Timeout: 100 * time.Millisecond,
		})
		client.teePrivKey = newECDSAMock()

		_, err := client.Unseal("seal-123", "0xpubkey", "0ximagehash")
		if err == nil {
			t.Error("expected error for failed connection")
		}
	})
}

func TestDecryptAgentSealKey(t *testing.T) {
	client := NewClient(&Config{BaseURL: "https://example.com"})

	t.Run("invalid hex string", func(t *testing.T) {
		invalidHex := "not-a-valid-hex-string"
		privKey := newECDSAMock()

		_, err := client.DecryptAgentSealKey(invalidHex, privKey)
		if err == nil {
			t.Error("expected error for invalid hex")
		}
	})

	t.Run("ECIES encryption/decryption roundtrip", func(t *testing.T) {
		// Create receiver key for testing ECIES
		receiverPriv, err := ethcrypto.GenerateKey()
		if err != nil {
			t.Fatalf("failed to generate receiver key: %v", err)
		}

		// Encrypt a message using ECIES
		plaintext := []byte("test-agent-seal-key")
		receiverPub := &receiverPriv.PublicKey

		eciesReceiverPub := ethecies.ImportECDSAPublic(receiverPub)
		ciphertext, err := ethecies.Encrypt(rand.Reader, eciesReceiverPub, plaintext, nil, nil)
		if err != nil {
			t.Fatalf("failed to encrypt: %v", err)
		}

		// Decrypt
		decrypted, err := client.DecryptAgentSealKey("0x"+hex.EncodeToString(ciphertext), receiverPriv)
		if err != nil {
			t.Fatalf("failed to decrypt: %v", err)
		}

		if string(decrypted) != string(plaintext) {
			t.Errorf("expected %s, got %s", plaintext, decrypted)
		}
	})
}

func TestGetAgentSealKey(t *testing.T) {
	t.Run("full flow success", func(t *testing.T) {
		// Generate a key pair for the test
		receiverPriv, err := ethcrypto.GenerateKey()
		if err != nil {
			t.Fatalf("failed to generate key: %v", err)
		}

		// Create the encrypted key that will be returned by the mock server
		plaintextKey := make([]byte, 32) // 32 bytes agent seal key
		for i := range plaintextKey {
			plaintextKey[i] = byte(i)
		}

		receiverPub := &receiverPriv.PublicKey
		eciesReceiverPub := ethecies.ImportECDSAPublic(receiverPub)
		encryptedKey, err := ethecies.Encrypt(rand.Reader, eciesReceiverPub, plaintextKey, nil, nil)
		if err != nil {
			t.Fatalf("failed to encrypt: %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check request path for /provision endpoint
			if r.URL.Path != "/provision" {
				t.Errorf("expected request to /provision, got %s", r.URL.Path)
			}
			resp := ProvisionResponse{
				EncryptedAgentSealPriv: hex.EncodeToString(encryptedKey),
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient(&Config{BaseURL: server.URL})
		client.teePrivKey = receiverPriv

		key, err := client.GetAgentSealKey("seal-123", "0xpubkey", "0ximagehash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(key) != 32 {
			t.Errorf("expected 32 byte key, got %d", len(key))
		}
	})

	t.Run("unseal failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewClient(&Config{BaseURL: server.URL})
		client.teePrivKey = newECDSAMock()

		_, err := client.GetAgentSealKey("seal-123", "0xpubkey", "0ximagehash")
		if err == nil {
			t.Error("expected error for failed unseal")
		}
	})
}

func TestValidateSignature(t *testing.T) {
	t.Run("valid signature", func(t *testing.T) {
		// 0x + 130 hex chars = 132 total
		validSig := "0x" + "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef" + "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef" + "1b"

		err := ValidateSignature(validSig)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid length", func(t *testing.T) {
		invalidSig := "0x1234"

		err := ValidateSignature(invalidSig)
		if err == nil {
			t.Error("expected error for invalid length")
		}
	})

	t.Run("missing 0x prefix", func(t *testing.T) {
		invalidSig := "1234567890abcdef"

		err := ValidateSignature(invalidSig)
		if err == nil {
			t.Error("expected error for missing prefix")
		}
	})
}

func TestParseSignature(t *testing.T) {
	t.Run("valid signature", func(t *testing.T) {
		sig := "0x" +
			"0000000000000000000000000000000000000000000000000000000000000001" + // r
			"0000000000000000000000000000000000000000000000000000000000000002" + // s
			"1b" // v

		r, s, v, err := ParseSignature(sig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if r.String() != "1" {
			t.Errorf("expected r=1, got %s", r.String())
		}
		if s.String() != "2" {
			t.Errorf("expected s=2, got %s", s.String())
		}
		if v != 0x1b {
			t.Errorf("expected v=0x1b, got %x", v)
		}
	})
}

func TestFormatSignature(t *testing.T) {
	t.Run("formats signature correctly", func(t *testing.T) {
		// Create test big ints
		r := new(big.Int).Lsh(big.NewInt(1), 255) // 2^255
		s := new(big.Int).Lsh(big.NewInt(2), 255) // 2^256
		v := byte(27)

		sig := FormatSignature(r, s, v)

		if len(sig) != 132 { // 0x + 130 hex chars
			t.Errorf("expected signature length 132, got %d", len(sig))
		}
		if sig[0:2] != "0x" {
			t.Errorf("expected signature to start with 0x, got %s", sig[0:2])
		}
	})
}

// newECDSAMock creates a mock ECDSA private key for testing
// Uses secp256k1 curve for actual protocol compliance
func newECDSAMock() *ecdsa.PrivateKey {
	// Generate a real secp256k1 key for testing
	privKey, err := ethcrypto.GenerateKey()
	if err != nil {
		// Fallback to a minimal valid key if generation fails
		return &ecdsa.PrivateKey{
			D: new(big.Int).SetUint64(1),
		}
	}
	return privKey
}

func TestTimestamp(t *testing.T) {
	ts := Timestamp()
	if ts == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestFormatTimestamp(t *testing.T) {
	ts := int64(1234567890)
	result := FormatTimestamp(ts)
	if result != "1234567890" {
		t.Errorf("expected '1234567890', got %s", result)
	}
}

func TestBytesToSecp256k1PrivateKey(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		// Create a 32-byte key
		keyBytes := make([]byte, 32)
		for i := range keyBytes {
			keyBytes[i] = byte(i + 1)
		}

		privKey, err := BytesToSecp256k1PrivateKey(keyBytes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if privKey == nil {
			t.Fatal("expected non-nil private key")
		}
	})

	t.Run("invalid length", func(t *testing.T) {
		keyBytes := make([]byte, 16) // Wrong length

		_, err := BytesToSecp256k1PrivateKey(keyBytes)
		if err == nil {
			t.Error("expected error for invalid key length")
		}
	})
}

func TestDecryptSealedKey(t *testing.T) {
	t.Run("successful decryption", func(t *testing.T) {
		// Generate a key pair
		receiverPriv, err := ethcrypto.GenerateKey()
		if err != nil {
			t.Fatalf("failed to generate key: %v", err)
		}

		// Create a sealed key (encrypted)
		plaintextKey := make([]byte, 32)
		for i := range plaintextKey {
			plaintextKey[i] = byte(i)
		}

		receiverPub := &receiverPriv.PublicKey
		eciesReceiverPub := ethecies.ImportECDSAPublic(receiverPub)
		sealedKey, err := ethecies.Encrypt(rand.Reader, eciesReceiverPub, plaintextKey, nil, nil)
		if err != nil {
			t.Fatalf("failed to encrypt: %v", err)
		}

		// Decrypt
		decrypted, err := DecryptSealedKey(sealedKey, receiverPriv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify
		if len(decrypted) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(decrypted))
		}
		for i := range plaintextKey {
			if decrypted[i] != plaintextKey[i] {
				t.Errorf("mismatch at byte %d: expected %d, got %d", i, plaintextKey[i], decrypted[i])
			}
		}
	})

	t.Run("empty sealed key", func(t *testing.T) {
		receiverPriv, _ := ethcrypto.GenerateKey()

		_, err := DecryptSealedKey([]byte{}, receiverPriv)
		if err == nil {
			t.Error("expected error for empty sealed key")
		}
	})

	t.Run("nil private key", func(t *testing.T) {
		sealedKey := make([]byte, 32)

		_, err := DecryptSealedKey(sealedKey, nil)
		if err == nil {
			t.Error("expected error for nil private key")
		}
	})
}
