# Agent Wrapper Architecture

This document provides a complete overview of the Agent Wrapper architecture with visual diagrams.

## Table of Contents

- [Complete Architecture](#complete-architecture)
- [Component Overview](#component-overview)
- [Startup Sequence](#startup-sequence)
- [Request Flow](#request-flow)
- [State Management](#state-management)
- [Data Distribution](#data-distribution)

## Complete Architecture

```mermaid
graph TB
    subgraph "External World"
        Dev["Agent Developer"]
        Sandbox["0g-sandbox"]
        User["End User"]
    end

    subgraph "Blockchain"
        Chain["Agent Metadata (ERC-7857 + ERC-8004 + Agentic)<br/>{agentId, sealId, agentSeal}<br/>IntelligentData[{dataDescription, dataHash}]"]
    end

    subgraph "External Services"
        Attestor["Attestor Service<br/>/attest, /key"]
        Storage["0G Storage<br/>Encrypted Config"]
        PyPI["PyPI / npm<br/>Framework Packages"]
    end

    subgraph "TEE Environment - 0g-sandbox"
        subgraph "Container Instance"
            subgraph "Agent Wrapper (Go)"
                HTTPInit["HTTP Init Server<br/>/_internal/init"]
                Orchestrator["Flow Orchestrator"]
                SealedState["Sealed State<br/>Key Pair, agentSealKey, agentId"]
                AttestClient["Attestor Client"]
                BlockchainClient["Blockchain Client"]
                FetchMeta["Fetch Metadata<br/>from Blockchain"]
                FetchConfig["Fetch Config<br/>from Storage"]
                Decrypt["Decrypt Config<br/>with agentSealKey"]
                FrameworkMgr["Framework Manager"]
                InstallFW["Install Framework<br/>pip/npm install"]
                ProcessMgr["Process Manager"]
                StartAgent["Start Agent"]
                Proxy["HTTP Proxy :8080"]
            end

            subgraph "Agent Framework"
                Agent["OpenClaw / Eliza / Custom<br/>Dynamically Installed<br/>HTTP Server :9000"]
            end
        end
    end

    %% Registration Flow
    Dev -->|"Register Agent<br/>(on-chain)"| Chain
    Dev -->|"Upload Config<br/>(encrypted)"| Storage

    %% Startup Flow
    Sandbox -->|"Start Container<br/>(official image)"| HTTPInit
    Dev -->|"POST /_internal/init<br/>{sealId, tempKey, attestorUrl}"| HTTPInit
    HTTPInit -->|"Trigger Flow"| Orchestrator
    Orchestrator -->|"Generate Keys"| SealedState
    Orchestrator -->|"Attest"| AttestClient
    AttestClient -->|"Get agentSealKey"| Attestor
    Attestor -->|"agentSealKey"| SealedState
    Orchestrator -->|"Query sealId→agentId"| BlockchainClient
    BlockchainClient --> Chain
    Chain -->|"agentId"| Orchestrator
    Orchestrator -->|"Get IntelligentData[]"| BlockchainClient
    BlockchainClient -->|"dataHash"| Orchestrator
    Orchestrator -->|"Fetch Config"| FetchConfig
    FetchConfig -->|"Encrypted Config"| Storage
    Storage --> Decrypt
    Decrypt -->|"AgentConfig"| FrameworkMgr
    FrameworkMgr -->|"Install"| InstallFW
    InstallFW -->|"Packages"| PyPI
    InstallFW --> ProcessMgr
    ProcessMgr --> StartAgent
    StartAgent --> Agent

    %% Runtime Flow
    User -->|"Request /chat"| Proxy
    Proxy -->|"Forward"| Agent
    Agent -->|"Response"| Proxy
    Proxy -->|"Sign with agentSealKey"| SealedState
    Proxy -->|"Signed Response"| User

    %% Styling
    classDef wrapper fill:#4CAF50,stroke:#2E7D32,color:#fff
    classDef agent fill:#2196F3,stroke:#1565C0,color:#fff
    classDef external fill:#FF9800,stroke:#E65100,color:#fff
    classDef blockchain fill:#9C27B0,stroke:#6A1B9A,color:#fff
    classDef package fill:#FF5722,stroke:#C62828,color:#fff

    class HTTPInit,Orchestrator,SealedState,AttestClient,BlockchainClient,FetchMeta,FetchConfig,Decrypt,FrameworkMgr,InstallFW,ProcessMgr,StartAgent,Proxy wrapper
    class Agent agent
    class Attestor,Storage,Sandbox external
    class Chain blockchain
    class PyPI package
```

## Component Overview

### Module Structure

```
internal/
├── attest/       # Attestor service client
├── blockchain/   # Blockchain client (ERC-7857/8004 queries)
├── config/       # Configuration encryption/decryption
├── flow/         # Initialization flow orchestrator
├── framework/    # Dynamic framework installation
├── init/         # HTTP initialization server
├── mock/         # Mock HTTP server for testing
├── process/      # Agent process management
├── proxy/        # HTTP proxy with ECDSA signing
└── sealed/       # Sealed state (keys, IDs)
```

### Module Responsibilities

| Module | Responsibility |
|--------|---------------|
| `internal/init` | HTTP server receiving init parameters |
| `internal/sealed` | Thread-safe key storage (key pair, agentSealKey, agentId) |
| `internal/attest` | Remote attestation with Attestor service |
| `internal/blockchain` | Query agentId and IntelligentData from blockchain |
| `internal/storage` | Fetch encrypted config from 0G Storage |
| `internal/config` | Decrypt and validate agent configuration |
| `internal/framework` | Dynamically install Python/Node.js frameworks |
| `internal/process` | Start/stop agent process with monitoring |
| `internal/proxy` | HTTP proxy with ECDSA response signing |
| `internal/flow` | Orchestrate complete initialization sequence |
| `internal/mock` | HTTP mock server for testing |

## Startup Sequence

```mermaid
sequenceDiagram
    participant S as 0g-sandbox
    participant W as Wrapper
    participant D as Developer
    participant A as Attestor
    participant B as Blockchain
    participant ST as 0G Storage
    participant PM as PyPI/npm
    participant AG as Agent

    S->>W: ① Start Container<br/>(official image)
    activate W
    W->>W: Start HTTP server<br/>:8080
    W-->>D: Waiting for init

    D->>W: ② POST /_internal/init<br/>{sealId, tempKey, attestorUrl}
    W->>W: ③ Generate key pair

    alt Demo Mode
        W->>W: Skip external calls
        W->>W: Use mock data
    else Production Mode
        W->>A: ④ Attest(sealId, pubkey, imageHash)
        A->>W: attestationToken

        W->>A: ⑤ GetKey(token)
        A->>W: agentSeal Private Key

        W->>B: ⑥ GetAgentIdBySealId(sealId)
        B->>W: agentId

        W->>B: ⑦ GetIntelligentDatas(agentId)
        B->>W: [{dataDescription, dataHash}]

        W->>ST: ⑧ FetchConfig(dataHash)
        ST->>W: encryptedConfig

        W->>W: ⑨ Decrypt(agentSealKey)

        W->>PM: ⑩ Install Framework(framework)
        PM->>W: packages installed
    end

    W->>AG: ⑪ Start(entryPoint)
    AG->>W: Ready on :9000

    W->>W: ⑫ Ready to serve
    deactivate W
```

## Request Flow

```mermaid
sequenceDiagram
    participant U as User
    participant P as Proxy (:8080)
    participant S as Sealed State
    participant A as Agent (:9000)

    U->>P: Request /chat

    P->>A: Forward Request

    A->>A: Process
    A->>P: Response

    P->>S: Get agentSealKey
    S->>P: agentSealKey

    P->>P: Sign(agentId, sealId, ts, response)

    P->>U: Signed Response<br/>X-Agent-Id, X-Signature, X-Timestamp
```

## State Management

```mermaid
stateDiagram-v2
    [*] --> waiting_init: Container Start
    waiting_init --> sealed: POST /_internal/init
    sealed --> attesting: Start Attestation
    attesting --> querying_agent: Attestation Complete
    querying_agent --> fetching_config: Got agentId
    fetching_config --> installing_framework: Got Config
    installing_framework --> starting_agent: Framework Ready
    starting_agent --> ready: Agent Running

    ready --> [*]: Shutdown

    note right of waiting_init
        HTTP server listening
        Waiting for init request
    end note

    note right of sealed
        Key pair generated
        Waiting for attestation
    end note

    note right of ready
        Agent process running
        Proxy active on :8080
    end note
```

## Data Distribution

### On-Chain vs Off-Chain

```mermaid
graph TB
    subgraph OnChain["Blockchain On-Chain"]
        AgentId[agentId: uint256]
        SealId[sealId: bytes32]
        AgentSeal[agentSeal: address]
        DataHash[dataHash: bytes32<br/>in intelligentData[0]]
    end

    subgraph OffChain["0G Storage Off-Chain (AES-256-GCM encrypted)"]
        EncryptedConfig[Encrypted Config]
        Framework[framework: {name, version}]
        Runtime[runtime: {entryPoint, workingDir, agentPort}]
        Env[env: {key-value pairs}]
    end

    subgraph RuntimeTEE["Runtime TEE Internal"]
        ImageHash[imageHash: from container]
        AgentSealKey[agentSealKey: private key<br/>never leaves TEE]
    end

    DataHash -->|fetch by hash| EncryptedConfig
    EncryptedConfig -->|decrypt with agentSealKey| Framework
    EncryptedConfig -->|decrypt with agentSealKey| Runtime
    EncryptedConfig -->|decrypt with agentSealKey| Env

    style OnChain fill:#e8f5e9
    style OffChain fill:#fff3e0
    style RuntimeTEE fill:#e1f5fe
```

### Data Flow Diagram

```mermaid
flowchart TD
    Start([Start: sealId]) --> QueryAgent[Query agentId]
    QueryAgent --> Blockchain{Blockchain Service}
    Blockchain -->|GET /agents/by-seal-id/{sealId}| GetAgentId[Get agentId]

    GetAgentId --> QueryData[Query IntelligentData]
    QueryData --> Blockchain2{Blockchain Service}
    Blockchain2 -->|GET /agents/{id}/intelligent-datas| DataList[IntelligentData[]]

    DataList --> Extract[Extract dataHash[0]]
    Extract --> FetchConfig[Fetch Encrypted Config]

    FetchConfig --> Storage{0G Storage}
    Storage -->|GET /config/{hash}| Encrypted[Encrypted Config]

    Encrypted --> Decrypt{Decrypt with agentSealKey<br/>AES-256-GCM}
    Decrypt --> AgentConfig[AgentConfig]

    AgentConfig --> Parse{Parse Fields}
    Parse --> Framework[framework]
    Parse --> Runtime[runtime]
    Parse --> Env[env]

    Framework --> Start[Start Agent]
    Runtime --> Start
    Env --> Start

    Start --> Ready([Ready])

    style Blockchain fill:#e8f5e9
    style Blockchain2 fill:#e8f5e9
    style Storage fill:#fff3e0
    style Decrypt fill:#e1f5fe
    style Start fill:#c8e6c9
```

## HTTP API Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant W as Wrapper
    participant A as Attestor

    Note over C,W: Initialization Phase
    C->>W: POST /_internal/init<br/>{sealId, tempKey, attestorUrl}
    W->>W: Validate params<br/>Generate key pair
    W-->>C: 200 OK<br/>{status: "sealed"}

    Note over W,A: Attestation Phase (Production only)
    W->>A: POST /attest<br/>{sealId, pubKey, imageHash}
    A-->>W: {token}

    W->>A: POST /key<br/>Authorization: Bearer {token}
    A-->>W: {agentSealPrivateKey}

    Note over C,W: Runtime Phase
    C->>W: GET /chat?msg=hello
    W->>W: Forward to Agent<br/>Sign response
    W-->>C: 200 OK<br/>X-Signature: ...
```

## Dynamic Framework Installation

```mermaid
flowchart TD
    Start([Get Framework Config]) --> Check{Framework<br/>Supported?}

    Check -->|Python| Pip[pip install<br/>{framework}=={version}]
    Check -->|Node.js| Npm[npm install<br/>{framework}@{version}]
    Check -->|demo/unknown| Skip[Skip installation]

    Pip --> Verify{Success?}
    Npm --> Verify

    Verify -->|Yes| Continue[Continue startup]
    Verify -->|No| LogError[Log warning<br/>Continue anyway]

    Skip --> Continue

    LogError --> Continue
    Continue --> Done([Framework Complete])

    style Pip fill:#e8f5e9
    style Npm fill:#fff3e0
    style Skip fill:#e1f5fe
    style Continue fill:#c8e6c9
```

## Security Model

### Trust Boundaries

```mermaid
graph TB
    subgraph Untrusted["Untrusted Zone"]
        Internet["Internet / External Network"]
    end

    subgraph TEE["TEE Boundary (Gramine)"]
        subgraph WrapperTrusted["Wrapper (Trusted)"]
            Proxy["HTTP Proxy"]
            Keys["Private Keys"]
            Sign["ECDSA Signing"]
        end

        subgraph AgentSandbox["Agent (May be untrusted)"]
            AgentLogic["Business Logic"]
        end
    end

    Internet --> Proxy
    Proxy --> AgentLogic
    Keys --> Sign

    style Untrusted fill:#ffcdd2
    style WrapperTrusted fill:#c8e6c9
    style AgentSandbox fill:#fff9c4
```

### Key Isolation

- **agentSeal Private Key** - Only in Wrapper process memory
- **Agent Process** - Cannot access Wrapper memory (process isolation)
- **TEE Protection** - Keys never leave TEE boundary
- **No Logging** - Private keys never logged or serialized

## Component Relationships

```mermaid
graph LR
    subgraph Wrapper["Wrapper Modules"]
        HI[HTTP Init]
        O[Orchestrator]
        SS[Sealed State]
        AC[Attest Client]
        BC[Blockchain Client]
        SC[Storage Client]
        CM[Config Manager]
        FM[Framework Manager]
        PM[Process Manager]
        PX[HTTP Proxy]
    end

    subgraph Deps["Dependencies"]
        AT[Attestor Service]
        BS[Blockchain]
        ST[0G Storage]
        PK[PyPI/npm]
    end

    HI --> O
    O --> SS
    O --> AC
    O --> BC
    O --> SC
    O --> CM
    O --> FM
    O --> PM
    FM --> PK
    PX --> SS

    AC --> AT
    BC --> BS
    SC --> ST

    style Wrapper fill:#e8f5e9
    style Deps fill:#fff3e0
```
