# TOS Mining Pool Design

## Overview

TOS Pool is a high-performance mining pool for TOS Hash V3 algorithm, designed to work with `tosminer`. The architecture features a modular design with Redis storage and master-slave topology for scalability and geographic distribution.

## Architecture Features

| Feature | TOS Pool |
|---------|----------|
| Language | Go |
| Storage | Redis |
| Topology | Master-Slave |
| Protocol | Stratum V1 |
| Payment | PPLNS |
| Encryption | TLS |
| Frontend | Vue.js |

## System Architecture

```
                                    ┌─────────────────┐
                                    │   TOS Node      │
                                    │  (Blockchain)   │
                                    └────────┬────────┘
                                             │ JSON-RPC
                                             ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              TOS Pool Master                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │    Block     │    │   Payment    │    │     API      │                   │
│  │   Unlocker   │    │  Processor   │    │   Server     │                   │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘                   │
│         │                   │                   │                            │
│         └───────────────────┼───────────────────┘                            │
│                             │                                                │
│                    ┌────────┴────────┐                                       │
│                    │     Redis       │                                       │
│                    │   (Database)    │                                       │
│                    └────────┬────────┘                                       │
│                             │                                                │
│         ┌───────────────────┼───────────────────┐                            │
│         │                   │                   │                            │
│  ┌──────┴───────┐    ┌──────┴───────┐    ┌──────┴───────┐                   │
│  │    Slave     │    │    Slave     │    │    Slave     │                   │
│  │  Connector   │    │  Connector   │    │  Connector   │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
         ▲                    ▲                    ▲
         │ Encrypted          │ Encrypted          │ Encrypted
         │ (TLS/ChaCha20)     │                    │
         ▼                    ▼                    ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│  Slave Node 1   │   │  Slave Node 2   │   │  Slave Node N   │
│  (Asia)         │   │  (Europe)       │   │  (Americas)     │
├─────────────────┤   ├─────────────────┤   ├─────────────────┤
│ Stratum Server  │   │ Stratum Server  │   │ Stratum Server  │
│ Share Validator │   │ Share Validator │   │ Share Validator │
│ Vardiff Engine  │   │ Vardiff Engine  │   │ Vardiff Engine  │
└────────┬────────┘   └────────┬────────┘   └────────┬────────┘
         │                     │                     │
    ┌────┴────┐           ┌────┴────┐          ┌────┴────┐
    │ Miners  │           │ Miners  │          │ Miners  │
    └─────────┘           └─────────┘          └─────────┘
```

## Component Details

### 1. Master Node

Central coordinator that manages:
- Block template distribution
- Share aggregation from slaves
- Payment calculation and processing
- Block submission to TOS node
- Statistics and API

```
tos-pool/
├── cmd/
│   ├── master/
│   │   ├── main.go           # Entry point
│   │   ├── config.go         # Configuration
│   │   ├── unlocker.go       # Block maturation
│   │   ├── payout.go         # Payment processor
│   │   ├── api.go            # REST API
│   │   └── slave_handler.go  # Slave communication
│   └── slave/
│       ├── main.go           # Entry point
│       ├── stratum.go        # Stratum server
│       ├── validator.go      # Share validation
│       ├── vardiff.go        # Difficulty adjustment
│       └── master_client.go  # Master connection
```

### 2. Slave Node

Handles direct miner connections:
- Stratum protocol server
- TOS Hash V3 share validation
- Variable difficulty per miner
- Job distribution

### 3. Redis Schema

```
# Node state
tos:nodes                          → Hash {height, difficulty, lastBeat}

# Current round shares
tos:shares:roundCurrent            → Hash {address: shareCount}
tos:shares:round:{height}:{hash}   → Hash {address: shareCount}

# Hashrate tracking (sorted sets with timestamp scores)
tos:hashrate                       → SortedSet {diff:addr:worker:ms -> timestamp}
tos:hashrate:{address}             → SortedSet {worker hashrate samples}

# Block lifecycle
tos:blocks:candidates              → SortedSet {height -> blockData}
tos:blocks:immature                → SortedSet {height -> blockData}
tos:blocks:matured                 → SortedSet {height -> blockData}

# Miner data
tos:miners:{address}               → Hash {balance, immature, paid, lastShare}
tos:miners:{address}:workers       → Hash {worker: lastSeen}

# Payment tracking
tos:payments:pending               → SortedSet {txHash -> paymentData}
tos:payments:all                   → SortedSet {timestamp -> txHash}
tos:payments:{address}             → List [payment records]

# Pool stats
tos:stats                          → Hash {roundShares, lastBlockFound, totalPaid}
tos:finders                        → SortedSet {address -> blocksFound}

# Access control
tos:blacklist                      → Set {banned addresses}
tos:whitelist                      → Set {whitelisted addresses}
```

### 4. Stratum Protocol

TOS Pool uses standard Stratum V1 protocol for compatibility:

```json
// Miner -> Pool: Subscribe
{"id": 1, "method": "mining.subscribe", "params": ["tosminer/1.0.0"]}

// Pool -> Miner: Subscribe response
{"id": 1, "result": [[["mining.notify", "subscription_id"]], "extranonce1", 4], "error": null}

// Miner -> Pool: Authorize
{"id": 2, "method": "mining.authorize", "params": ["wallet_address.worker_name", "x"]}

// Pool -> Miner: Job notification
{
  "id": null,
  "method": "mining.notify",
  "params": [
    "job_id",           // Job identifier
    "prev_hash",        // Previous block hash (32 bytes hex)
    "merkle_root",      // Merkle root (32 bytes hex)
    "timestamp",        // Block timestamp (4 bytes hex)
    "target",           // Compact target (4 bytes hex)
    true                // Clean jobs flag
  ]
}

// Miner -> Pool: Submit share
{"id": 4, "method": "mining.submit", "params": ["wallet.worker", "job_id", "nonce"]}

// Pool -> Miner: Set difficulty
{"id": null, "method": "mining.set_difficulty", "params": [difficulty]}
```

### 5. Share Validation (TOS Hash V3)

```go
// Share validation pipeline
func ValidateShare(share *Share) (bool, bool) {
    // 1. Reconstruct block header
    header := BuildHeader(share.JobID, share.Nonce, share.ExtraNonce)

    // 2. Compute TOS Hash V3
    hash := toshash.Hash(header)

    // 3. Check share difficulty
    shareDiff := CalculateDifficulty(hash)
    if shareDiff < share.MinDifficulty {
        return false, false // Invalid share
    }

    // 4. Check block difficulty
    if shareDiff >= share.NetworkDifficulty {
        // BLOCK FOUND!
        return true, true
    }

    return true, false // Valid share, not a block
}
```

### 6. Variable Difficulty (Vardiff)

Targets 10-20 shares per minute per worker:

```go
type Vardiff struct {
    MinDiff      float64       // Minimum difficulty (1000)
    MaxDiff      float64       // Maximum difficulty (1e12)
    TargetTime   float64       // Target seconds between shares (4)
    RetargetTime float64       // Retarget interval (90s)
    VariancePercent float64    // Allowed variance (30%)
}

func (v *Vardiff) Adjust(worker *Worker) float64 {
    // Calculate actual share rate
    elapsed := time.Since(worker.LastRetarget).Seconds()
    if elapsed < v.RetargetTime {
        return worker.Difficulty
    }

    shareRate := float64(worker.SharesSinceRetarget) / elapsed
    targetRate := 1.0 / v.TargetTime

    // Calculate new difficulty
    ratio := shareRate / targetRate
    newDiff := worker.Difficulty * ratio

    // Apply variance dampening
    if ratio > 1+v.VariancePercent/100 {
        newDiff = worker.Difficulty * (1 + v.VariancePercent/100)
    } else if ratio < 1-v.VariancePercent/100 {
        newDiff = worker.Difficulty * (1 - v.VariancePercent/100)
    }

    // Clamp to bounds
    return math.Max(v.MinDiff, math.Min(v.MaxDiff, newDiff))
}
```

### 7. PPLNS Payment Scheme

Pay Per Last N Shares - rewards based on shares in a sliding window:

```go
const PPLNSWindow = 2.0 // Window multiplier (2x network difficulty)

func CalculateRewards(block *Block) map[string]uint64 {
    // Window size = 2 * network_difficulty worth of shares
    windowSize := uint64(float64(block.Difficulty) * PPLNSWindow)

    // Get shares in window
    shares := GetSharesInWindow(windowSize)

    // Calculate total score
    var totalScore float64
    for _, share := range shares {
        totalScore += float64(share.Difficulty)
    }

    // Calculate rewards
    rewards := make(map[string]uint64)
    blockReward := block.Reward * (100 - PoolFeePercent) / 100

    for _, share := range shares {
        score := float64(share.Difficulty) / totalScore
        rewards[share.Address] += uint64(float64(blockReward) * score)
    }

    return rewards
}
```

### 8. Block Maturation

Wait for confirmations before crediting rewards:

```go
const (
    ImmatureDepth = 10   // Blocks before marking immature
    MatureDepth   = 100  // Blocks before marking mature
)

func ProcessBlocks(currentHeight uint64) {
    // Get candidate blocks
    candidates := redis.GetCandidates()

    for _, block := range candidates {
        confirmations := currentHeight - block.Height

        if confirmations >= MatureDepth {
            // Check if block is still in main chain
            if IsMainChain(block.Hash) {
                // Credit miners with confirmed balance
                CreditMiners(block)
                redis.MoveToMatured(block)
            } else {
                // Orphan block
                redis.RemoveCandidate(block)
            }
        } else if confirmations >= ImmatureDepth {
            // Credit miners with immature balance
            CreditImmature(block)
            redis.MoveToImmature(block)
        }
    }
}
```

### 9. API Endpoints

```
GET  /api/stats              # Pool statistics
GET  /api/blocks             # Recent blocks
GET  /api/payments           # Recent payments
GET  /api/miners/{address}   # Miner statistics
GET  /api/workers/{address}  # Worker details

Response: /api/stats
{
    "pool": {
        "hashrate": 125000000000,
        "miners": 1250,
        "workers": 3400,
        "blocks_found": 156,
        "last_block_time": 1702900000
    },
    "network": {
        "height": 500000,
        "difficulty": 12500000000000,
        "hashrate": 890000000000000
    },
    "prices": {
        "usd": 0.05
    }
}

Response: /api/miners/{address}
{
    "hashrate": 50000000,
    "hashrate_24h": 48000000,
    "shares": {
        "valid": 15000,
        "invalid": 12,
        "stale": 45
    },
    "balance": {
        "confirmed": 125000000,
        "immature": 50000000,
        "paid": 500000000
    },
    "workers": [
        {"name": "rig1", "hashrate": 25000000, "last_seen": 1702900000},
        {"name": "rig2", "hashrate": 25000000, "last_seen": 1702899990}
    ],
    "payments": [
        {"txid": "abc...", "amount": 100000000, "timestamp": 1702800000}
    ]
}
```

## Configuration

```yaml
# config.yaml

# Pool identity
pool:
  name: "TOS Mining Pool"
  fee: 1.0                    # 1% pool fee
  fee_address: "tos1..."      # Fee recipient

# TOS node connection
node:
  url: "http://127.0.0.1:8545"
  timeout: 10s

# Redis database
redis:
  url: "127.0.0.1:6379"
  password: ""
  db: 0

# Master server
master:
  bind: "0.0.0.0:3221"
  secret: "shared_secret_for_slaves"

# Slave servers (run separately)
slave:
  master_url: "master.pool.com:3221"
  stratum_bind: "0.0.0.0:3333"
  stratum_tls_bind: "0.0.0.0:3334"
  tls_cert: "/path/to/cert.pem"
  tls_key: "/path/to/key.pem"

# Mining settings
mining:
  initial_difficulty: 1000000
  min_difficulty: 1000
  max_difficulty: 1000000000000
  vardiff_target_time: 4        # 4 seconds per share
  vardiff_retarget: 90          # Retarget every 90 seconds
  vardiff_variance: 30          # 30% variance tolerance

# Share validation
validation:
  trust_threshold: 50           # Shares before trusting miner
  trust_check_percent: 75       # Check 75% of trusted shares

# Block processing
unlocker:
  enabled: true
  interval: 60                  # Check every 60 seconds
  immature_depth: 10
  mature_depth: 100

# Payment processing
payouts:
  enabled: true
  interval: 3600                # Process every hour
  threshold: 100000000          # 0.1 TOS minimum
  max_addresses_per_tx: 100

# API server
api:
  enabled: true
  bind: "0.0.0.0:8080"
  stats_cache: 10               # Cache stats for 10 seconds

# Security
security:
  max_connections_per_ip: 100
  ban_threshold: 30             # Ban at 30% invalid shares
  ban_duration: 3600            # Ban for 1 hour
```

## Deployment

### Single Server (Development)

```bash
# Start Redis
redis-server

# Start Master + Slave combined
./tos-pool --config config.yaml --mode combined
```

### Production (Distributed)

```bash
# Server 1: Master
./tos-pool --config master.yaml --mode master

# Server 2-N: Slaves (geographic distribution)
./tos-pool --config slave-asia.yaml --mode slave
./tos-pool --config slave-eu.yaml --mode slave
./tos-pool --config slave-us.yaml --mode slave
```

### Docker Compose

```yaml
version: '3.8'
services:
  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

  tos-node:
    image: tos-network/tos-node
    command: --rpc-addr 0.0.0.0:8545

  master:
    image: tos-network/tos-pool
    command: --mode master
    depends_on:
      - redis
      - tos-node

  slave:
    image: tos-network/tos-pool
    command: --mode slave
    ports:
      - "3333:3333"
      - "3334:3334"
    depends_on:
      - master

  web:
    image: tos-network/tos-pool-web
    ports:
      - "80:80"
    depends_on:
      - master

volumes:
  redis_data:
```

## Project Structure

```
tos-pool/
├── cmd/
│   └── tos-pool/
│       └── main.go              # Entry point
├── internal/
│   ├── config/
│   │   └── config.go            # Configuration
│   ├── master/
│   │   ├── master.go            # Master coordinator
│   │   ├── unlocker.go          # Block unlocker
│   │   ├── payout.go            # Payment processor
│   │   └── slave_handler.go     # Slave communication
│   ├── slave/
│   │   ├── slave.go             # Slave coordinator
│   │   ├── stratum.go           # Stratum server
│   │   ├── session.go           # Miner sessions
│   │   ├── validator.go         # Share validation
│   │   ├── vardiff.go           # Difficulty adjustment
│   │   └── master_client.go     # Master connection
│   ├── storage/
│   │   ├── redis.go             # Redis client
│   │   └── schema.go            # Data structures
│   ├── rpc/
│   │   └── tos_client.go        # TOS node RPC
│   ├── toshash/
│   │   └── toshash.go           # TOS Hash V3 verification
│   ├── api/
│   │   ├── server.go            # API server
│   │   ├── handlers.go          # HTTP handlers
│   │   └── middleware.go        # CORS, auth, etc.
│   ├── policy/
│   │   ├── banning.go           # Ban policy
│   │   └── rate_limit.go        # Rate limiting
│   └── util/
│       ├── difficulty.go        # Difficulty calculations
│       ├── hex.go               # Hex encoding
│       └── log.go               # Logging
├── web/                         # Vue.js frontend
│   ├── src/
│   │   ├── views/
│   │   │   ├── Home.vue
│   │   │   ├── Stats.vue
│   │   │   ├── Miners.vue
│   │   │   └── Blocks.vue
│   │   └── components/
│   └── package.json
├── scripts/
│   ├── build.sh
│   └── deploy.sh
├── docker/
│   ├── Dockerfile
│   └── docker-compose.yaml
├── config/
│   ├── config.example.yaml
│   └── config.dev.yaml
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Implementation Priority

### Phase 1: Core Infrastructure
1. Configuration system
2. Redis storage layer
3. TOS node RPC client
4. TOS Hash V3 validator

### Phase 2: Mining Server
1. Stratum protocol server
2. Session management
3. Job distribution
4. Share validation
5. Variable difficulty

### Phase 3: Block Processing
1. Block template fetching
2. Block submission
3. Block unlocking
4. Orphan detection

### Phase 4: Payment System
1. PPLNS reward calculation
2. Balance management
3. Payment processing
4. Transaction confirmation

### Phase 5: API & Frontend
1. REST API server
2. Pool statistics
3. Miner dashboard
4. Vue.js frontend

### Phase 6: Production Features
1. Master-slave communication
2. TLS encryption
3. Rate limiting
4. Ban policy
5. Monitoring & alerts

## Security Considerations

1. **Share Validation**: Always verify TOS Hash V3 on pool side
2. **DDoS Protection**: Rate limiting, connection limits per IP
3. **TLS**: Encrypt stratum connections (stratum+ssl://)
4. **Master-Slave Encryption**: ChaCha20-Poly1305 or TLS
5. **Access Control**: Whitelist/blacklist addresses
6. **Payment Security**: Multi-sig or cold wallet for pool funds
