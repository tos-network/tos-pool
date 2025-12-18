# TOS Pool Configuration Example

This document provides a complete configuration example for TOS Pool with all available options.

## Complete Configuration File

Create a `config.yaml` file with the following content:

```yaml
# =============================================================================
# TOS Pool Configuration
# =============================================================================

# -----------------------------------------------------------------------------
# Pool Identity
# -----------------------------------------------------------------------------
pool:
  name: "TOS Mining Pool"
  fee: 1.0                              # Pool fee percentage (1.0 = 1%)
  fee_address: "tos1..."                # Address to receive pool fees

# -----------------------------------------------------------------------------
# TOS Node Connection
# -----------------------------------------------------------------------------
node:
  # Primary node URL (for backward compatibility)
  url: "http://127.0.0.1:8545"
  timeout: 10s

  # Multiple upstream nodes for high availability
  upstreams:
    - name: "primary"
      url: "http://127.0.0.1:8545"
      timeout: 10s
      weight: 10                        # Higher weight = preferred

    - name: "backup-1"
      url: "http://backup1.example.com:8545"
      timeout: 15s
      weight: 5

    - name: "backup-2"
      url: "http://backup2.example.com:8545"
      timeout: 20s
      weight: 1

  # Health check settings
  health_check_interval: 5s             # How often to check node health
  health_check_timeout: 3s              # Timeout for health check requests
  max_failures: 3                       # Failures before marking unhealthy
  recovery_threshold: 2                 # Successes before marking healthy

# -----------------------------------------------------------------------------
# Redis Database
# -----------------------------------------------------------------------------
redis:
  url: "127.0.0.1:6379"
  password: ""                          # Leave empty if no password
  db: 0

# -----------------------------------------------------------------------------
# Master Server (coordinates mining)
# -----------------------------------------------------------------------------
master:
  enabled: true
  bind: "0.0.0.0:3221"
  secret: "your-secret-key-here"        # Required for master-slave communication

# -----------------------------------------------------------------------------
# Slave Server (handles miner connections)
# -----------------------------------------------------------------------------
slave:
  enabled: true
  master_url: ""                        # Set if running slave separately

  # Stratum Protocol
  stratum_bind: "0.0.0.0:3333"          # Standard Stratum port
  stratum_tls_bind: "0.0.0.0:3334"      # TLS Stratum port
  tls_cert: "/etc/ssl/pool/cert.pem"    # TLS certificate
  tls_key: "/etc/ssl/pool/key.pem"      # TLS private key

  # HTTP GetWork (legacy)
  getwork_enabled: false
  getwork_bind: "0.0.0.0:8888"

  # WebSocket GetWork (real-time)
  websocket_enabled: true
  websocket_bind: "0.0.0.0:3335"

  # Xatum Protocol (TLS JSON)
  xatum_enabled: true
  xatum_bind: "0.0.0.0:3336"

# -----------------------------------------------------------------------------
# Mining Difficulty Settings
# -----------------------------------------------------------------------------
mining:
  initial_difficulty: 1000000           # Starting difficulty for new miners
  min_difficulty: 1000                  # Minimum allowed difficulty
  max_difficulty: 1000000000000         # Maximum allowed difficulty
  vardiff_target_time: 4.0              # Target seconds between shares
  vardiff_retarget: 90.0                # Seconds between difficulty adjustments
  vardiff_variance: 30.0                # Allowed variance percentage
  job_refresh_interval: 500ms           # How often to check for new blocks

# -----------------------------------------------------------------------------
# Share Validation
# -----------------------------------------------------------------------------
validation:
  trust_threshold: 50                   # Shares before miner is trusted
  trust_check_percent: 75               # Percent of shares to validate for trusted
  hashrate_window: 10m                  # Window for current hashrate
  hashrate_large_window: 3h             # Window for average hashrate

# -----------------------------------------------------------------------------
# Block Unlocking
# -----------------------------------------------------------------------------
unlocker:
  enabled: true
  interval: 60s                         # How often to check for matured blocks
  immature_depth: 10                    # Blocks before reward is pending
  mature_depth: 100                     # Blocks before reward is spendable

# -----------------------------------------------------------------------------
# PPLNS Payment Scheme
# -----------------------------------------------------------------------------
pplns:
  window: 2.0                           # N multiplier for PPLNS window
  dynamic_window: false                 # Enable dynamic window sizing
  min_window: 0.5                       # Minimum N multiplier
  max_window: 4.0                       # Maximum N multiplier

# -----------------------------------------------------------------------------
# Payout Processing
# -----------------------------------------------------------------------------
payouts:
  enabled: true
  interval: 1h                          # How often to process payouts
  threshold: 100000000                  # Minimum balance for payout (0.1 TOS)
  max_addresses_per_tx: 100             # Batch size for payouts
  gas_limit: 21000
  gas_price: "auto"                     # "auto" or specific value in wei
  withdrawal_fee: 0                     # Fixed fee per withdrawal
  withdrawal_fee_rate: 0.0              # Percentage fee (0.01 = 1%)

# -----------------------------------------------------------------------------
# REST API Server
# -----------------------------------------------------------------------------
api:
  enabled: true
  bind: "0.0.0.0:8080"
  stats_cache: 10s                      # Cache duration for stats endpoint
  cors_origins:
    - "*"                               # Allowed CORS origins
  admin_enabled: true                   # Enable admin API endpoints
  admin_password: "your-admin-password" # Admin API password

# -----------------------------------------------------------------------------
# Security Settings
# -----------------------------------------------------------------------------
security:
  max_connections_per_ip: 100           # Max concurrent connections per IP
  max_workers_per_address: 256          # Max workers per miner address
  ban_threshold: 30                     # Invalid shares before ban
  ban_duration: 1h                      # Ban duration
  rate_limit_shares: 100                # Max shares per second per connection

# -----------------------------------------------------------------------------
# Notifications
# -----------------------------------------------------------------------------
notify:
  enabled: true
  pool_url: "https://pool.example.com"  # Pool URL for notification links

  # Discord webhook
  discord_url: "https://discord.com/api/webhooks/..."

  # Telegram bot
  telegram_bot: "your_bot_token"
  telegram_chat: "your_chat_id"

# -----------------------------------------------------------------------------
# Logging
# -----------------------------------------------------------------------------
log:
  level: "info"                         # debug, info, warn, error
  format: "console"                     # console or json
  file: ""                              # Log file path (empty = stdout)

# -----------------------------------------------------------------------------
# pprof Profiling (for debugging)
# -----------------------------------------------------------------------------
profiling:
  enabled: false                        # Enable in development only
  bind: "127.0.0.1:6060"                # Only bind to localhost!

# -----------------------------------------------------------------------------
# New Relic APM (optional monitoring)
# -----------------------------------------------------------------------------
newrelic:
  enabled: false
  app_name: "TOS Pool"
  license_key: "your_newrelic_license_key"
```

## Minimal Configuration

For a quick start, use this minimal configuration:

```yaml
pool:
  name: "My TOS Pool"
  fee: 1.0
  fee_address: "tos1your_address_here"

node:
  url: "http://127.0.0.1:8545"

redis:
  url: "127.0.0.1:6379"

master:
  enabled: true
  bind: "0.0.0.0:3221"
  secret: "change-this-secret"

slave:
  enabled: true
  stratum_bind: "0.0.0.0:3333"

api:
  enabled: true
  bind: "0.0.0.0:8080"
```

## Production Configuration

For production deployments, consider these settings:

```yaml
pool:
  name: "TOS Mining Pool"
  fee: 1.0
  fee_address: "tos1production_fee_address"

node:
  upstreams:
    - name: "local"
      url: "http://127.0.0.1:8545"
      weight: 100
    - name: "backup"
      url: "http://backup.internal:8545"
      weight: 10
  health_check_interval: 5s
  max_failures: 3

redis:
  url: "127.0.0.1:6379"
  password: "your_redis_password"

master:
  enabled: true
  bind: "0.0.0.0:3221"
  secret: "strong-random-secret-key"

slave:
  enabled: true
  stratum_bind: "0.0.0.0:3333"
  stratum_tls_bind: "0.0.0.0:3334"
  tls_cert: "/etc/ssl/pool/fullchain.pem"
  tls_key: "/etc/ssl/pool/privkey.pem"
  websocket_enabled: true
  websocket_bind: "0.0.0.0:3335"

mining:
  initial_difficulty: 5000000
  vardiff_target_time: 4.0

validation:
  trust_threshold: 100

payouts:
  enabled: true
  interval: 1h
  threshold: 500000000                  # 0.5 TOS minimum

api:
  enabled: true
  bind: "0.0.0.0:8080"
  admin_enabled: true
  admin_password: "strong-admin-password"

security:
  max_connections_per_ip: 50
  ban_threshold: 20
  ban_duration: 2h

notify:
  enabled: true
  pool_url: "https://pool.example.com"
  discord_url: "https://discord.com/api/webhooks/..."

log:
  level: "info"
  format: "json"
  file: "/var/log/tos-pool/pool.log"
```

## Environment Variables

All configuration options can be overridden with environment variables using the prefix `TOS_POOL_`:

```bash
export TOS_POOL_POOL_FEE=1.5
export TOS_POOL_NODE_URL="http://node.example.com:8545"
export TOS_POOL_REDIS_PASSWORD="secret"
export TOS_POOL_MASTER_SECRET="env-secret"
export TOS_POOL_API_ADMIN_PASSWORD="admin123"
```

## Port Reference

| Port | Protocol | Description |
|------|----------|-------------|
| 3221 | TCP | Master coordination |
| 3333 | Stratum | Standard mining |
| 3334 | Stratum+TLS | Encrypted mining |
| 3335 | WebSocket | WebSocket GetWork |
| 3336 | Xatum | TLS JSON mining |
| 8080 | HTTP | REST API |
| 8888 | HTTP | HTTP GetWork (legacy) |
| 6060 | HTTP | pprof (debug only) |

## Running the Pool

```bash
# Combined mode (default)
./tos-pool -config config.yaml

# Master only mode
./tos-pool -config config.yaml -mode master

# Slave only mode
./tos-pool -config config.yaml -mode slave
```
