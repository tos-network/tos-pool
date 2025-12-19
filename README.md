# TOS Mining Pool

A high-performance mining pool for TOS Hash V3 algorithm, designed to work with [tosminer](https://github.com/tos-network/tosminer).

## Features

- **Stratum Protocol**: Full Stratum V1 support with TLS encryption
- **Variable Difficulty**: Automatic difficulty adjustment per miner
- **PPLNS Payment**: Pay Per Last N Shares reward system
- **Master-Slave Architecture**: Distributed deployment for scalability
- **Redis Storage**: High-performance data persistence
- **REST API**: Real-time statistics and miner dashboard
- **Block Unlocking**: Automatic reward maturation
- **Payment Processing**: Automated payout system with safety features
- **Security Policy**: IP banning, rate limiting, invalid share tracking

## Documentation

| Document | Description |
|----------|-------------|
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | System architecture overview |
| [DESIGN.md](docs/DESIGN.md) | Detailed design documentation |
| [Testing Guide](docs/testing-guide.md) | Integration testing with tosminer |

## Architecture

```
┌─────────────────┐     ┌─────────────────┐
│   TOS Node      │     │     Redis       │
└────────┬────────┘     └────────┬────────┘
         │                       │
         └───────────┬───────────┘
                     │
         ┌───────────┴───────────┐
         │     Master Node       │
         │ - Block Template      │
         │ - Share Validation    │
         │ - Payment Processing  │
         │ - API Server          │
         └───────────┬───────────┘
                     │
    ┌────────────────┼────────────────┐
    │                │                │
┌───┴───┐       ┌────┴────┐      ┌────┴────┐
│ Slave │       │  Slave  │      │  Slave  │
│ (Asia)│       │ (Europe)│      │  (US)   │
└───┬───┘       └────┬────┘      └────┬────┘
    │                │                │
  Miners           Miners           Miners
```

## Quick Start

### Prerequisites

- Go 1.22+
- Redis 6+
- TOS Node (running with RPC enabled)

### Installation

```bash
# Clone repository
git clone https://github.com/tos-network/tos-pool.git
cd tos-pool

# Build
make build

# Copy and edit configuration
cp config/config.example.yaml config/config.yaml
nano config/config.yaml

# Run
./bin/tos-pool --config config/config.yaml
```

### Docker

```bash
# Build image
make docker-build

# Start with Docker Compose
make docker-up
```

### Testing with tosminer

See [Testing Guide](docs/testing-guide.md) for detailed instructions on local testing.

```bash
# Quick test setup
docker run -d --name tos-redis -p 6379:6379 redis:latest
python3 scripts/mock_tos_node.py 8545 &
./bin/tos-pool --config config/config.yaml &

# Connect tosminer
tosminer --pool stratum+tcp://127.0.0.1:3333 \
    --user tos1qr8d7mslv5q4zkvhp9vy9aqj0aqwts40xjsz4sqr8d7mslv5q4zkvhp9vy.worker1
```

## Configuration

See [config/config.example.yaml](config/config.example.yaml) for all options.

### Key Settings

```yaml
# Pool identity
pool:
  name: "TOS Mining Pool"
  fee: 1.0                    # 1% pool fee
  fee_address: "tos1..."      # Fee recipient

# TOS node
node:
  url: "http://127.0.0.1:8545"

# Redis
redis:
  url: "127.0.0.1:6379"

# Stratum server
slave:
  stratum_bind: "0.0.0.0:3333"
  stratum_tls_bind: "0.0.0.0:3334"

# Mining settings
mining:
  initial_difficulty: 1000000
  vardiff_target_time: 4      # 4 seconds per share
```

## Deployment Modes

### Combined Mode (Development)
```bash
./bin/tos-pool --mode combined
```
Runs master and slave on the same instance.

### Master Mode (Production)
```bash
./bin/tos-pool --mode master
```
Runs only the coordinator (block processing, payments, API).

### Slave Mode (Production)
```bash
./bin/tos-pool --mode slave
```
Runs only the stratum server (miner connections).

## Connecting Miners

```bash
# Using tosminer (TCP)
tosminer --pool stratum+tcp://pool.example.com:3333 \
    --user tos1your_wallet_address.worker_name

# Using tosminer (TLS)
tosminer --pool stratum+ssl://pool.example.com:3334 \
    --user tos1your_wallet_address.worker_name
```

**Note**: TOS addresses must be 62 characters in bech32 format (starting with `tos1`).

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/stats` | Pool and network statistics |
| `GET /api/blocks` | Recent blocks found |
| `GET /api/payments` | Recent payments |
| `GET /api/miners/:address` | Miner statistics |
| `GET /api/miners/:address/payments` | Miner payment history |
| `GET /health` | Health check |

### Example Response: /api/stats

```json
{
  "pool": {
    "hashrate": 125000000000,
    "miners": 1250,
    "workers": 3400,
    "blocks_found": 156,
    "fee": 1.0
  },
  "network": {
    "height": 500000,
    "difficulty": 12500000000000,
    "hashrate": 890000000000000
  }
}
```

## Payment System

### PPLNS (Pay Per Last N Shares)

The pool uses PPLNS with a window of 2x network difficulty:
- Shares are weighted by difficulty
- Rewards distributed proportionally
- Discourages pool hopping

### Payment Flow

1. **Block Found**: Stored as candidate
2. **10 Confirmations**: Marked immature, balance credited
3. **100 Confirmations**: Marked mature, balance confirmed
4. **Hourly**: Payments processed for balances above threshold

### Payment Safety Features

- **Payment Locking**: Redis-based lock prevents double payments
- **TX Confirmation**: Waits for transaction to be mined
- **Balance Rollback**: Automatic recovery on failed payments
- **Peer Count Check**: Requires minimum peers before payout

## Security

- **TLS Encryption**: For stratum connections
- **Rate Limiting**: Connection limits per IP with grace period
- **IP Banning**: Auto-ban with ipset kernel integration
- **Invalid Share Tracking**: Auto-ban high invalid ratio miners
- **Address Blacklist/Whitelist**: Block known bad actors
- **Socket Flood Detection**: Max request size enforcement
- **Malformed Request Tracking**: Ban repeated bad requests

## Development

```bash
# Run tests
make test

# Format code
make fmt

# Lint
make lint

# Build
make build
```

## Project Structure

```
tos-pool/
├── cmd/tos-pool/          # Entry point
├── internal/
│   ├── api/               # REST API server
│   ├── config/            # Configuration
│   ├── master/            # Pool coordinator
│   ├── policy/            # Security policies
│   ├── rpc/               # TOS node client
│   ├── slave/             # Stratum server
│   ├── storage/           # Redis storage
│   ├── toshash/           # TOS Hash V3
│   └── util/              # Utilities
├── config/                # Configuration files
├── docs/                  # Documentation
├── scripts/               # Testing scripts
├── docker/                # Docker files
└── web/                   # Frontend (Vue.js)
```

## Scripts

| Script | Description |
|--------|-------------|
| `scripts/mock_tos_node.py` | Mock TOS node for testing |
| `scripts/mock_stratum_server.py` | Mock stratum server for miner testing |
| `scripts/test_stratum.py` | Stratum protocol test client |

## License

This project is licensed under the [GPL-3.0 License](LICENSE).

## Credits

- Architecture inspired by [open-ethereum-pool](https://github.com/sammy007/open-ethereum-pool)
