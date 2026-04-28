// Package flow provides initialization flow orchestration
package flow

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/0gfoundation/agent-wrapper/internal/attest"
	"github.com/0gfoundation/agent-wrapper/internal/blockchain"
	"github.com/0gfoundation/agent-wrapper/internal/config"
	"github.com/0gfoundation/agent-wrapper/internal/framework"
	"github.com/0gfoundation/agent-wrapper/internal/process"
	"github.com/0gfoundation/agent-wrapper/internal/sealed"
	"github.com/0gfoundation/agent-wrapper/internal/storage"
	initpkg "github.com/0gfoundation/agent-wrapper/internal/init"
)

// StatusProvider provides status information about the initialization flow
type StatusProvider interface {
	IsFlowComplete() bool
	GetAgentPort() string
}

// Orchestrator manages the initialization flow
type Orchestrator struct {
	initServer      *initpkg.Server
	sealedState     *sealed.State // Must be shared with proxy
	frameworkInst   *framework.Installer
	configManager   *config.Manager
	attestClient    *attest.Client
	blockchainCli   *blockchain.Client
	storageCli      *storage.Client
	processMgr      *process.Manager
	agentConfig     *config.AgentConfig

	mu              sync.RWMutex
	initialized     bool
	flowComplete    bool
}

// Config holds configuration for the orchestrator
type Config struct {
	StorageEndpoint string
	AttestorURL     string
	BlockchainURL   string
	RPCURL          string // Ethereum RPC URL for ITransferred event scanning
	ContractAddr    string // AgenticID contract address
}

// New creates a new orchestrator
func New(initServer *initpkg.Server, sealedState *sealed.State, cfg *Config) *Orchestrator {
	orchestratorCfg := &framework.Config{
		PythonCmd:      "python3",
		NPMCmd:         "npm",
		TimeoutSeconds: 300,
	}

	// Create attest client
	attestClient := attest.NewClient(&attest.Config{
		BaseURL: cfg.AttestorURL,
	})

	// Create blockchain client
	blockchainCli := blockchain.NewClient(&blockchain.Config{
		Endpoint: cfg.BlockchainURL,
	})

	// Create storage client
	storageCli := storage.NewClient(&storage.Config{
		Endpoint: cfg.StorageEndpoint,
	})

	return &Orchestrator{
		initServer:    initServer,
		sealedState:   sealedState, // Use shared state
		frameworkInst: framework.NewInstaller(orchestratorCfg),
		configManager: config.NewManager(&config.ManagerConfig{
			StorageEndpoint: cfg.StorageEndpoint,
		}),
		attestClient:  attestClient,
		blockchainCli: blockchainCli,
		storageCli:    storageCli,
		processMgr:    process.NewManager(nil),
	}
}

// logf logs to both stdout and the init server's log buffer (for /dashboard)
// This replaces direct log.Printf calls to enable dashboard viewing.
func (o *Orchestrator) logf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	// Always output to stdout (existing behavior)
	log.Println(msg)
	// Also log to init server for dashboard viewing
	if o.initServer != nil {
		o.initServer.Log(msg)
	}
}

// Run runs the initialization flow
func (o *Orchestrator) Run(ctx context.Context) error {
	o.logf("Starting initialization flow...")

	// Step 1: Wait for initialization via HTTP
	o.logf("Step 1: Waiting for HTTP initialization...")
	initState, err := o.waitForInit(ctx)
	if err != nil {
		return fmt.Errorf("wait for init failed: %w", err)
	}

	// Step 2: Generate key pair
	o.logf("Step 2: Generating key pair...")
	pubKey, _, err := sealed.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate key pair failed: %w", err)
	}

	// Initialize sealed state
	if err := o.sealedState.Initialize(initState.SealID, initState.TempKey, initState.AttestorURL); err != nil {
		return fmt.Errorf("initialize sealed state failed: %w", err)
	}

	o.logf("Public key generated: %s", pubKey)
	o.logf("Seal ID: %s", initState.SealID)

	// Step 3: Enter sealed state
	o.logf("Step 3: Entering sealed state...")
	o.initServer.SetStatus("sealed")

	// Check for demo mode
	demoMode := os.Getenv("DEMO_MODE") == "true"

	if demoMode {
		o.logf("Demo mode detected - skipping external service calls")

		// Generate mock agent seal key for demo
		mockAgentSealKey := make([]byte, 32)
		for i := range mockAgentSealKey {
			mockAgentSealKey[i] = byte(i % 256)
		}
		o.sealedState.SetAgentSealKey(mockAgentSealKey)

		// Set mock agent ID
		mockAgentID := "demo-agent-" + initState.SealID[:16]
		o.sealedState.SetAgentID(mockAgentID)

		o.logf("Demo mode - using mock agent ID: %s", mockAgentID)
		o.initServer.SetStatus("attested")
	} else {
		// Step 4: Perform remote attestation (unseal agentSeal key)
		o.logf("Step 4: Performing remote attestation...")
		o.initServer.SetStatus("attesting")

		// Set TEE private key for signing attestor requests
		teePrivKeyBytes := o.sealedState.GetPrivateKeyBytes()
		if err := o.attestClient.SetTEEKeyFromHex(hex.EncodeToString(teePrivKeyBytes)); err != nil {
			return fmt.Errorf("set TEE private key failed: %w", err)
		}

		// Get image hash (in production, get from actual container)
		imageHash := o.getImageHash()

		// Perform attestation
		agentSealKey, err := o.attestClient.GetAgentSealKey(
			initState.SealID,
			o.sealedState.GetPublicKey(),
			imageHash,
		)
		if err != nil {
			return fmt.Errorf("unseal failed: %w", err)
		}

		// Store agentSeal key
		o.sealedState.SetAgentSealKey(agentSealKey)

		o.logf("Unseal successful, agentSeal key received")
		o.initServer.SetStatus("attested")

		// Step 5: Query agentId from sealId
		o.logf("Step 5: Querying agentId from sealId...")
		o.initServer.SetStatus("querying_agent")

		agentID, err := o.blockchainCli.GetAgentIdBySealId(initState.SealID)
		if err != nil {
			return fmt.Errorf("get agentId by sealId failed: %w", err)
		}
		o.sealedState.SetAgentID(agentID)
		o.logf("Agent ID: %s", agentID)

		o.sealedState.SetAgentID(agentID)
		o.logf("Agent ID: %s", agentID)

		// Step 6: Get sealed keys from ITransferred event
		o.logf("Step 6: Getting sealed keys from ITransferred event...")
		o.initServer.SetStatus("fetching_sealed_keys")

		sealedKeys, err := o.blockchainCli.GetSealedKeys(o.getRPCURL(), o.getContractAddr(), agentID)
		if err != nil {
			o.logf("Warning: Could not get sealed keys from blockchain: %v", err)
			// Fall back to legacy flow
		} else {
			o.logf("Found %d sealed key(s)", len(sealedKeys))
		}

		// Step 7: Fetch intelligent datas from blockchain
		o.logf("Step 7: Fetching intelligent datas...")
		intelligentDatas, err := o.blockchainCli.GetIntelligentDatas(agentID)
		if err != nil {
			return fmt.Errorf("get intelligent datas failed: %w", err)
		}
		if len(intelligentDatas) == 0 {
			return fmt.Errorf("no intelligent datas found for agent")
		}
		o.logf("Found %d intelligent data(s)", len(intelligentDatas))

		// Step 8: Process intelligent data (download + decrypt)
		o.logf("Step 8: Processing intelligent data...")
		o.initServer.SetStatus("processing_data")

		agentSealPrivKey := o.sealedState.GetAgentSealKey()
		if agentSealPrivKey == nil {
			return fmt.Errorf("agentSeal private key not available")
		}

		// Convert agentSealKey bytes to secp256k1 ECDSA private key for decryption
		agentSealPrivECDSA, err := attest.BytesToSecp256k1PrivateKey(agentSealPrivKey)
		if err != nil {
			return fmt.Errorf("convert agentSeal key to ECDSA: %w", err)
		}

		// Try to process with sealed keys first (new flow)
		var configFound bool
		if sealedKeys != nil && len(sealedKeys) > 0 {
			for i, idata := range intelligentDatas {
				// Parse dataHash to [32]byte
				var dataHash [32]byte
				hashHex := strings.TrimPrefix(idata.DataHash, "0x")
				hexBytes, err := hex.DecodeString(hashHex)
				if err != nil {
					o.logf("Warning[%d]: invalid dataHash: %v", i, err)
					continue
				}
				copy(dataHash[:], hexBytes)

				// Get sealed key for this dataHash
				sealedKey, ok := sealedKeys[dataHash]
				if !ok {
					o.logf("Warning[%d]: no sealed key found for dataHash %s", i, idata.DataHash)
					continue
				}

				// Decrypt sealedKey -> dataKey (ECIES with secp256k1)
				dataKey, err := attest.DecryptSealedKey(sealedKey, agentSealPrivECDSA)
				if err != nil {
					o.logf("Warning[%d]: ECIES decrypt sealed key failed: %v", i, err)
					continue
				}

				// Download encrypted file from storage
				encryptedFile, err := o.storageCli.DownloadFile(idata.DataHash)
				if err != nil {
					o.logf("Warning[%d]: download file failed: %v", i, err)
					continue
				}

				// Decrypt file -> config (AES-GCM-256)
				// Format: nonce(12) || ciphertext+tag(16 at end)
				if len(encryptedFile) < 12+16 {
					o.logf("Warning[%d]: file too short (%d bytes)", i, len(encryptedFile))
					continue
				}

				decryptedConfig, err := o.configManager.DecryptConfig(encryptedFile, dataKey)
				if err != nil {
					o.logf("Warning[%d]: decrypt config failed: %v", i, err)
					continue
				}

				// Success! Use this config
				o.agentConfig = decryptedConfig
				configFound = true
				o.logf("Successfully processed intelligent data[%d]", i)
				break
			}
		}

		// Fallback: use first intelligent data's hash with legacy flow
		if !configFound {
			o.logf("Falling back to legacy config flow...")
			configHash := intelligentDatas[0].DataHash
			o.logf("Config hash: %s", configHash)

			// Step 9: Fetch encrypted config from Storage
			o.logf("Step 9: Fetching encrypted config from Storage...")
			o.initServer.SetStatus("fetching_config")

			encryptedConfig, err := o.storageCli.FetchConfig(configHash)
			if err != nil {
				// For demo: use default config if storage fetch fails
				o.logf("Warning: Could not fetch config from Storage: %v, using defaults", err)
				o.agentConfig = o.defaultConfig()
			} else {
				// Step 10: Decrypt config using agentSeal private key (legacy)
				o.logf("Step 10: Decrypting config (legacy)...")
				aesKey, err := o.deriveAESKey(agentSealPrivKey)
				if err != nil {
					o.logf("Warning: Could not derive AES key: %v, using defaults", err)
					o.agentConfig = o.defaultConfig()
				} else {
					o.agentConfig, err = o.configManager.DecryptConfig(encryptedConfig, aesKey)
					if err != nil {
						o.logf("Warning: Could not decrypt config: %v, using defaults", err)
						o.agentConfig = o.defaultConfig()
					} else {
						o.logf("Config decrypted successfully")
					}
				}
			}
		}

		// Step 11: Report status to Attestor
		o.logf("Step 11: Reporting status to Attestor...")
		if err := o.reportStatusToAttestor("ready", ""); err != nil {
			o.logf("Warning: Failed to report status: %v", err)
		}
	}
	if demoMode {
		o.logf("Demo mode - using default config")
		o.agentConfig = o.defaultConfig()
	}

	// Validate config
	if err := config.ValidateConfig(o.agentConfig); err != nil {
		o.logf("Warning: Config validation failed: %v, using defaults", err)
		o.agentConfig = o.defaultConfig()
	}

	// Step 9: Install framework
	o.logf("Step 9: Installing framework...")
	o.initServer.SetStatus("installing_framework")

	if o.agentConfig.Framework != nil && o.agentConfig.Framework.Name != "demo" {
		fwName := o.agentConfig.Framework.Name
		fwVersion := o.agentConfig.Framework.Version

		if fwVersion == "" {
			fwVersion = "latest" // Default version if not specified
		}

		o.logf("Installing framework: %s==%s", fwName, fwVersion)
		err = o.installFramework(fwName, fwVersion)
		if err != nil {
			o.logf("Warning: Framework install failed: %v, continuing anyway", err)
		} else {
			o.logf("Framework %s==%s installed", fwName, fwVersion)
		}
	} else {
		// Demo mode: test framework installation if config specifies a framework
		if o.agentConfig.Framework != nil && o.agentConfig.Framework.Name != "" {
			fwName := o.agentConfig.Framework.Name
			fwVersion := o.agentConfig.Framework.Version
			if fwVersion == "" {
				fwVersion = "0.1.0"
			}
			o.logf("Demo mode: attempting to install framework %s==%s", fwName, fwVersion)
			err := o.installFramework(fwName, fwVersion)
			if err != nil {
				o.logf("Demo mode: framework install result: %v", err)
			} else {
				o.logf("Demo mode: framework %s verified", fwName)
			}
		} else {
				o.logf("Demo mode detected, no framework specified")
		}
	}

	// Step 10: Start agent
	o.logf("Step 10: Starting agent process...")
	o.initServer.SetStatus("starting_agent")
	err = o.startAgent(ctx, o.agentConfig)
	if err != nil {
		return fmt.Errorf("start agent failed: %w", err)
	}
	o.logf("Agent started on port %s", o.getAgentPort())

	// Step 11: Ready
	o.initServer.SetStatus("ready")
	o.mu.Lock()
	o.flowComplete = true
	o.mu.Unlock()

	o.logf("Initialization flow complete. Agent ready.")

	return nil
}

// waitForInit waits for HTTP initialization
func (o *Orchestrator) waitForInit(ctx context.Context) (*initpkg.InitState, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if o.initServer.IsInitialized() {
				o.mu.Lock()
				o.initialized = true
				o.mu.Unlock()
				return o.initServer.GetState(), nil
			}
		}
	}
}

// defaultConfig returns default agent config
func (o *Orchestrator) defaultConfig() *config.AgentConfig {
	// Check for demo mode
	demoMode := os.Getenv("DEMO_MODE") == "true"

	if demoMode {
		// Demo mode with openclaw framework for testing
		o.logf("Demo mode - using openclaw framework for testing")
		return &config.AgentConfig{
			Framework: &config.Framework{
				Name:    "openclaw",
				Version: "0.1.0",
			},
			Runtime: &config.Runtime{
				EntryPoint: "python3 -m openclaw.agent",
				WorkingDir: "/app",
				AgentPort:  9000,
			},
			Env: map[string]string{
				"LOG_LEVEL":     "debug",
				"FRAMEWORK":     "openclaw",
				"AGENT_VERSION": "0.1.0",
				"AGENT_ID":      "demo-openclaw-agent",
			},
		}
	}

	// Production config
	return &config.AgentConfig{
		Framework: &config.Framework{
			Name:    "openclaw",
			Version: "0.1.0",
		},
		Runtime: &config.Runtime{
			EntryPoint: "python3 main.py",
			WorkingDir: "/app",
			AgentPort:  9000,
		},
		Env: map[string]string{
			"LOG_LEVEL": "info",
		},
	}
}

// findDemoAgent looks for the demo agent script
func (o *Orchestrator) findDemoAgent() string {
	// Possible locations for demo agent
	locations := []string{
		"./examples/demo-agent.py",
		"../examples/demo-agent.py",
		"../../examples/demo-agent.py",
		"/app/examples/demo-agent.py",
	}

	// Also check current directory and traverse up
	cwd, err := os.Getwd()
	if err == nil {
		// Check from cwd going up the directory tree
		for i := 0; i < 4; i++ {
			path := cwd + strings.Repeat("/..", i) + "/examples/demo-agent.py"
			locations = append(locations, path)

			// Also try direct join
			directPath := filepath.Join(cwd, "examples/demo-agent.py")
			locations = append(locations, directPath)

			// Go up one directory
			cwd = filepath.Dir(cwd)
		}
	}

	for _, loc := range locations {
		absPath, err := filepath.Abs(loc)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			return absPath
		}
	}

	return ""
}

// installFramework installs the agent framework
func (o *Orchestrator) installFramework(fwName, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := o.frameworkInst.Install(ctx, fwName, version)
	if err != nil {
		return fmt.Errorf("framework install failed: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("framework install failed: %s", result.Error)
	}

	o.logf("Framework installed in %dms", result.Duration)
	return nil
}

// startAgent starts the agent process
func (o *Orchestrator) startAgent(ctx context.Context, agentConfig *config.AgentConfig) error {
	// Use the process manager to start the agent
	if err := o.processMgr.Start(ctx, agentConfig); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	return nil
}

// deriveAESKey derives an AES key from the ECDSA private key
// This is a simplified implementation - in production, use proper KDF
func (o *Orchestrator) deriveAESKey(privKey []byte) ([]byte, error) {
	// For simplicity, we'll use the first 32 bytes of the private key
	// In production, use a proper KDF like HKDF
	if len(privKey) < 32 {
		return nil, fmt.Errorf("private key too short for AES key derivation")
	}
	return privKey[:32], nil
}

// getAgentPort returns the agent port from config or default
func (o *Orchestrator) getAgentPort() string {
	if o.agentConfig != nil && o.agentConfig.Runtime != nil {
		return o.agentConfig.Runtime.GetAgentPort()
	}
	return "9000" // default
}

// Stop stops the orchestrator and cleans up
func (o *Orchestrator) Stop() {
	if o.processMgr != nil {
		o.logf("Stopping agent process...")
		o.processMgr.Stop()
	}
}

// IsFlowComplete returns whether the flow is complete
func (o *Orchestrator) IsFlowComplete() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.flowComplete
}

// GetAgentConfig returns the agent config
func (o *Orchestrator) GetAgentConfig() *config.AgentConfig {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.agentConfig
}

// GetAgentPort returns the agent port
func (o *Orchestrator) GetAgentPort() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.getAgentPort()
}

// getImageHash returns the container image hash
// Reads from IMAGE_HASH environment variable set by TEE runtime
func (o *Orchestrator) getImageHash() string {
	// Try environment variable first
	imageHash := os.Getenv("IMAGE_HASH")
	if imageHash != "" {
		// Ensure it has 0x prefix for consistency
		if !strings.HasPrefix(imageHash, "0x") {
			imageHash = "0x" + imageHash
		}
		return imageHash
	}

	// Fallback: try reading from /proc/self/environ (TEE containers)
	data, err := os.ReadFile("/proc/self/environ")
	if err == nil {
		envPairs := strings.Split(string(data), "\x00")
		for _, pair := range envPairs {
			if strings.HasPrefix(pair, "IMAGE_HASH=") {
				hash := strings.TrimPrefix(pair, "IMAGE_HASH=")
				if hash != "" && !strings.HasPrefix(hash, "0x") {
					hash = "0x" + hash
				}
				return hash
			}
		}
	}

	// Final fallback: use development placeholder
	o.logf("WARNING: IMAGE_HASH not set, using development placeholder")
	return "0x" + hex.EncodeToString([]byte("dev-image-hash-placeholder"))
}

// getRPCURL returns the Ethereum RPC URL for ITransferred event scanning
func (o *Orchestrator) getRPCURL() string {
	if o.blockchainCli != nil {
		return o.blockchainCli.GetRPCURL()
	}
	return os.Getenv("RPC_URL")
}

// getContractAddr returns the AgenticID contract address
func (o *Orchestrator) getContractAddr() string {
	if addr := os.Getenv("CONTRACT_ADDR"); addr != "" {
		return addr
	}
	return os.Getenv("AGENTIC_ID_CONTRACT")
}

// reportStatusToAttestor sends a status report to the Attestor service
func (o *Orchestrator) reportStatusToAttestor(status, errorDetail string) error {
	agentSealPrivKey := o.sealedState.GetAgentSealKey()
	if agentSealPrivKey == nil {
		return fmt.Errorf("agentSeal key not set")
	}

	// Convert to secp256k1 ECDSA private key for signing
	agentSealPrivECDSA, err := attest.BytesToSecp256k1PrivateKey(agentSealPrivKey)
	if err != nil {
		return fmt.Errorf("convert agentSeal key: %w", err)
	}

	sealID := o.sealedState.GetSealID()
	// Strip 0x prefix for reportStatus
	sealIDHex := strings.TrimPrefix(sealID, "0x")

	_, err = o.attestClient.ReportStatus(agentSealPrivECDSA, sealIDHex, status, errorDetail)
	return err
}
