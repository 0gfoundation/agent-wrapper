package storage

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	t.Run("creates new client with defaults", func(t *testing.T) {
		cfg := &Config{
			Endpoint: "https://storage.0g.io",
		}

		client := NewClient(cfg)

		assert.NotNil(t, client)
		assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
		assert.Equal(t, cfg.Endpoint, client.config.Endpoint)
	})

	t.Run("creates new client with custom timeout", func(t *testing.T) {
		cfg := &Config{
			Endpoint: "https://storage.0g.io",
			Timeout:  60 * time.Second,
		}

		client := NewClient(cfg)

		assert.Equal(t, 60*time.Second, client.httpClient.Timeout)
	})

	t.Run("creates new client with API key", func(t *testing.T) {
		cfg := &Config{
			Endpoint: "https://storage.0g.io",
			APIKey:   "test-key",
		}

		client := NewClient(cfg)

		assert.Equal(t, "test-key", client.config.APIKey)
	})
}

func TestValidateConfigHash(t *testing.T) {
	t.Run("accepts valid hex hash", func(t *testing.T) {
		hash := "abc123def4567890"
		err := ValidateConfigHash(hash)
		assert.NoError(t, err)
	})

	t.Run("accepts hash with 0x prefix", func(t *testing.T) {
		hash := "0xabc123def456"
		err := ValidateConfigHash(hash)
		assert.NoError(t, err)
	})

	t.Run("rejects empty hash", func(t *testing.T) {
		hash := ""
		err := ValidateConfigHash(hash)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hash cannot be empty")
	})

	t.Run("rejects non-hex characters", func(t *testing.T) {
		hash := "abc123xyz"
		err := ValidateConfigHash(hash)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid hexadecimal")
	})
}

func TestFetchConfig(t *testing.T) {
	t.Run("rejects empty hash", func(t *testing.T) {
		cfg := &Config{
			Endpoint: "https://storage.0g.io",
		}
		client := NewClient(cfg)

		_, err := client.FetchConfig("")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hash cannot be empty")
	})

	t.Run("returns error when server unavailable", func(t *testing.T) {
		cfg := &Config{
			Endpoint: "https://storage.0g.io",
		}
		client := NewClient(cfg)

		_, err := client.FetchConfig("abc123")

		assert.Error(t, err)
	})
}

func TestIntegrationWithMockServer(t *testing.T) {
	t.Run("mock storage server returns config", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/config/abc123", r.URL.Path)
			assert.Equal(t, "GET", r.Method)

			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("encrypted-config-data"))
		}))
		defer server.Close()

		cfg := &Config{
			Endpoint: server.URL,
		}
		client := NewClient(cfg)

		data, err := client.FetchConfig("abc123")

		assert.NoError(t, err)
		assert.Equal(t, []byte("encrypted-config-data"), data)
	})

	t.Run("returns error on 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		cfg := &Config{
			Endpoint: server.URL,
		}
		client := NewClient(cfg)

		_, err := client.FetchConfig("abc123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code")
	})

	t.Run("includes API key in header when set", func(t *testing.T) {
		receivedKey := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedKey = r.Header.Get("X-API-Key")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &Config{
			Endpoint: server.URL,
			APIKey:   "my-secret-key",
		}
		client := NewClient(cfg)

		client.FetchConfig("abc123")

		assert.Equal(t, "my-secret-key", receivedKey)
	})
}
