# TOS Ecosystem API Consistency Check Report

This document provides a detailed comparison of API interface consistency between **tos-pool**, **tosminer**, and **tos daemon** components.

Based on TOS daemon source code analysis (`daemon/src/rpc/rpc.rs`).

---

## Table of Contents

1. [Component Overview](#1-component-overview)
2. [Mining Protocol Consistency](#2-mining-protocol-consistency)
3. [RPC Method Mapping](#3-rpc-method-mapping)
4. [Data Structure Consistency](#4-data-structure-consistency)
5. [TOS Daemon API Reference](#5-tos-daemon-api-reference)
6. [Issues and Recommendations](#6-issues-and-recommendations)
7. [Consistency Matrix](#7-consistency-matrix)
8. [Conclusion](#8-conclusion)

---

## 1. Component Overview

| Component | Language | Role | Primary Interfaces |
|-----------|----------|------|-------------------|
| **tos daemon** | Rust | Blockchain Node | JSON-RPC (80+ methods), GetWork WebSocket |
| **tos-pool** | Go | Mining Pool Proxy | REST API, Stratum, WebSocket, Xatum, GetWork |
| **tosminer** | C++ | Mining Client | HTTP API, Stratum Client |

### Communication Flow

```
┌─────────────┐    Stratum/WS/Xatum    ┌─────────────┐    JSON-RPC    ┌─────────────┐
│   tosminer  │ <───────────────────> │   tos-pool  │ <────────────> │ tos daemon  │
└─────────────┘                        └─────────────┘                └─────────────┘
     Miner                                  Pool                           Node
```

### Communication Architecture Details

The TOS mining ecosystem uses a two-layer communication architecture:

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                           LAYER 1: Stratum Protocol                              │
│                         (tosminer ↔ tos-pool)                                    │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌─────────────┐                                         ┌─────────────┐         │
│  │  tosminer   │  ──── mining.subscribe ────────────────>│             │         │
│  │  (C++)      │  ──── mining.authorize ────────────────>│  tos-pool   │         │
│  │             │  ──── mining.submit ───────────────────>│  (Go)       │         │
│  │  Stratum    │  <─── mining.notify ────────────────────│             │         │
│  │  Client     │  <─── mining.set_difficulty ────────────│  Stratum    │         │
│  │             │  ──── mining.ping ─────────────────────>│  Server     │         │
│  └─────────────┘                                         └─────────────┘         │
│                                                                                  │
│  Protocol: Stratum (TCP/TLS)                                                     │
│  Format: JSON-RPC with array params                                              │
│  Status: FULLY ALIGNED                                                           │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────────────────┐
│                           LAYER 2: Daemon RPC                                    │
│                         (tos-pool ↔ tos daemon)                                  │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌─────────────┐                                         ┌─────────────┐         │
│  │  tos-pool   │  ──── get_block_template ──────────────>│             │         │
│  │  (Go)       │  ──── submit_block ────────────────────>│ tos daemon  │         │
│  │             │  ──── get_block_at_topoheight ─────────>│ (Rust)      │         │
│  │  RPC        │  ──── get_balance ─────────────────────>│             │         │
│  │  Adapter    │  ──── get_nonce ───────────────────────>│  JSON-RPC   │         │
│  │  Layer      │  ──── p2p_status ──────────────────────>│  Server     │         │
│  └─────────────┘                                         └─────────────┘         │
│                                                                                  │
│  Protocol: JSON-RPC 2.0 (HTTP)                                                   │
│  Format: JSON-RPC with object params                                             │
│  Status: REQUIRES ADAPTER LAYER                                                  │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

#### Key Points

1. **tosminer does NOT communicate directly with tos daemon**
   - tosminer only uses Stratum protocol to communicate with tos-pool
   - tosminer does not need to know about TOS daemon's native RPC API

2. **tos-pool acts as the bridge**
   - Receives work requests from miners via Stratum
   - Fetches block templates from daemon via JSON-RPC
   - Validates and submits blocks to daemon

3. **Adapter Layer Location**
   - The adapter layer (`internal/rpc/adapter.go`) is in tos-pool
   - Converts Ethereum-style RPC calls to TOS daemon native methods
   - Handles parameter format conversion (array → object)
   - Handles response format conversion

#### Protocol Comparison

| Aspect | Layer 1 (Stratum) | Layer 2 (Daemon RPC) |
|--------|-------------------|----------------------|
| Endpoints | tosminer ↔ tos-pool | tos-pool ↔ tos daemon |
| Protocol | Stratum (JSON-RPC variant) | JSON-RPC 2.0 |
| Transport | TCP/TLS | HTTP |
| Params Format | Array `[]` | Object `{}` |
| Hex Prefix | No `0x` prefix | No `0x` prefix |
| Alignment Status | Fully Aligned | Requires Adapter |

---

## 2. Mining Protocol Consistency

### 2.1 Stratum Protocol Method Comparison

| Method | tosminer (Client) | tos-pool (Server) | Consistency |
|--------|-------------------|-------------------|-------------|
| `mining.subscribe` | Sends | Handles | Consistent |
| `mining.authorize` | Sends | Handles | Consistent |
| `mining.submit` | Sends | Handles | Compatible (pool supports 4/5 params) |
| `mining.notify` | Receives | Sends | Compatible (normalize `0x` prefix) |
| `mining.set_difficulty` | Receives | Sends | Consistent |
| `mining.ping` | Sends | Handles | Consistent |
| `mining.extranonce.subscribe` | Not sent | Handles | N/A (pool supports optional) |

#### 2.1.1 `mining.subscribe` Detailed Comparison

**tosminer Request Format:**
```json
{
  "id": 1,
  "method": "mining.subscribe",
  "params": ["tosminer/1.0.0"]
}
```

**tos-pool Response Format:**
```json
{
  "id": 1,
  "result": [
    [["mining.notify", "subscription_id"], ["mining.set_difficulty", "subscription_id"]],
    "extranonce1_hex",
    4
  ]
}
```

**Consistency:** Fully consistent

#### 2.1.2 `mining.authorize` Detailed Comparison

**tosminer Request Format:**
```json
{
  "id": 2,
  "method": "mining.authorize",
  "params": ["wallet.worker", "password"]
}
```

**tos-pool Handling:**
- Username format: `address.worker`
- Password: Optional (typically "x")
- Validation: Address format validation

**Consistency:** Fully consistent

#### 2.1.3 `mining.submit` Detailed Comparison

**tosminer Request Format:**
```json
{
  "id": 100,
  "method": "mining.submit",
  "params": ["wallet.worker", "job_id", "extranonce2", "nonce"]
}
```

**tos-pool Accepted Formats:**
- Format 1 (4 params): `[worker, job_id, extranonce2, nonce]`
- Format 2 (5 params): `[worker, job_id, extranonce2, ntime, nonce]`

**Consistency:** Note parameter count difference; tos-pool supports both formats

#### 2.1.4 `mining.notify` Detailed Comparison

**tos-pool Send Format (TOS Simplified Format):**
```json
{
  "id": null,
  "method": "mining.notify",
  "params": ["job_id", "header_hex", "target_hex", height, clean_jobs]
}
```

- `job_id`: Job identifier
- `header_hex`: Block header (224 chars = 112 bytes). **No `0x` prefix.**
- `target_hex`: Target difficulty (256-bit). **No `0x` prefix.**
- `height`: Block height
- `clean_jobs`: Whether to clear old jobs

**tosminer Expected Format:**
```
[job_id, header_hex, target_hex, height, clean_jobs]
```

**Consistency:** Fully consistent (0x prefix issue fixed in tos-pool).

---

### 2.2 Pool to Daemon RPC Consistency

| tos-pool Call | tos daemon Method | Consistency |
|---------------|-------------------|-------------|
| `tos_getWork` | `get_block_template` | Needs adapter (name/params/target conversion) |
| `tos_getBlockTemplate` | `get_block_template` | Needs adapter (response shape differs) |
| `tos_submitWork` | `submit_block` | Needs adapter (params differ; object params) |
| `tos_submitBlock` | `submit_block` | Needs adapter (params differ; object params) |
| `tos_getBlockByNumber` | `get_block_at_topoheight` / `get_top_block` | Needs adapter (`latest` mapping; response differs) |
| `tos_getBlockByHash` | `get_block_by_hash` | Needs adapter (response differs) |
| `tos_blockNumber` | `get_topoheight` | Needs adapter (number vs hex string) |
| `net_peerCount` | `p2p_status` | Needs adapter (object vs hex string) |
| `tos_getBalance` | `get_balance` | Needs adapter (asset required; object vs hex string) |
| `tos_getTransactionReceipt` | `get_transaction` / `get_transaction_executor` | No direct receipt; needs custom mapping |
| `tos_sendRawTransaction` | `submit_transaction` | Needs adapter (bool vs tx hash) |
| `tos_getTransactionCount` | `get_nonce` | Needs adapter (object vs hex string) |
| `tos_estimateGas` | N/A | TOS has no gas concept |
| `tos_gasPrice` | N/A | TOS has no gas concept |
| `tos_syncing` | N/A | Use `get_info` instead |

---

## 3. RPC Method Mapping

### 3.1 tos-pool to tos daemon Method Mapping

#### 3.1.1 Mining Related

| tos-pool Call | tos daemon Actual Method | Status |
|---------------|--------------------------|--------|
| `tos_getWork` | `get_block_template` | Needs adapter layer |
| `tos_getBlockTemplate` | `get_block_template` | Needs adapter layer (response shape differs) |
| `tos_submitWork` | `submit_block` | Needs adapter layer |
| `tos_submitBlock` | `submit_block` | Needs adapter layer (params shape differs) |

**Note:** Daemon mining methods (`get_block_template`, `get_miner_work`, `submit_block`) are only registered when `allow_mining_methods` is enabled.

**tos daemon `get_block_template` (rpc.rs:730)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_block_template",
  "params": {
    "address": "tos1..."
  },
  "id": 1
}
```

Response (`GetBlockTemplateResult`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "template": "hex_encoded_block_header",
    "algorithm": "tos/v3",
    "height": 12345,
    "topoheight": 12345,
    "difficulty": "1000000"
  }
}
```

**tos daemon `submit_block` (rpc.rs:817)**

Request (submit solved header directly; `miner_work` omitted):
```json
{
  "jsonrpc": "2.0",
  "method": "submit_block",
  "params": {
    "block_template": "hex_encoded_block_header_with_nonce"
  },
  "id": 1
}
```

Request (optional `miner_work` applied to `block_template`):
```json
{
  "jsonrpc": "2.0",
  "method": "submit_block",
  "params": {
    "block_template": "hex_encoded_block_header",
    "miner_work": "hex_encoded_miner_work"
  },
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": true
}
```

**tos daemon `get_miner_work` (rpc.rs:769)**

Request (address optional):
```json
{
  "jsonrpc": "2.0",
  "method": "get_miner_work",
  "params": {
    "template": "hex_encoded_block_template"
  },
  "id": 1
}
```

Response (`GetMinerWorkResult`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "algorithm": "tos/v3",
    "miner_work": "hex_encoded_miner_job",
    "height": 12345,
    "difficulty": "1000000",
    "topoheight": 12345
  }
}
```

#### 3.1.2 Block Query Related

| tos-pool Call | tos daemon Method | Parameter Mapping |
|---------------|-------------------|-------------------|
| `tos_getBlockByNumber(0x...)` | `get_block_at_topoheight` | hex to topoheight |
| `tos_getBlockByHash(hash)` | `get_block_by_hash` | Direct mapping |
| `tos_blockNumber` | `get_topoheight` | Return format hex |

**tos daemon `get_block_at_topoheight` (rpc.rs:692)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_block_at_topoheight",
  "params": {
    "topoheight": 12345,
    "include_txs": false
  },
  "id": 1
}
```

Response (`RPCBlockResponse`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "hash": "block_hash_hex",
    "topoheight": 12345,
    "block_type": "Normal",
    "difficulty": "1000000",
    "supply": 100000000000000,
    "reward": 100000000,
    "miner_reward": 90000000,
    "dev_reward": 10000000,
    "cumulative_difficulty": "12345000000",
    "total_fees": 0,
    "total_size_in_bytes": 1024,
    "version": 1,
    "tips": ["parent_hash_1", "parent_hash_2"],
    "timestamp": 1734567890000,
    "height": 12345,
    "nonce": 123456789,
    "extra_nonce": "0000000000000000000000000000000000000000000000000000000000000000",
    "miner": "tos1...",
    "txs_hashes": ["tx_hash_1", "tx_hash_2"],
    "transactions": []
  }
}
```

**Key differences from Ethereum-style:**
- `topoheight` instead of `number` (TOS uses DAG)
- `tips[]` instead of single `parentHash` (DAG can have multiple parents)
- `block_type`: "Sync", "Side", "Orphaned", "Normal"
- `miner_reward` and `dev_reward` separate from total `reward`
- `timestamp` in milliseconds
- No `gasUsed`, `gasLimit` fields (TOS has no gas concept)

**tos daemon `get_block_by_hash` (rpc.rs:706)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_block_by_hash",
  "params": {
    "hash": "block_hash_hex",
    "include_txs": false
  },
  "id": 1
}
```

Response: Same as `get_block_at_topoheight`

**tos daemon `get_top_block` (rpc.rs:716)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_top_block",
  "params": {
    "include_txs": false
  },
  "id": 1
}
```

Response: Same as `get_block_at_topoheight`

#### 3.1.3 Height/TopoHeight Methods

**tos daemon `get_height` (rpc.rs:632)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_height",
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": 12345
}
```

**tos daemon `get_topoheight` (rpc.rs:638)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_topoheight",
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": 12345
}
```

**tos daemon `get_stable_height` (rpc.rs:663)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_stable_height",
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": 12337
}
```

**tos daemon `get_stable_topoheight` (rpc.rs:672)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_stable_topoheight",
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": 12337
}
```

#### 3.1.4 Account/Balance Methods

**tos daemon `get_balance` (rpc.rs:841)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_balance",
  "params": {
    "address": "tos1...",
    "asset": "0000000000000000000000000000000000000000000000000000000000000000"
  },
  "id": 1
}
```

Response (`GetBalanceResult`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "balance": 100000000000000,
    "topoheight": 12345
  }
}
```

**Note:** The `asset` parameter is required. Use 64 zeros for native TOS token.

**tos daemon `get_balance_at_topoheight` (rpc.rs:1020)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_balance_at_topoheight",
  "params": {
    "address": "tos1...",
    "asset": "0000000000000000000000000000000000000000000000000000000000000000",
    "topoheight": 12345
  },
  "id": 1
}
```

Response (`VersionedBalance`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "previous_topoheight": 12300,
    "output_balance": null,
    "final_balance": 100000000000000,
    "balance_type": "input"
  }
}
```

**tos daemon `has_balance` (rpc.rs:919)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "has_balance",
  "params": {
    "address": "tos1...",
    "asset": "0000000000000000000000000000000000000000000000000000000000000000"
  },
  "id": 1
}
```

**Note:** `topoheight` is optional; when provided, it checks balance existence at that exact topoheight.

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "exist": true
  }
}
```

**tos daemon `get_nonce` (rpc.rs:1075)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_nonce",
  "params": {
    "address": "tos1..."
  },
  "id": 1
}
```

Response (`GetNonceResult`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "topoheight": 12345,
    "nonce": 0,
    "previous_topoheight": null
  }
}
```

**tos daemon `get_nonce_at_topoheight` (rpc.rs:1096)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_nonce_at_topoheight",
  "params": {
    "address": "tos1...",
    "topoheight": 12345
  },
  "id": 1
}
```

Response (`VersionedNonce`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "nonce": 0,
    "previous_topoheight": null
  }
}
```

**tos daemon `has_nonce` (rpc.rs:1050)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "has_nonce",
  "params": {
    "address": "tos1..."
  },
  "id": 1
}
```

**Note:** `topoheight` is optional; when provided, it checks nonce existence at that exact topoheight.

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "exist": true
  }
}
```

#### 3.1.5 Transaction Methods

**tos daemon `submit_transaction` (rpc.rs:1255)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "submit_transaction",
  "params": {
    "data": "hex_encoded_transaction"
  },
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": true
}
```

**tos daemon `get_transaction` (rpc.rs:1277)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_transaction",
  "params": {
    "hash": "tx_hash_hex"
  },
  "id": 1
}
```

Response (`TransactionResponse`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "hash": "tx_hash_hex",
    "blocks": ["block_hash_1"],
    "executed_in_block": "block_hash_1",
    "in_mempool": false,
    "first_seen": null,
    "version": 0,
    "source": "tos1...",
    "data": {
      "Transfer": [{
        "asset": "0000000000000000000000000000000000000000000000000000000000000000",
        "amount": 1000000000,
        "destination": "tos1..."
      }]
    },
    "fee": 1000,
    "nonce": 0,
    "reference": {
      "topoheight": 12344,
      "hash": "reference_block_hash"
    },
    "multisig": null,
    "signature": "signature_hex",
    "size": 256
  }
}
```

**tos daemon `get_transactions` (rpc.rs:1623)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_transactions",
  "params": {
    "tx_hashes": ["tx_hash_1", "tx_hash_2"]
  },
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": [
    { /* TransactionResponse */ },
    { /* TransactionResponse */ }
  ]
}
```

**tos daemon `get_transaction_executor` (rpc.rs:1289)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_transaction_executor",
  "params": {
    "hash": "tx_hash_hex"
  },
  "id": 1
}
```

Response (`GetTransactionExecutorResult`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "block_topoheight": 12345,
    "block_timestamp": 1734567890000,
    "block_hash": "block_hash_hex"
  }
}
```

**tos daemon `get_mempool` (rpc.rs:1365)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_mempool",
  "params": {
    "maximum": 100,
    "skip": 0
  },
  "id": 1
}
```

Response (`GetMempoolResult`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "transactions": [],
    "total": 0
  }
}
```

#### 3.1.6 Network Methods

**tos daemon `p2p_status` (rpc.rs:1310)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "p2p_status",
  "id": 1
}
```

Response (`P2pStatusResult`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "peer_count": 10,
    "max_peers": 32,
    "tag": null,
    "our_topoheight": 12345,
    "best_topoheight": 12345,
    "median_topoheight": 12345,
    "peer_id": 1234567890
  }
}
```

**tos daemon `get_info` (rpc.rs:948)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_info",
  "id": 1
}
```

Response (`GetInfoResult`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "height": 12345,
    "topoheight": 12345,
    "stableheight": 12337,
    "stable_topoheight": 12337,
    "pruned_topoheight": null,
    "top_block_hash": "top_block_hash_hex",
    "circulating_supply": 100000000000000,
    "burned_supply": 0,
    "emitted_supply": 100000000000000,
    "maximum_supply": 1000000000000000,
    "difficulty": "1000000",
    "block_time_target": 3000,
    "average_block_time": 3000,
    "block_reward": 100000000,
    "dev_reward": 10000000,
    "miner_reward": 90000000,
    "mempool_size": 0,
    "version": "1.0.0",
    "network": "mainnet",
    "block_version": 0
  }
}
```

**tos daemon `get_version` (rpc.rs:627)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_version",
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "1.0.0"
}
```

**tos daemon `get_difficulty`**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_difficulty",
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "1000000"
}
```

**tos daemon `get_tips`**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_tips",
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": ["tip_hash_1", "tip_hash_2"]
}
```

**tos daemon `get_peers` (rpc.rs:1339)**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "get_peers",
  "id": 1
}
```

Response (`GetPeersResponse`):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "peers": [
      {
        "id": 1234567890,
        "addr": "192.168.1.1:8080",
        "local_port": 8080,
        "tag": null,
        "version": "1.0.0",
        "top_block_hash": "hash",
        "topoheight": 12345,
        "height": 12345,
        "last_ping": 1734567890,
        "pruned_topoheight": null,
        "peers": {},
        "cumulative_difficulty": "12345000000",
        "connected_on": 1734567000,
        "bytes_sent": 1024,
        "bytes_recv": 2048
      }
    ],
    "total_peers": 10,
    "hidden_peers": 0
  }
}
```

---

## 4. Data Structure Consistency

### 4.1 Work Package Structure Comparison

#### tosminer `WorkPackage`:
```cpp
struct WorkPackage {
    std::string jobId;
    std::array<uint8_t, 112> header;  // 112 bytes
    Hash256 target;                    // 32 bytes
    uint64_t height;
    Nonce startNonce;
    std::string extraNonce1;
    unsigned extraNonce2Size;          // 4-8 bytes
    Hash256 headerHash;
    bool valid;
};
```

#### tos-pool `GetworkJob`:
```go
type GetworkJob struct {
    JobID      string `json:"job_id"`
    HeaderHash string `json:"header_hash"`
    Target     string `json:"target"`
    Height     uint64 `json:"height"`
    Difficulty uint64 `json:"difficulty"`
    Timestamp  int64  `json:"timestamp"`
}
```

#### tos daemon `GetBlockTemplateResult`:
```rust
struct GetBlockTemplateResult {
    template: String,           // hex encoded block header
    algorithm: Algorithm,       // "tos/v1", "tos/v2", "tos/v3" (currently v3)
    height: u64,
    topoheight: TopoHeight,
    difficulty: Difficulty,
}
```

**Consistency Analysis:**

| Field | tosminer | tos-pool | tos daemon | Consistency |
|-------|----------|----------|------------|-------------|
| Job ID | `jobId` | `JobID` | N/A (pool generated) | OK |
| Header | `header[112]` | `HeaderHash` (actually header hex) | `template` (header hex) | Same payload, different naming |
| Target | `target` | `Target` | `difficulty` | Needs conversion |
| Height | `height` | `Height` | `height` | OK |
| ExtraNonce1 | `extraNonce1` | Stratum: `ExtraNonce1` | Encoded inside `template` | Pool managed |
| ExtraNonce2Size | `extraNonce2Size` | N/A | N/A | Pool managed |

### 4.2 Share Submission Structure Comparison

#### tosminer Submission:
```json
{
  "id": 100,
  "method": "mining.submit",
  "params": ["worker", "job_id", "extranonce2", "nonce"]
}
```

#### tos-pool Handling:
```go
type Share struct {
    Address    string
    Worker     string
    JobID      string
    Nonce      string
    Hash       string
    Difficulty uint64
    Height     uint64
    Timestamp  int64
    Valid      bool
    BlockHash  string
}
```

#### tos daemon Submission:
```json
{
  "method": "submit_block",
  "params": {
    "block_template": "hex_string",
    "miner_work": "hex_string"
  }
}
```

**Consistency Issues:**
- tosminer to tos-pool: Consistent
- tos-pool to tos daemon: Requires block_template reconstruction

---

## 5. TOS Daemon API Reference

This section lists the daemon methods most relevant to mining/pool integrations. The daemon registers many more JSON-RPC methods; see `tos/daemon/src/rpc/rpc.rs` (`register_methods`) for the authoritative list.

### 5.1 Mining Methods

| Method | Description |
|--------|-------------|
| `get_block_template` | Get block template for mining |
| `submit_block` | Submit solved block |
| `get_miner_work` | Get miner work from template |

**Note:** Mining methods are only available when `allow_mining_methods` is enabled on the daemon.

### 5.2 Block Query Methods

| Method | Description |
|--------|-------------|
| `get_height` | Get current blockchain height |
| `get_topoheight` | Get current topological height |
| `get_stable_height` | Get stable (finalized) height |
| `get_stable_topoheight` | Get stable topological height |
| `get_block_at_topoheight` | Get block at specific topoheight |
| `get_block_by_hash` | Get block by hash |
| `get_top_block` | Get top/best block |
| `get_blocks_at_height` | Get blocks at height (DAG) |
| `get_tips` | Get current DAG tips |
| `get_dag_order` | Get blocks in topological order |
| `get_difficulty` | Get current network difficulty |

### 5.3 Account Methods

| Method | Description |
|--------|-------------|
| `get_balance` | Get account balance (requires asset) |
| `get_balance_at_topoheight` | Get balance at specific topoheight |
| `has_balance` | Check if account has balance |
| `get_nonce` | Get account nonce |
| `get_nonce_at_topoheight` | Get nonce at specific topoheight |
| `has_nonce` | Check if account has nonce |

### 5.4 Transaction Methods

| Method | Description |
|--------|-------------|
| `submit_transaction` | Submit transaction to mempool |
| `get_transaction` | Get transaction by hash |
| `get_transactions` | Get multiple transactions |
| `get_transaction_executor` | Get block where tx was executed |
| `get_mempool` | Get mempool transactions |

### 5.5 Network Methods

| Method | Description |
|--------|-------------|
| `p2p_status` | Get P2P network status |
| `get_info` | Get complete daemon info |
| `get_version` | Get daemon version |
| `get_peers` | Get connected peers |

### 5.6 Asset Methods

| Method | Description |
|--------|-------------|
| `get_asset` | Get asset info (decimals, etc.) |

---

## 6. Issues and Recommendations

### 6.1 Identified Consistency Issues

#### 6.1.1 Critical Issues (Requires Adapter Layer)

| Issue | Description | Impact | Recommendation |
|-------|-------------|--------|----------------|
| **RPC Method Name Mismatch** | tos-pool uses `tos_getWork`, daemon uses `get_block_template` | Cannot communicate directly | Add adapter layer in tos-pool |
| **Response Structure Difference** | daemon returns Rust struct, pool expects Ethereum style | Parsing failure | Implement response converter |
| **Missing Methods** | `tos_estimateGas`, `tos_gasPrice`, `tos_syncing` | Some features unavailable | Remove dependency (TOS has no gas) |

#### 6.1.2 Medium Issues (Needs Attention)

| Issue | Description | Recommendation |
|-------|-------------|----------------|
| **Height vs TopoHeight** | TOS uses DAG with both concepts | Use topoheight consistently |
| **Asset Parameter** | `get_balance` requires asset | Pool defaults to native token (64 zeros) |
| **Tips vs ParentHash** | DAG has multiple parents | Use tips[0] as primary parent |
| **Stratum Hex Prefix** | ~~tos-pool prefixes `0x`~~ **FIXED** - No longer prefixes `0x` | ✅ Resolved |

#### 6.1.3 Minor Issues (Can Ignore)

| Issue | Description |
|-------|-------------|
| Case sensitivity | camelCase vs snake_case |
| Timestamp format | TOS uses milliseconds |

### 6.2 Recommended Solutions

#### Solution A: Add Adapter Layer in tos-pool

```go
// internal/rpc/adapter.go

type TOSAdapter struct {
    client *TOSClient
}

// Convert Ethereum style calls to TOS daemon calls
func (a *TOSAdapter) GetWork() (*GetWorkResult, error) {
    // Call get_block_template
    template, err := a.client.GetBlockTemplate(minerAddress)
    if err != nil {
        return nil, err
    }

    // Convert to Ethereum style response
    return &GetWorkResult{
        HeaderHash: template.Template,
        Target:     difficultyToTarget(template.Difficulty),
        Height:     template.Height,
    }, nil
}
```

#### Solution B: Add Compatible API in tos daemon

```rust
// Add Ethereum compatible RPC methods in daemon

#[rpc_method("tos_getWork")]
async fn tos_get_work(&self) -> Result<[String; 4], Error> {
    let template = self.get_block_template(addr).await?;
    Ok([
        template.header_hash(),
        "".to_string(),  // seedHash (unused)
        template.target_hex(),
        template.height.to_string(),
    ])
}
```

### 6.3 Priority Ranking

| Priority | Issue | Action |
|----------|-------|--------|
| **P0** | RPC method name mismatch | Implement adapter layer |
| **P0** | Response structure difference | Implement response converter |
| **P1** | Missing RPC methods | Evaluate if necessary |
| **P2** | DAG related fields | Documentation |
| **P3** | Format differences (hex, case) | Gradual fix |

---

## 7. Consistency Matrix

### 7.1 Stratum Protocol Consistency (tosminer to tos-pool)

| Feature | Status | Notes |
|---------|--------|-------|
| mining.subscribe | Fully consistent | |
| mining.authorize | Fully consistent | |
| mining.submit | Compatible | Supports 4/5 parameter formats |
| mining.notify | Fully consistent | ✅ 0x prefix issue fixed |
| mining.set_difficulty | Fully consistent | |
| mining.ping | Fully consistent | |
| Connection management | Consistent | TLS support, reconnection |

**Overall Assessment:** **Fully Consistent** (0x prefix issue fixed).

### 7.2 RPC Protocol Consistency (tos-pool to tos daemon)

| Feature | Status | Notes |
|---------|--------|-------|
| Get work | Needs adaptation | Method name and response differ |
| Submit block | Needs adaptation | Parameter structure differs |
| Get block info | Needs adaptation | DAG characteristics |
| Get balance | Needs adaptation | Requires asset parameter |
| Get nonce | Needs adaptation | Response is an object (not hex) |
| Submit transaction | Needs adaptation | Returns bool (not tx hash) |
| Gas related | Not supported | TOS has no Gas concept |
| Sync status | Use get_info | |

**Overall Assessment:** **Adapter Layer Implemented** (`internal/rpc/adapter.go`)

### 7.3 Data Structure Consistency

| Structure | tosminer to tos-pool | tos-pool to tos daemon |
|-----------|---------------------|----------------------|
| WorkPackage | Consistent | Needs conversion |
| Share | Consistent | N/A |
| Block | N/A | DAG differences |
| Transaction | N/A | Field differences |

---

## 8. Conclusion

### 8.1 Overall Consistency Assessment

| Communication Link | Consistency Score | Status |
|--------------------|-------------------|--------|
| tosminer to tos-pool | **100%** | ✅ Fully Consistent |
| tos-pool to tos daemon | **100%** | ✅ Adapter Layer Implemented |

### 8.2 Key Takeaways

1. **TOS is not Ethereum**: TOS uses DAG structure with `topoheight`, no gas concept
2. **Native asset hash**: `0000000000000000000000000000000000000000000000000000000000000000`
3. **POW Algorithm**: `tos/v3` (versions: `tos/v1`, `tos/v2`, `tos/v3` - currently all use v3)
4. **Timestamps**: Milliseconds (not seconds)
5. **JSON-RPC params**: Object style `{}`, not array style `[]`

### 8.3 Implementation Status

1. ✅ **DONE**: TOS daemon adapter layer implemented (`internal/rpc/adapter.go`)
2. ✅ **DONE**: Stratum work hex normalized (no `0x` prefix in `mining.notify`)
3. **Future**: Evaluate compatible API in daemon (optional)
4. **Future**: Shared protobuf definitions (optional)

---

*Document updated: 2025-12-19*
*Based on: TOS daemon source code (daemon/src/rpc/rpc.rs)*
*Applicable versions: tos-pool v1.0, tosminer v1.0, tos daemon (latest)*
