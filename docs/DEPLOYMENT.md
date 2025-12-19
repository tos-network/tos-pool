# TOS Pool Deployment Guide

This guide covers the deployment and configuration of TOS Mining Pool.

## Prerequisites

- Go 1.21 or later
- Redis 6.0 or later
- TOS Daemon (running and synced)
- TOS Wallet (for payouts)

## Architecture Overview

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Miners    │────▶│  tos-pool   │────▶│ TOS Daemon  │
│  (Stratum)  │     │   (Master)  │     │   (RPC)     │
└─────────────┘     └──────┬──────┘     └─────────────┘
                          │
                          ▼
                   ┌─────────────┐     ┌─────────────┐
                   │    Redis    │     │ TOS Wallet  │
                   │  (Storage)  │     │   (RPC)     │
                   └─────────────┘     └─────────────┘
```

## Quick Start

### 1. Build the Pool

```bash
cd tos-pool
go build -o tos-pool ./cmd/pool
```

### 2. Configure

Copy the example configuration:

```bash
cp config/config.example.yaml config/config.yaml
```

Edit `config/config.yaml` with your settings.

### 3. Start Services

```bash
# Start Redis
redis-server

# Start TOS Daemon
./tos_daemon --network testnet

# Start TOS Wallet (for payouts)
./tos_wallet --network testnet \
  --wallet-path pool_wallet \
  --password YOUR_PASSWORD \
  --rpc-bind-address 127.0.0.1:8081 \
  --daemon-address http://127.0.0.1:8080 \
  --interactive

# In wallet prompt, start RPC server
> start_rpc_server

# Start Pool
./tos-pool --config config/config.yaml
```

## Configuration Reference

### Pool Identity

```yaml
pool:
  name: "TOS Mining Pool"        # Pool name displayed to miners
  fee: 1.0                       # Pool fee percentage (1.0 = 1%)
  fee_address: "tos1..."         # Address to receive pool fees
```

### TOS Node Connection

```yaml
node:
  url: "http://127.0.0.1:8080"   # TOS daemon RPC endpoint
  timeout: 10s                   # RPC request timeout
```

For high availability, configure multiple upstreams:

```yaml
node:
  upstreams:
    - name: "primary"
      url: "http://node1:8080"
      weight: 10
    - name: "backup"
      url: "http://node2:8080"
      weight: 5
  health_check_interval: 5s
  max_failures: 3
```

### Redis Database

```yaml
redis:
  url: "127.0.0.1:6379"
  password: ""                   # Redis password (if required)
  db: 0                          # Redis database number
```

### Mining Settings

```yaml
mining:
  initial_difficulty: 1000000    # Starting difficulty for new miners
  min_difficulty: 1000           # Minimum allowed difficulty
  max_difficulty: 1000000000000  # Maximum allowed difficulty
  vardiff_target_time: 4         # Target seconds per share
  vardiff_retarget: 90           # Seconds between difficulty adjustments
  vardiff_variance: 30           # Allowed variance percentage
  job_refresh_interval: 500ms    # How often to refresh mining jobs
```

### Block Processing

```yaml
unlocker:
  enabled: true
  interval: 60s                  # Check interval for new blocks
  immature_depth: 10             # Confirmations before marking immature
  mature_depth: 100              # Confirmations before marking mature
```

### Payout Configuration

```yaml
payouts:
  enabled: true
  interval: 1h                   # Payout processing interval
  threshold: 100000000           # Minimum balance for payout (0.1 TOS)
  max_addresses_per_tx: 25       # Max recipients per batch transaction
```

#### Wallet RPC Settings

The pool requires a running TOS wallet with RPC server enabled to send payouts.

```yaml
payouts:
  # Wallet RPC endpoint (host:port)
  wallet_rpc: "127.0.0.1:8081"

  # Optional: RPC authentication
  wallet_user: ""
  wallet_password: ""

  # Network (must match daemon network)
  wallet_network: "testnet"
```

### API Server

```yaml
api:
  enabled: true
  bind: "0.0.0.0:8088"           # API server bind address
  stats_cache: 10s               # Stats cache duration
  cors_origins:
    - "*"                        # Allowed CORS origins
```

### Security Settings

```yaml
security:
  max_connections_per_ip: 100    # Max stratum connections per IP
  max_workers_per_address: 256   # Max workers per mining address
  ban_threshold: 30              # Invalid share % to trigger ban
  ban_duration: 1h               # Ban duration
  rate_limit_shares: 100         # Max shares per second per IP
```

## Wallet Setup for Payouts

### Creating a Pool Wallet

```bash
# Create new wallet
./tos_wallet --network testnet \
  --wallet-path /path/to/pool_wallet \
  --password YOUR_SECURE_PASSWORD \
  --offline-mode \
  --exec "display_address"
```

Save the displayed address - this will be used for receiving mining rewards.

### Starting Wallet with RPC Server

The wallet must be running with RPC server enabled for the pool to send payouts:

```bash
./tos_wallet --network testnet \
  --wallet-path /path/to/pool_wallet \
  --password YOUR_SECURE_PASSWORD \
  --daemon-address http://127.0.0.1:8080 \
  --rpc-bind-address 127.0.0.1:8081 \
  --rpc-username admin \
  --rpc-password YOUR_RPC_PASSWORD \
  --interactive
```

Then in the wallet prompt:

```
> start_rpc_server
```

The RPC server will start listening on the specified address.

### Wallet RPC Security

For production deployments:

1. **Bind to localhost only**: Use `127.0.0.1:8081` instead of `0.0.0.0:8081`
2. **Enable authentication**: Set `--rpc-username` and `--rpc-password`
3. **Use TLS**: Configure TLS certificates for encrypted connections
4. **Firewall**: Block external access to wallet RPC port

### Funding the Pool Wallet

The pool wallet needs sufficient balance to send payouts. Transfer TOS to the pool wallet address before enabling payouts.

## Running in Production

### Systemd Service

Create `/etc/systemd/system/tos-pool.service`:

```ini
[Unit]
Description=TOS Mining Pool
After=network.target redis.service

[Service]
Type=simple
User=pool
WorkingDirectory=/opt/tos-pool
ExecStart=/opt/tos-pool/tos-pool --config /opt/tos-pool/config/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Create `/etc/systemd/system/tos-wallet.service`:

```ini
[Unit]
Description=TOS Wallet RPC Service
After=network.target

[Service]
Type=simple
User=pool
WorkingDirectory=/opt/tos-wallet
ExecStart=/opt/tos-wallet/tos_wallet \
  --network mainnet \
  --wallet-path /opt/tos-wallet/pool_wallet \
  --password-file /opt/tos-wallet/.password \
  --daemon-address http://127.0.0.1:8080 \
  --rpc-bind-address 127.0.0.1:8081 \
  --interactive
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start services:

```bash
sudo systemctl daemon-reload
sudo systemctl enable tos-wallet tos-pool
sudo systemctl start tos-wallet tos-pool
```

### Monitoring

Enable the profiling endpoint for debugging:

```yaml
profiling:
  enabled: true
  bind: "127.0.0.1:6060"
```

Access pprof at `http://localhost:6060/debug/pprof/`

### Logging

```yaml
log:
  level: "info"      # debug, info, warn, error
  format: "console"  # console or json
  file: "/var/log/tos-pool/pool.log"
```

## Troubleshooting

### Wallet Connection Issues

**Error**: `wallet connectivity check failed`

- Verify wallet is running with RPC server started
- Check `wallet_rpc` address in config matches wallet's `--rpc-bind-address`
- Ensure network connectivity between pool and wallet

**Error**: `wallet is offline`

- Wallet is not connected to daemon
- Check daemon is running and synced
- Verify `--daemon-address` in wallet startup command

### Payout Failures

**Error**: `Batch payout failed`

- Check wallet has sufficient balance
- Verify recipient addresses are valid
- Check daemon is synced and accepting transactions

### Block Verification Issues

**Error**: `Block orphaned`

- Normal behavior when another miner finds block at same height
- Check pool hashrate vs network hashrate

## Network Reference

| Network  | Daemon Port | Address Prefix |
|----------|-------------|----------------|
| Mainnet  | 8080        | tos1           |
| Testnet  | 8080        | tst1           |
| Devnet   | 8080        | tst1           |

## Support

For issues and feature requests, please open an issue on the GitHub repository.
