package storage

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config holds configuration for the Storage client
type Config struct {
	// Endpoint is the 0G Storage service URL
	Endpoint string

	// Timeout for HTTP requests
	Timeout time.Duration

	// APIKey for authenticated requests (optional)
	APIKey string
}

// Client handles communication with the 0G Storage service
type Client struct {
	config     *Config
	httpClient *http.Client
}

// NewClient creates a new Storage client
func NewClient(cfg *Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
			// Explicitly disable proxy to avoid connection issues
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
	}
}

// FetchConfig retrieves encrypted agent configuration from 0G Storage
func (c *Client) FetchConfig(configHash string) ([]byte, error) {
	if err := ValidateConfigHash(configHash); err != nil {
		return nil, fmt.Errorf("invalid hash: %w", err)
	}

	// Strip 0x prefix if present
	hash := strings.TrimPrefix(configHash, "0x")

	url := fmt.Sprintf("%s/config/%s", c.config.Endpoint, hash)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch config failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return data, nil
}

// DownloadFile downloads a file from 0G Storage by its hash
// This is used for downloading intelligent data blobs
func (c *Client) DownloadFile(fileHash string) ([]byte, error) {
	if err := ValidateConfigHash(fileHash); err != nil {
		return nil, fmt.Errorf("invalid hash: %w", err)
	}

	// Strip 0x prefix if present
	hash := strings.TrimPrefix(fileHash, "0x")

	url := fmt.Sprintf("%s/file/%s", c.config.Endpoint, hash)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("download file failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return data, nil
}

// ValidateConfigHash validates that a config hash is valid hexadecimal
func ValidateConfigHash(hash string) error {
	if hash == "" {
		return errors.New("hash cannot be empty")
	}

	// Strip 0x prefix if present
	hexStr := strings.TrimPrefix(hash, "0x")

	// Validate it's valid hex
	_, err := hex.DecodeString(hexStr)
	if err != nil {
		return fmt.Errorf("invalid hexadecimal hash: %w", err)
	}

	return nil
}

// doRequest performs an HTTP request
func (c *Client) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	if c.config.APIKey != "" {
		req.Header.Set("X-API-Key", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp, nil
}
