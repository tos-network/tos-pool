# Additional Features

This document describes optional features that enhance TOS Pool's functionality.

## Table of Contents

- [WebSocket GetWork](#websocket-getwork)
- [Xatum Protocol](#xatum-protocol)
- [pprof Profiling](#pprof-profiling)
- [New Relic APM](#new-relic-apm)

---

## WebSocket GetWork

WebSocket GetWork provides real-time mining work distribution over WebSocket connections. This is an alternative to HTTP polling and Stratum that offers lower latency job notifications.

### Configuration

```yaml
slave:
  websocket_enabled: true
  websocket_bind: "0.0.0.0:3335"
```

### Protocol

The WebSocket server accepts JSON-RPC style messages:

#### Authorization

```json
{
  "id": 1,
  "method": "authorize",
  "params": ["tos1address.workername"]
}
```

Response:
```json
{
  "id": 1,
  "result": true
}
```

#### Get Work

```json
{
  "id": 2,
  "method": "getwork",
  "params": []
}
```

Response:
```json
{
  "id": 2,
  "result": {
    "headerHash": "0x...",
    "target": "0x...",
    "height": 123456,
    "jobId": "job_123",
    "difficulty": 1000000
  }
}
```

#### Submit Work

```json
{
  "id": 3,
  "method": "submit",
  "params": ["job_123", "0xnonce"]
}
```

#### Job Notifications

The server pushes new jobs automatically:

```json
{
  "method": "mining.notify",
  "params": ["job_124", "0xheaderhash", "0xtarget", 123457, true]
}
```

### Benefits

- Lower latency than HTTP polling
- Real-time job notifications
- Persistent connection reduces overhead
- Compatible with web-based miners

---

## Xatum Protocol

Xatum is a TLS-secured JSON mining protocol. It provides a cleaner JSON format than Stratum with mandatory encryption.

### Configuration

```yaml
slave:
  xatum_enabled: true
  xatum_bind: "0.0.0.0:3336"
  tls_cert: "/path/to/cert.pem"
  tls_key: "/path/to/key.pem"
```

**Note:** Xatum requires TLS certificates. The same certificates used for Stratum TLS can be reused.

### Protocol

#### Handshake

Client sends:
```json
{
  "id": "1",
  "method": "handshake",
  "params": {"version": "1.0.0", "protocol": "xatum/1.0"}
}
```

Server responds:
```json
{
  "id": "1",
  "result": {
    "version": "1.0.0",
    "protocol": "xatum/1.0",
    "session_id": "0x1234",
    "difficulty": 1000000
  }
}
```

#### Authorization

```json
{
  "id": "2",
  "method": "authorize",
  "params": {"address": "tos1...", "worker": "rig1"}
}
```

#### Job Notification

Server pushes:
```json
{
  "method": "job",
  "result": {
    "id": "job_123",
    "header_hash": "0x...",
    "target": "0x...",
    "height": 123456,
    "difficulty": 1000000,
    "clean": true
  }
}
```

#### Submit

```json
{
  "id": "3",
  "method": "submit",
  "params": {"job_id": "job_123", "nonce": "0x..."}
}
```

#### Ping/Pong

Client sends ping:
```json
{"id": "4", "method": "ping"}
```

Server responds:
```json
{"method": "pong"}
```

### Benefits

- Mandatory TLS encryption
- Clean JSON format
- Structured parameter objects
- Easy to implement and debug

---

## pprof Profiling

Go's built-in pprof profiling is available for debugging and performance analysis.

### Configuration

```yaml
profiling:
  enabled: true
  bind: "127.0.0.1:6060"  # Only bind to localhost for security
```

### Available Endpoints

| Endpoint | Description |
|----------|-------------|
| `/debug/pprof/` | Index page with links to all profiles |
| `/debug/pprof/goroutine` | Stack traces of all goroutines |
| `/debug/pprof/heap` | Heap memory profile |
| `/debug/pprof/allocs` | Memory allocation profile |
| `/debug/pprof/threadcreate` | Stack traces of threads |
| `/debug/pprof/block` | Blocking profile |
| `/debug/pprof/mutex` | Mutex contention profile |
| `/debug/pprof/profile` | CPU profile (30s default) |
| `/debug/pprof/trace` | Execution trace |

### Usage Examples

#### View goroutine dump
```bash
curl http://127.0.0.1:6060/debug/pprof/goroutine?debug=1
```

#### Capture CPU profile (30 seconds)
```bash
go tool pprof http://127.0.0.1:6060/debug/pprof/profile
```

#### Capture heap profile
```bash
go tool pprof http://127.0.0.1:6060/debug/pprof/heap
```

#### Interactive analysis
```bash
go tool pprof -http=:8081 http://127.0.0.1:6060/debug/pprof/heap
```

### Security Warning

**Never expose the pprof endpoint to the public internet.** The default binding is `127.0.0.1:6060` which only allows local access. If you need remote access, use SSH tunneling or a VPN.

---

## New Relic APM

New Relic Application Performance Monitoring provides real-time insights into pool performance.

### Configuration

```yaml
newrelic:
  enabled: true
  app_name: "TOS Pool Production"
  license_key: "your_license_key_here"
```

### Tracked Metrics

#### Custom Events

| Event Type | Description |
|------------|-------------|
| `ShareSubmission` | Every share submission (address, worker, difficulty, valid/invalid) |
| `BlockFound` | Block discoveries (height, finder, reward) |
| `Payment` | Payouts to miners (address, amount, txHash) |
| `MinerConnected` | New miner connections |
| `MinerDisconnected` | Miner disconnections |

#### Custom Metrics

| Metric | Description |
|--------|-------------|
| `Custom/Pool/Hashrate` | Current pool hashrate |
| `Custom/Pool/Miners` | Active miner count |
| `Custom/Pool/Workers` | Active worker count |
| `Custom/Network/Height` | Current block height |
| `Custom/Network/Difficulty` | Network difficulty |
| `Custom/Network/Hashrate` | Network hashrate |

### Dashboard Examples

Create New Relic dashboards to monitor:

1. **Pool Overview**
   - Hashrate over time
   - Active miners/workers
   - Shares per minute

2. **Block Discovery**
   - Blocks found timeline
   - Block rewards
   - Time between blocks

3. **Miner Activity**
   - Top miners by hashrate
   - Connection/disconnection events
   - Invalid share ratio

### Alerting

Set up New Relic alerts for:

- Pool hashrate drops below threshold
- High invalid share rate
- No blocks found in X hours
- Memory/CPU usage spikes

### Getting Started

1. Sign up for New Relic: https://newrelic.com
2. Create a new application and get your license key
3. Add the configuration to your pool config
4. Restart the pool
5. View data in New Relic dashboards

---

## Complete Configuration Example

```yaml
# Slave server settings
slave:
  enabled: true
  stratum_bind: "0.0.0.0:3333"
  stratum_tls_bind: "0.0.0.0:3334"
  tls_cert: "/etc/ssl/pool/cert.pem"
  tls_key: "/etc/ssl/pool/key.pem"

  # WebSocket GetWork
  websocket_enabled: true
  websocket_bind: "0.0.0.0:3335"

  # Xatum protocol
  xatum_enabled: true
  xatum_bind: "0.0.0.0:3336"

# pprof profiling
profiling:
  enabled: true
  bind: "127.0.0.1:6060"

# New Relic APM
newrelic:
  enabled: true
  app_name: "TOS Pool"
  license_key: "your_license_key_here"
```

## Port Summary

| Port | Protocol | Description |
|------|----------|-------------|
| 3333 | Stratum | Standard mining protocol |
| 3334 | Stratum+TLS | Encrypted Stratum |
| 3335 | WebSocket | WebSocket GetWork |
| 3336 | Xatum | TLS JSON protocol |
| 6060 | HTTP | pprof profiling (localhost only) |
