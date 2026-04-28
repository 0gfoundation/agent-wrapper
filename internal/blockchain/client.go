// Package blockchain provides a client for interacting with the blockchain
// to fetch agent metadata and listen for events.
package blockchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Config holds configuration for the blockchain client
type Config struct {
	// Endpoint is the blockchain service URL
	Endpoint string

	// Timeout for HTTP requests
	Timeout time.Duration

	// APIKey for authenticated requests (optional)
	APIKey string
}

// IntelligentData represents a piece of intelligent data bound to a token
// This matches the contract's IntelligentData struct
type IntelligentData struct {
	DataDescription string `json:"dataDescription"`
	DataHash        string `json:"dataHash"`
}

// AgentMetadata represents agent metadata from the blockchain
// Simplified to only include what's actually stored on-chain
type AgentMetadata struct {
	AgentID    uint256          `json:"agentId"`
	SealID     string           `json:"sealId"`
	AgentSeal  string           `json:"agentSeal"`
	IntelligentDatas []IntelligentData `json:"intelligentDatas"`
}

// uint256 is a custom type for large integers (agentId is uint256 in Solidity)
type uint256 string

// Client handles communication with the blockchain service
type Client struct {
	config     *Config
	httpClient *http.Client
}

// NewClient creates a new blockchain client
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

// GetAgentIdBySealId retrieves the agentId for a given sealId
func (c *Client) GetAgentIdBySealId(sealID string) (string, error) {
	if err := ValidateSealID(sealID); err != nil {
		return "", fmt.Errorf("invalid seal ID: %w", err)
	}

	url := fmt.Sprintf("%s/agents/by-seal-id/%s", c.formatEndpoint(), sealID)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("get agentId by sealId failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AgentID string `json:"agentId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.AgentID, nil
}

// GetIntelligentDatas retrieves intelligent data for an agent
func (c *Client) GetIntelligentDatas(agentID string) ([]IntelligentData, error) {
	if err := ValidateAgentID(agentID); err != nil {
		return nil, fmt.Errorf("invalid agent ID: %w", err)
	}

	url := fmt.Sprintf("%s/agents/%s/intelligent-datas", c.formatEndpoint(), agentID)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get intelligent datas failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var datas []IntelligentData
	if err := json.NewDecoder(resp.Body).Decode(&datas); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return datas, nil
}

// GetAgentMetadata retrieves complete agent metadata
// This combines agentId, sealId, agentSeal, and intelligentDatas
func (c *Client) GetAgentMetadata(agentID string) (*AgentMetadata, error) {
	if err := ValidateAgentID(agentID); err != nil {
		return nil, fmt.Errorf("invalid agent ID: %w", err)
	}

	url := fmt.Sprintf("%s/agents/%s/metadata", c.formatEndpoint(), agentID)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get metadata failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var metadata AgentMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &metadata, nil
}

// ListenForSealBonded creates an event listener for SealBonded events
func (c *Client) ListenForSealBonded(sealID string) *EventListener {
	ctx, cancel := context.WithCancel(context.Background())
	return &EventListener{
		client: c,
		sealID: sealID,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

// doRequest performs an HTTP request
func (c *Client) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// formatEndpoint formats the endpoint URL
func (c *Client) formatEndpoint() string {
	endpoint := c.config.Endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	return endpoint
}

// ValidateAgentID validates an agent ID
func ValidateAgentID(agentID string) error {
	if agentID == "" {
		return errors.New("agent ID cannot be empty")
	}
	// Agent ID is a uint256, validate as hex string
	// Strip 0x prefix
	hexStr := strings.TrimPrefix(agentID, "0x")
	// Validate hex
	_, err := hex.DecodeString(hexStr)
	if err != nil {
		return fmt.Errorf("must be a valid hex string: %w", err)
	}
	// Check even length
	if len(hexStr)%2 != 0 {
		return errors.New("must be a valid hex string with even length")
	}
	return nil
}

// ValidateSealID validates a seal ID
func ValidateSealID(sealID string) error {
	if sealID == "" {
		return errors.New("seal ID cannot be empty")
	}
	// Seal ID is bytes32 (64 hex chars)
	// Strip 0x prefix
	hexStr := strings.TrimPrefix(sealID, "0x")
	// Validate hex
	_, err := hex.DecodeString(hexStr)
	if err != nil {
		return fmt.Errorf("must be a valid hex string: %w", err)
	}
	// Check even length
	if len(hexStr)%2 != 0 {
		return errors.New("must be a valid hex string with even length")
	}
	return nil
}

// EventListener listens for blockchain events
type EventListener struct {
	client    *Client
	sealID    string
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	started   bool
}

// Start starts listening for events
func (l *EventListener) Start(callback func(agentID string)) {
	l.mu.Lock()
	if l.started {
		l.mu.Unlock()
		return
	}
	l.started = true
	l.mu.Unlock()

	go func() {
		l.closeOnce.Do(func() {
			close(l.done)
		})
		for {
			select {
			case <-l.ctx.Done():
				return
			default:
				// Poll for events
				// In production, this would use WebSocket or similar
				time.Sleep(5 * time.Second)

				// TODO: Implement actual event polling
				_ = callback
			}
		}
	}()
}

// Stop stops listening for events
func (l *EventListener) Stop() {
	l.mu.Lock()
	started := l.started
	l.mu.Unlock()

	if started && l.cancel != nil {
		l.cancel()
	}
	<-l.done
}

// WaitForEvent waits for a SealBonded event for the configured sealID
func (l *EventListener) WaitForEvent(ctx context.Context) (string, error) {
	// TODO: Implement actual event waiting
	// For now, return a placeholder
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return "", errors.New("event not implemented yet")
	}
}

// ── ITransferred Event Scanning ────────────────────────────────────────

// AgenticID ABI subset for ITransferred event
const agenticIDABI = `[
	{"type":"event","name":"ITransferred","anonymous":false,"inputs":[
		{"name":"from","type":"address","indexed":true},
		{"name":"to","type":"address","indexed":true},
		{"name":"tokenId","type":"uint256","indexed":true},
		{"name":"entries","type":"tuple[]","indexed":false,"components":[
			{"name":"dataHash","type":"bytes32"},
			{"name":"sealedKey","type":"bytes"}
		]}
	]}
]`

// SealedKeyEntry represents an entry in ITransferred event
type SealedKeyEntry struct {
	DataHash  [32]byte `json:"dataHash"`
	SealedKey []byte   `json:"sealedKey"`
}

// GetSealedKeys retrieves sealed keys from ITransferred events for a given agent ID
// This scans blockchain logs to find the most recent ITransferred event
func (c *Client) GetSealedKeys(rpcURL, contractAddr, agentID string) (map[[32]byte][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Connect to Ethereum RPC
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial RPC: %w", err)
	}
	defer client.Close()

	// Parse ABI
	parsedABI, err := abi.JSON(strings.NewReader(agenticIDABI))
	if err != nil {
		return nil, fmt.Errorf("parse ABI: %w", err)
	}

	// Parse contract address
	contract := common.HexToAddress(contractAddr)

	// Parse agent ID (tokenId)
	tokenID := new(big.Int)
	if strings.HasPrefix(agentID, "0x") {
		tokenID, _ = new(big.Int).SetString(agentID[2:], 16)
	} else {
		tokenID, _ = new(big.Int).SetString(agentID, 10)
	}

	// Get the ITransferred event
	event, ok := parsedABI.Events["ITransferred"]
	if !ok {
		return nil, errors.New("ITransferred event not found in ABI")
	}

	tokenTopic := common.BigToHash(tokenID)

	// Scan strategy: try head first, then backward scan
	const scanChunk = 5000

	// Get latest block
	latest, err := client.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("get block number: %w", err)
	}

	// Phase 1: Try head window (recent blocks)
	var from uint64
	if latest >= scanChunk {
		from = latest - scanChunk + 1
	}
	q := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(latest),
		Addresses: []common.Address{contract},
		Topics:    [][]common.Hash{{event.ID}, nil, nil, {tokenTopic}},
	}

	logs, err := client.FilterLogs(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("filter logs (head): %w", err)
	}

	if len(logs) > 0 {
		// Use the most recent log (last in array)
		return parseITransferredLogs(parsedABI, logs)
	}

	// Phase 2: Backward scan
	to := from
	if to >= scanChunk {
		to -= scanChunk
	} else {
		return nil, fmt.Errorf("no ITransferred found for tokenId %s", agentID)
	}

	chunks := 0
	for {
		var from uint64
		if to >= scanChunk {
			from = to - scanChunk + 1
		}
		q = ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(from),
			ToBlock:   new(big.Int).SetUint64(to),
			Addresses: []common.Address{contract},
			Topics:    [][]common.Hash{{event.ID}, nil, nil, {tokenTopic}},
		}

		logs, err = client.FilterLogs(ctx, q)
		chunks++
		if err != nil {
			return nil, fmt.Errorf("filter logs [chunk %d]: %w", chunks, err)
		}

		if len(logs) > 0 {
			return parseITransferredLogs(parsedABI, logs)
		}

		if from == 0 {
			return nil, fmt.Errorf("no ITransferred found for tokenId %s (%d chunks scanned)", agentID, chunks)
		}
		to = from - 1

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
}

// parseITransferredLogs parses ITransferred event logs and returns sealed keys map
func parseITransferredLogs(parsedABI abi.ABI, logs []types.Log) (map[[32]byte][]byte, error) {
	if len(logs) == 0 {
		return nil, errors.New("no logs to parse")
	}

	// Use the most recent log (last in array)
	lg := logs[len(logs)-1]

	var result struct {
		Entries []SealedKeyEntry
	}
	if err := parsedABI.UnpackIntoInterface(&result, "ITransferred", lg.Data); err != nil {
		return nil, fmt.Errorf("unpack ITransferred log: %w", err)
	}

	out := make(map[[32]byte][]byte)
	for _, entry := range result.Entries {
		out[entry.DataHash] = entry.SealedKey
	}

	return out, nil
}

// GetRPCURL extracts RPC URL from the endpoint config
// This is a helper for using the blockchain client with direct RPC calls
func (c *Client) GetRPCURL() string {
	// If endpoint is already an RPC URL, return it
	if strings.Contains(c.config.Endpoint, "rpc") || strings.Contains(c.config.Endpoint, "8545") {
		return c.formatEndpoint()
	}
	// Otherwise, the endpoint is a service URL, need to construct RPC URL
	// This typically requires additional config or service discovery
	// For now, return the endpoint and let the caller handle it
	return c.formatEndpoint()
}
