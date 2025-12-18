# TOS Pool Deployment Guide

This guide covers deploying tos-pool to **Testnet** and **Mainnet** environments.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Building from Source](#building-from-source)
- [Testnet Deployment](#testnet-deployment)
- [Mainnet Deployment](#mainnet-deployment)
- [TLS Configuration](#tls-configuration)
- [Systemd Service Setup](#systemd-service-setup)
- [Nginx Reverse Proxy](#nginx-reverse-proxy)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

### System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 2 cores | 4+ cores |
| RAM | 2 GB | 8 GB |
| Disk | 20 GB SSD | 100 GB SSD |
| Network | 100 Mbps | 1 Gbps |

### Software Requirements

- **Go** 1.22 or higher
- **Redis** 6.0 or higher
- **TOS Node** (tos-daemon) running with RPC enabled
- **Git** for cloning the repository

### Install Dependencies (Ubuntu/Debian)

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Go
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Install Redis
sudo apt install redis-server -y
sudo systemctl enable redis-server
sudo systemctl start redis-server

# Verify installations
go version
redis-cli ping
```

---

## Building from Source

```bash
# Clone repository
git clone https://github.com/tos-network/tos-pool.git
cd tos-pool

# Build binary
make build

# Or build for Linux (if cross-compiling from macOS)
make build-linux

# Verify build
./bin/tos-pool --version
```

---

## Testnet Deployment

### 1. Start TOS Testnet Node

```bash
# Run tos-daemon with testnet configuration
./tos-daemon --network testnet --rpc-bind-address 127.0.0.1:8080
```

### 2. Create Testnet Configuration

```bash
cp config/config.example.yaml config/config-testnet.yaml
```

Edit `config/config-testnet.yaml`:

```yaml
# TOS Pool Testnet Configuration

pool:
  name: "TOS Testnet Pool"
  fee: 1.0
  fee_address: "tst1your_testnet_fee_address_here"  # Testnet address (tst1...)

node:
  url: "http://127.0.0.1:8080"  # Testnet node RPC
  timeout: 10s

redis:
  url: "127.0.0.1:6379"
  password: ""
  db: 1  # Use different DB for testnet

master:
  enabled: true
  bind: "0.0.0.0:3221"
  secret: "your_random_secret_here"

slave:
  enabled: true
  master_url: "127.0.0.1:3221"
  stratum_bind: "0.0.0.0:3333"
  stratum_tls_bind: "0.0.0.0:3334"

mining:
  initial_difficulty: 300       # Lower for testnet
  min_difficulty: 100
  max_difficulty: 1000000
  vardiff_target_time: 4
  vardiff_retarget: 90
  vardiff_variance: 30
  job_refresh_interval: 500ms

validation:
  trust_threshold: 50
  trust_check_percent: 75
  hashrate_window: 10m
  hashrate_large_window: 3h

unlocker:
  enabled: true
  interval: 60s
  immature_depth: 10
  mature_depth: 100

pplns:
  window: 2.0

payouts:
  enabled: true
  interval: 1h
  threshold: 10000000           # 0.01 TOS for testnet
  max_addresses_per_tx: 100
  gas_limit: 21000
  gas_price: "auto"

api:
  enabled: true
  bind: "0.0.0.0:8088"
  stats_cache: 10s
  cors_origins:
    - "*"

security:
  max_connections_per_ip: 100
  max_workers_per_address: 256
  ban_threshold: 30
  ban_duration: 1h
  rate_limit_shares: 100

log:
  level: "debug"                # More verbose for testnet
  format: "console"
  file: "/var/log/tos-pool/testnet.log"
```

### 3. Start Pool

```bash
# Create log directory
sudo mkdir -p /var/log/tos-pool
sudo chown $USER:$USER /var/log/tos-pool

# Run pool
./bin/tos-pool --config config/config-testnet.yaml
```

### 4. Connect Miner (Testing)

```bash
# Using tosminer
./tosminer -C -t 2 -P stratum+tcp://127.0.0.1:3333 \
    -u tst1your_testnet_address.worker1
```

### 5. Verify Operation

```bash
# Check pool stats
curl http://localhost:8088/api/stats

# Check miner connected
curl http://localhost:8088/api/miners/tst1your_testnet_address
```

---

## Mainnet Deployment

### 1. Start TOS Mainnet Node

```bash
# Run tos-daemon with mainnet configuration
./tos-daemon --network mainnet --rpc-bind-address 127.0.0.1:8080
```

### 2. Create Mainnet Configuration

```bash
cp config/config.example.yaml config/config-mainnet.yaml
```

Edit `config/config-mainnet.yaml`:

```yaml
# TOS Pool Mainnet Configuration

pool:
  name: "TOS Mining Pool"
  fee: 1.0
  fee_address: "tos1your_mainnet_fee_address_here"  # Mainnet address (tos1...)

node:
  url: "http://127.0.0.1:8080"  # Mainnet node RPC
  timeout: 10s

redis:
  url: "127.0.0.1:6379"
  password: "your_redis_password"  # Use password in production
  db: 0

master:
  enabled: true
  bind: "0.0.0.0:3221"
  secret: "generate_a_secure_random_string_here"

slave:
  enabled: true
  master_url: "127.0.0.1:3221"
  stratum_bind: "0.0.0.0:3333"
  stratum_tls_bind: "0.0.0.0:3334"
  tls_cert: "/etc/tos-pool/ssl/cert.pem"
  tls_key: "/etc/tos-pool/ssl/key.pem"

mining:
  initial_difficulty: 1000000   # Higher for mainnet
  min_difficulty: 10000
  max_difficulty: 1000000000000
  vardiff_target_time: 4
  vardiff_retarget: 90
  vardiff_variance: 30
  job_refresh_interval: 500ms

validation:
  trust_threshold: 50
  trust_check_percent: 75
  hashrate_window: 10m
  hashrate_large_window: 3h

unlocker:
  enabled: true
  interval: 60s
  immature_depth: 10
  mature_depth: 100

pplns:
  window: 2.0

payouts:
  enabled: true
  interval: 1h
  threshold: 100000000          # 0.1 TOS minimum
  max_addresses_per_tx: 100
  gas_limit: 21000
  gas_price: "auto"

api:
  enabled: true
  bind: "127.0.0.1:8088"        # Bind to localhost, use nginx for public
  stats_cache: 10s
  cors_origins:
    - "https://pool.example.com"

security:
  max_connections_per_ip: 50    # Stricter for mainnet
  max_workers_per_address: 128
  ban_threshold: 20
  ban_duration: 2h
  rate_limit_shares: 50

log:
  level: "info"
  format: "json"                # JSON format for log aggregation
  file: "/var/log/tos-pool/mainnet.log"
```

### 3. Security Hardening

```bash
# Create dedicated user
sudo useradd -r -s /bin/false tos-pool

# Create directories
sudo mkdir -p /etc/tos-pool/ssl
sudo mkdir -p /var/log/tos-pool
sudo mkdir -p /opt/tos-pool

# Copy files
sudo cp bin/tos-pool /opt/tos-pool/
sudo cp config/config-mainnet.yaml /etc/tos-pool/config.yaml

# Set permissions
sudo chown -R tos-pool:tos-pool /opt/tos-pool
sudo chown -R tos-pool:tos-pool /etc/tos-pool
sudo chown -R tos-pool:tos-pool /var/log/tos-pool
sudo chmod 600 /etc/tos-pool/config.yaml
```

### 4. Configure Redis Security

Edit `/etc/redis/redis.conf`:

```conf
# Bind to localhost only
bind 127.0.0.1

# Require password
requirepass your_redis_password

# Disable dangerous commands
rename-command FLUSHDB ""
rename-command FLUSHALL ""
rename-command DEBUG ""
```

```bash
sudo systemctl restart redis-server
```

### 5. Firewall Configuration

```bash
# Allow stratum ports
sudo ufw allow 3333/tcp    # Stratum TCP
sudo ufw allow 3334/tcp    # Stratum TLS
sudo ufw allow 80/tcp      # HTTP (nginx)
sudo ufw allow 443/tcp     # HTTPS (nginx)

# Enable firewall
sudo ufw enable
```

---

## TLS Configuration

### Using Let's Encrypt

```bash
# Install certbot
sudo apt install certbot -y

# Generate certificate
sudo certbot certonly --standalone -d pool.example.com

# Copy certificates
sudo cp /etc/letsencrypt/live/pool.example.com/fullchain.pem /etc/tos-pool/ssl/cert.pem
sudo cp /etc/letsencrypt/live/pool.example.com/privkey.pem /etc/tos-pool/ssl/key.pem
sudo chown tos-pool:tos-pool /etc/tos-pool/ssl/*.pem
sudo chmod 600 /etc/tos-pool/ssl/*.pem
```

### Auto-Renewal

```bash
# Add to crontab
sudo crontab -e

# Add line:
0 0 1 * * certbot renew --quiet && cp /etc/letsencrypt/live/pool.example.com/*.pem /etc/tos-pool/ssl/ && systemctl restart tos-pool
```

---

## Systemd Service Setup

Create `/etc/systemd/system/tos-pool.service`:

```ini
[Unit]
Description=TOS Mining Pool
After=network.target redis-server.service
Requires=redis-server.service

[Service]
Type=simple
User=tos-pool
Group=tos-pool
WorkingDirectory=/opt/tos-pool
ExecStart=/opt/tos-pool/tos-pool --config /etc/tos-pool/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65535

# Security hardening
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/log/tos-pool

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable tos-pool
sudo systemctl start tos-pool

# Check status
sudo systemctl status tos-pool

# View logs
sudo journalctl -u tos-pool -f
```

---

## Nginx Reverse Proxy

Install and configure nginx for the API:

```bash
sudo apt install nginx -y
```

Create `/etc/nginx/sites-available/tos-pool`:

```nginx
upstream tos_pool_api {
    server 127.0.0.1:8088;
    keepalive 64;
}

server {
    listen 80;
    server_name pool.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name pool.example.com;

    ssl_certificate /etc/letsencrypt/live/pool.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/pool.example.com/privkey.pem;

    # SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
    ssl_prefer_server_ciphers off;

    # API proxy
    location /api/ {
        proxy_pass http://tos_pool_api;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # CORS headers
        add_header Access-Control-Allow-Origin *;
        add_header Access-Control-Allow-Methods "GET, OPTIONS";
    }

    # Health check
    location /health {
        proxy_pass http://tos_pool_api;
    }

    # Static frontend (optional)
    location / {
        root /var/www/tos-pool;
        try_files $uri $uri/ /index.html;
    }
}
```

```bash
# Enable site
sudo ln -s /etc/nginx/sites-available/tos-pool /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

---

## Monitoring

### Health Check Script

Create `/opt/tos-pool/health-check.sh`:

```bash
#!/bin/bash

POOL_API="http://127.0.0.1:8088"
ALERT_EMAIL="admin@example.com"

# Check pool API
if ! curl -sf "$POOL_API/health" > /dev/null; then
    echo "Pool API is down!" | mail -s "TOS Pool Alert" $ALERT_EMAIL
    exit 1
fi

# Check Redis
if ! redis-cli ping > /dev/null 2>&1; then
    echo "Redis is down!" | mail -s "TOS Pool Alert" $ALERT_EMAIL
    exit 1
fi

# Check block height is progressing
CURRENT_HEIGHT=$(curl -sf "$POOL_API/api/stats" | jq -r '.network.height')
LAST_HEIGHT_FILE="/tmp/tos_pool_last_height"

if [ -f "$LAST_HEIGHT_FILE" ]; then
    LAST_HEIGHT=$(cat "$LAST_HEIGHT_FILE")
    if [ "$CURRENT_HEIGHT" -le "$LAST_HEIGHT" ]; then
        echo "Block height not progressing! Current: $CURRENT_HEIGHT" | mail -s "TOS Pool Alert" $ALERT_EMAIL
    fi
fi

echo "$CURRENT_HEIGHT" > "$LAST_HEIGHT_FILE"
```

Add to crontab:

```bash
*/5 * * * * /opt/tos-pool/health-check.sh
```

### Log Rotation

Create `/etc/logrotate.d/tos-pool`:

```
/var/log/tos-pool/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0640 tos-pool tos-pool
    postrotate
        systemctl reload tos-pool > /dev/null 2>&1 || true
    endscript
}
```

---

## Troubleshooting

### Common Issues

#### 1. "Connection refused" to TOS Node

```bash
# Check if node is running
curl http://127.0.0.1:8080/json_rpc -d '{"jsonrpc":"2.0","method":"get_info","id":1}'

# Verify node RPC is enabled
./tos-daemon --help | grep rpc
```

#### 2. "Low difficulty share" Errors

- Ensure pool and miner use the same TOS Hash V3 algorithm
- Check nonce byte order (must be big-endian)
- Verify MinerWork format (112 bytes)

#### 3. Redis Connection Failed

```bash
# Check Redis status
sudo systemctl status redis-server

# Test connection
redis-cli -a your_password ping
```

#### 4. Port Already in Use

```bash
# Find process using port
sudo lsof -i :3333

# Kill process
sudo kill -9 <PID>
```

#### 5. Block Submission Failed

- Check hex encoding (no `0x` prefix)
- Verify node is synced
- Check node has peers

### Debug Mode

Run pool with debug logging:

```bash
./bin/tos-pool --config config/config.yaml --log-level debug
```

### View Real-time Stats

```bash
# Pool stats
watch -n 1 'curl -s http://localhost:8088/api/stats | jq'

# Connected miners
watch -n 1 'curl -s http://localhost:8088/api/miners | jq'
```

---

## Configuration Differences Summary

| Setting | Testnet | Mainnet |
|---------|---------|---------|
| Address prefix | `tst1...` | `tos1...` |
| Initial difficulty | 300 | 1,000,000 |
| Min difficulty | 100 | 10,000 |
| Payment threshold | 0.01 TOS | 0.1 TOS |
| Redis DB | 1 | 0 |
| Log level | debug | info |
| API bind | 0.0.0.0 | 127.0.0.1 |
| TLS | Optional | Required |
| Redis password | Optional | Required |

---

## Quick Reference

### Start/Stop Commands

```bash
# Systemd
sudo systemctl start tos-pool
sudo systemctl stop tos-pool
sudo systemctl restart tos-pool

# Manual
./bin/tos-pool --config config/config.yaml
```

### Useful Endpoints

```bash
# Pool stats
curl http://localhost:8088/api/stats

# Network info
curl http://localhost:8088/api/stats | jq '.network'

# Miner stats
curl http://localhost:8088/api/miners/<address>

# Recent blocks
curl http://localhost:8088/api/blocks

# Health check
curl http://localhost:8088/health
```

### Miner Connection

```bash
# TCP (testnet)
./tosminer -P stratum+tcp://pool.example.com:3333 -u tst1address.worker

# TLS (mainnet)
./tosminer -P stratum+ssl://pool.example.com:3334 -u tos1address.worker
```
