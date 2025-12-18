# TOS Pool & Miner Integration Testing Guide

This guide explains how to test tos-pool and tosminer together in a local development environment.

## Prerequisites

- Go 1.21+
- Python 3.8+
- Docker (for Redis)
- CMake and C++ compiler (for tosminer)

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  tosminer   │────▶│  tos-pool   │────▶│  TOS Node   │
│  (Miner)    │     │  (Pool)     │     │  (Mock)     │
└─────────────┘     └─────────────┘     └─────────────┘
      │                   │                   │
      │ Stratum:3333      │ RPC:8545          │
      │                   │ API:8080          │
      │                   │                   │
      │                   ▼                   │
      │             ┌─────────────┐           │
      │             │   Redis     │           │
      │             │   :6379     │           │
      │             └─────────────┘           │
```

## Quick Start

### 1. Start Redis

```bash
# Using Docker
docker run -d --name tos-redis -p 6379:6379 redis:latest

# Verify Redis is running
docker exec tos-redis redis-cli ping
# Should return: PONG
```

### 2. Start Mock TOS Node

```bash
cd ~/tos-network/tos-pool/scripts
python3 mock_tos_node.py 8545
```

Expected output:
```
============================================================
Mock TOS Node for Testing
============================================================
Listening on: http://127.0.0.1:8545
Starting height: 1000000
Difficulty: 1000000

Supported methods:
  - tos_getWork, tos_getBlockTemplate
  - tos_blockNumber, tos_getBlockByNumber, tos_getBlockByHash
  - tos_submitWork, tos_submitBlock
  - net_peerCount, tos_syncing, tos_gasPrice
  - tos_getBalance, tos_sendTransaction, tos_getTransactionReceipt

Press Ctrl+C to stop
============================================================
```

### 3. Build and Start tos-pool

```bash
cd ~/tos-network/tos-pool

# Copy example config if not exists
cp config/config.example.yaml config/config.yaml

# Build
go build -o bin/tos-pool ./cmd/tos-pool/

# Run
./bin/tos-pool --config config/config.yaml
```

Expected output:
```
INFO  TOS Pool v1.0.0 starting in combined mode
INFO  Connected to Redis at 127.0.0.1:6379
INFO  Policy server started
INFO  Pool master started
INFO  API server listening on 0.0.0.0:8080
INFO  Stratum server listening on 0.0.0.0:3333
INFO  Pool started successfully. Press Ctrl+C to stop.
```

### 4. Build and Connect tosminer

```bash
cd ~/tos-network/tosminer

# Build if not already built
mkdir -p build && cd build
cmake ..
make -j$(nproc)

# Connect to pool
# Note: TOS addresses must be 62 characters (bech32 format)
./bin/tosminer \
    --pool stratum+tcp://127.0.0.1:3333 \
    --user tos1qr8d7mslv5q4zkvhp9vy9aqj0aqwts40xjsz4sqr8d7mslv5q4zkvhp9vy.worker1
```

Expected output:
```
[I] Starting TOS Miner...
[I] Connecting to 127.0.0.1:3333 (TCP)...
[I] Connected to 127.0.0.1:3333
[I] Subscribed (session=1, extranonce1=00000001, extranonce2_size=4)
[I] Difficulty set to 1000000.000000
[I] Authorized with pool as tos1qr8d7...9vy.worker1
[I] New job: abc123... (height=1000001)
```

## Verification

### Check Pool API

```bash
# Health check
curl http://127.0.0.1:8080/health
# {"status":"ok"}

# Pool stats
curl http://127.0.0.1:8080/api/stats

# Recent blocks
curl http://127.0.0.1:8080/api/blocks
```

### Check Stratum Port

```bash
nc -z 127.0.0.1 3333 && echo "Stratum OK" || echo "Stratum FAIL"
```

### Test Stratum Protocol Manually

```bash
cd ~/tos-network/tos-pool/scripts
python3 test_stratum.py
```

## Test Scenarios

### 1. Basic Connection Test

Test that tosminer can connect, subscribe, authorize, and receive jobs.

```bash
timeout 10 ./bin/tosminer \
    --pool stratum+tcp://127.0.0.1:3333 \
    --user tos1qr8d7mslv5q4zkvhp9vy9aqj0aqwts40xjsz4sqr8d7mslv5q4zkvhp9vy.test
```

**Expected**: Connect, subscribe, authorize, receive jobs.

### 2. Invalid Address Test

Test address validation in the pool.

```bash
./bin/tosminer \
    --pool stratum+tcp://127.0.0.1:3333 \
    --user invalid_address.worker
```

**Expected**: "Authorization failed: Invalid TOS address"

### 3. Multiple Workers Test

Test multiple workers connecting simultaneously.

```bash
# Terminal 1
./bin/tosminer --pool stratum+tcp://127.0.0.1:3333 \
    --user tos1qr8d7mslv5q4zkvhp9vy9aqj0aqwts40xjsz4sqr8d7mslv5q4zkvhp9vy.gpu0

# Terminal 2
./bin/tosminer --pool stratum+tcp://127.0.0.1:3333 \
    --user tos1qr8d7mslv5q4zkvhp9vy9aqj0aqwts40xjsz4sqr8d7mslv5q4zkvhp9vy.gpu1
```

### 4. TLS Connection Test

```bash
# Generate test certificates first
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes

# Update config.yaml with TLS paths
# Then connect with TLS
./bin/tosminer \
    --pool stratum+ssl://127.0.0.1:3334 \
    --user tos1qr8d7mslv5q4zkvhp9vy9aqj0aqwts40xjsz4sqr8d7mslv5q4zkvhp9vy.worker
```

### 5. Benchmark Mode

Test tosminer performance without pool connection.

```bash
./bin/tosminer --benchmark
```

## Configuration

### Pool Configuration (config/config.yaml)

Key settings for testing:

```yaml
# Use test address for fee collection
pool:
  fee: 1.0
  fee_address: "tos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"

# Node connection (mock node)
node:
  url: "http://127.0.0.1:8545"
  timeout: 10s

# Redis
redis:
  url: "127.0.0.1:6379"
  password: ""
  db: 0

# Stratum server
slave:
  stratum_bind: "0.0.0.0:3333"

# Mining difficulty
mining:
  initial_difficulty: 1000000
  min_difficulty: 1000
  max_difficulty: 1000000000000
```

### Miner Options

```bash
./bin/tosminer --help

Options:
  --pool URL           Pool URL (stratum+tcp:// or stratum+ssl://)
  --user WALLET.WORKER Wallet address and worker name
  --pass PASSWORD      Pool password (optional)
  --benchmark          Run benchmark mode
  --list-devices       List available mining devices
  --opencl             Use OpenCL backend
  --cuda               Use CUDA backend
  --devices IDS        Comma-separated device IDs
```

## Troubleshooting

### "Connection refused" on port 3333

- Ensure tos-pool is running
- Check if port 3333 is not blocked by firewall

### "Invalid TOS address"

- TOS addresses must be exactly 62 characters
- Must start with "tos1"
- Must use valid bech32 characters

### "No mining devices available"

- This is expected if no GPU is present
- Use `--benchmark` to test CPU hashing

### Redis connection failed

- Ensure Redis is running: `docker ps | grep redis`
- Check Redis connection: `docker exec tos-redis redis-cli ping`

### Mock node errors

- Ensure mock_tos_node.py is running on port 8545
- Check for Python errors in the mock node terminal

## Cleanup

```bash
# Stop all processes
pkill -f tos-pool
pkill -f tosminer
pkill -f mock_tos_node

# Stop and remove Redis container
docker stop tos-redis
docker rm tos-redis
```

## Automated Test Script

Create a test script for CI/CD:

```bash
#!/bin/bash
# test_integration.sh

set -e

echo "Starting integration test..."

# Start Redis
docker run -d --name test-redis -p 6379:6379 redis:latest
sleep 2

# Start mock node
python3 scripts/mock_tos_node.py 8545 &
NODE_PID=$!
sleep 2

# Start pool
./bin/tos-pool --config config/config.yaml &
POOL_PID=$!
sleep 3

# Test connection
timeout 10 ../tosminer/build/bin/tosminer \
    --pool stratum+tcp://127.0.0.1:3333 \
    --user tos1qr8d7mslv5q4zkvhp9vy9aqj0aqwts40xjsz4sqr8d7mslv5q4zkvhp9vy.test \
    2>&1 | grep -q "Authorized" && echo "PASS" || echo "FAIL"

# Cleanup
kill $POOL_PID $NODE_PID 2>/dev/null
docker stop test-redis && docker rm test-redis

echo "Integration test complete"
```

## Protocol Reference

### Stratum Protocol Flow

```
Miner                           Pool
  |                               |
  |------- mining.subscribe ----->|
  |<-------- result --------------|
  |<--- mining.set_difficulty ----|
  |                               |
  |------- mining.authorize ----->|
  |<-------- result (true) -------|
  |<------ mining.notify ---------|
  |                               |
  |------- mining.submit -------->|
  |<-------- result --------------|
```

### Job Format (mining.notify)

```json
{
  "id": null,
  "method": "mining.notify",
  "params": [
    "job_id",        // Job identifier
    "header_hex",    // Block header (hex)
    "target",        // Target difficulty (hex)
    1000001,         // Block height
    true             // Clean jobs flag
  ]
}
```

### Share Submit Format (mining.submit)

```json
{
  "id": 1,
  "method": "mining.submit",
  "params": [
    "worker",        // Worker name
    "job_id",        // Job ID
    "extranonce2",   // Extra nonce 2
    "nonce"          // Nonce (hex)
  ]
}
```
