# TOS Ecosystem API Consistency Check Report

This document provides a detailed comparison of API interface consistency between **tos-pool**, **tosminer**, and **tos daemon** components.

---

## Table of Contents

1. [Component Overview](#1-component-overview)
2. [Mining Protocol Consistency](#2-mining-protocol-consistency)
3. [RPC Method Mapping](#3-rpc-method-mapping)
4. [Data Structure Consistency](#4-data-structure-consistency)
5. [Issues and Recommendations](#5-issues-and-recommendations)
6. [Consistency Matrix](#6-consistency-matrix)

---

## 1. Component Overview

| Component | Language | Role | Primary Interfaces |
|-----------|----------|------|-------------------|
| **tos daemon** | Rust | Blockchain Node | JSON-RPC (81 methods), GetWork WebSocket |
| **tos-pool** | Go | Mining Pool Proxy | REST API, Stratum, WebSocket, Xatum, GetWork |
| **tosminer** | C++ | Mining Client | HTTP API, Stratum Client |

### Communication Flow

```
┌─────────────┐    Stratum/WS/Xatum    ┌─────────────┐    JSON-RPC    ┌─────────────┐
│   tosminer  │ <───────────────────> │   tos-pool  │ <────────────> │ tos daemon  │
└─────────────┘                        └─────────────┘                └─────────────┘
     Miner                                  Pool                           Node
```

---

## 2. Mining Protocol Consistency

### 2.1 Stratum Protocol Method Comparison

| Method | tosminer (Client) | tos-pool (Server) | Consistency |
|--------|-------------------|-------------------|-------------|
| `mining.subscribe` | ✅ Sends | ✅ Handles | ✅ Consistent |
| `mining.authorize` | ✅ Sends | ✅ Handles | ✅ Consistent |
| `mining.submit` | ✅ Sends | ✅ Handles | ⚠️ Parameter format differs |
| `mining.notify` | ✅ Receives | ✅ Sends | ⚠️ Parameter format differs |
| `mining.set_difficulty` | ✅ Receives | ✅ Sends | ✅ Consistent |
| `mining.ping` | ✅ Sends | ✅ Handles | ✅ Consistent |
| `mining.extranonce.subscribe` | ✅ Sends | ✅ Handles | ✅ Consistent |

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

**Consistency:** ✅ Fully consistent

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

**Consistency:** ✅ Fully consistent

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

**Consistency:** ⚠️ Note parameter count difference; tos-pool supports both formats

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
- `header_hex`: Block header (224 chars = 112 bytes)
- `target_hex`: Target difficulty (256-bit)
- `height`: Block height
- `clean_jobs`: Whether to clear old jobs

**tosminer Expected Format:**
```
[job_id, header_hex, target_hex, height, clean_jobs]
```

**Consistency:** ✅ Consistent (TOS simplified format)

**Note:** tosminer also supports standard Stratum format as fallback:
```
[job_id, prev_hash, coinbase1, coinbase2, merkle_branches[], version, nbits, ntime, clean_jobs]
```

---

### 2.2 Pool ↔ Daemon RPC Consistency

| tos-pool Call | tos daemon Method | Consistency |
|---------------|-------------------|-------------|
| `tos_getWork` | `get_block_template` | ⚠️ Different names, needs mapping |
| `tos_getBlockTemplate` | `get_block_template` | ✅ Semantically consistent |
| `tos_submitWork` | `submit_block` | ⚠️ Different names, needs mapping |
| `tos_submitBlock` | `submit_block` | ✅ Semantically consistent |
| `tos_getBlockByNumber` | `get_block_at_topoheight` | ⚠️ Parameter format differs |
| `tos_getBlockByHash` | `get_block_by_hash` | ✅ Semantically consistent |
| `tos_blockNumber` | `get_height` / `get_topoheight` | ⚠️ TOS uses topoheight |
| `net_peerCount` | `p2p_status` | ⚠️ Response structure differs |
| `tos_getBalance` | `get_balance` | ✅ Semantically consistent |
| `tos_getTransactionReceipt` | `get_transaction` | ⚠️ Response structure differs |
| `tos_sendRawTransaction` | `submit_transaction` | ✅ Semantically consistent |
| `tos_getTransactionCount` | `get_nonce` | ✅ Semantically consistent |
| `tos_estimateGas` | ❌ Does not exist | ❌ Missing |
| `tos_gasPrice` | ❌ Does not exist | ❌ Missing |
| `tos_syncing` | ❌ Does not exist | ❌ Missing |

---

## 3. RPC Method Mapping

### 3.1 tos-pool → tos daemon Method Mapping

#### 3.1.1 Mining Related

| tos-pool Call | tos daemon Actual Method | Status |
|---------------|--------------------------|--------|
| `tos_getWork` | `get_block_template` | ⚠️ Needs adapter layer |
| `tos_getBlockTemplate` | `get_block_template` | ✅ |
| `tos_submitWork` | `submit_block` | ⚠️ Needs adapter layer |
| `tos_submitBlock` | `submit_block` | ✅ |

**tos daemon `get_block_template` Request:**
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

**tos daemon `get_block_template` Response:**
```json
{
  "template": "hex_string",
  "algorithm": "V3",
  "height": 12345,
  "topoheight": 12345,
  "difficulty": "Difficulty"
}
```

**tos-pool Expected `tos_getWork` Response:**
```json
["headerHash", "seedHash", "target", "height"]
```

**Consistency Issue:** ❌ Completely different response structures, requires adapter layer conversion

#### 3.1.2 Block Query Related

| tos-pool Call | tos daemon Method | Parameter Mapping |
|---------------|-------------------|-------------------|
| `tos_getBlockByNumber(0x...)` | `get_block_at_topoheight` | hex → topoheight |
| `tos_getBlockByHash(hash)` | `get_block_by_hash` | ✅ Direct mapping |
| `tos_blockNumber` | `get_topoheight` | Return format hex |

**tos daemon `get_block_by_hash` Response Structure:**
```json
{
  "hash": "Hash",
  "topoheight": 12345,
  "block_type": "Normal",
  "difficulty": "Difficulty",
  "supply": 12345,
  "reward": 12345,
  "miner": "Address",
  "nonce": "u64",
  "extra_nonce": "Hash",
  "timestamp": 1234567890,
  "height": 12345,
  "tips": ["Hash"],
  "txs_hashes": ["Hash"],
  "version": 1
}
```

**tos-pool Expected `BlockInfo` Structure:**
```go
type BlockInfo struct {
    Hash             string
    ParentHash       string
    Number           uint64
    Timestamp        uint64
    Difficulty       uint64
    TotalDifficulty  string
    Nonce            string
    Miner            string
    Reward           uint64
    Size             uint64
    GasUsed          uint64
    GasLimit         uint64
    TransactionCount int
    TxFees           uint64
}
```

**Consistency Issues:** ⚠️ Significant field differences
- TOS uses `topoheight` instead of `number`
- TOS uses `tips[]` (DAG) instead of single `parentHash`
- TOS has no `gasUsed`, `gasLimit` fields

#### 3.1.3 Account/Transaction Related

| tos-pool Call | tos daemon Method | Consistency |
|---------------|-------------------|-------------|
| `tos_getBalance(addr, "latest")` | `get_balance(addr, asset)` | ⚠️ asset parameter required |
| `tos_getTransactionCount(addr)` | `get_nonce(addr)` | ✅ Semantically consistent |
| `tos_sendRawTransaction(tx)` | `submit_transaction(data)` | ✅ |
| `tos_getTransactionReceipt(hash)` | `get_transaction(hash)` | ⚠️ Structure differs |

**tos daemon `get_balance` Request:**
```json
{
  "method": "get_balance",
  "params": {
    "address": "tos1...",
    "asset": "0000000000000000000000000000000000000000000000000000000000000000"
  }
}
```

**Consistency Issue:** ⚠️ TOS requires asset hash specification; native token uses all-zero hash

---

### 3.2 tosminer → tos-pool Method Mapping

| tosminer Method | tos-pool Handler | Consistency |
|-----------------|------------------|-------------|
| `mining.subscribe` | `handleSubscribe()` | ✅ |
| `mining.authorize` | `handleAuthorize()` | ✅ |
| `mining.submit` | `handleSubmit()` | ✅ |
| `eth_submitLogin` | `handleAuthorize()` | ✅ (alias) |

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

#### tos daemon `BlockTemplate`:
```rust
struct GetBlockTemplateResult {
    template: String,      // hex encoded block header
    algorithm: AlgorithmType,
    height: u64,
    topoheight: u64,
    difficulty: Difficulty,
}
```

**Consistency Analysis:**

| Field | tosminer | tos-pool | tos daemon | Consistency |
|-------|----------|----------|------------|-------------|
| Job ID | `jobId` | `JobID` | N/A (pool generated) | ✅ |
| Header | `header[112]` | `HeaderHash` | `template` | ⚠️ Different formats |
| Target | `target` | `Target` | `difficulty` | ⚠️ Needs conversion |
| Height | `height` | `Height` | `height` | ✅ |
| ExtraNonce1 | `extraNonce1` | N/A | `extra_nonce` | ⚠️ |
| ExtraNonce2Size | `extraNonce2Size` | N/A | N/A | ⚠️ |

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
- tosminer → tos-pool: ✅ Consistent
- tos-pool → tos daemon: ⚠️ Requires block_template reconstruction

---

## 5. Issues and Recommendations

### 5.1 Identified Consistency Issues

#### 5.1.1 Critical Issues (Requires Adapter Layer)

| Issue | Description | Impact | Recommendation |
|-------|-------------|--------|----------------|
| **RPC Method Name Mismatch** | tos-pool uses `tos_getWork`, daemon uses `get_block_template` | Cannot communicate directly | Add adapter layer in tos-pool |
| **Response Structure Difference** | daemon returns Rust struct, pool expects Ethereum style | Parsing failure | Implement response converter |
| **Missing Methods** | `tos_estimateGas`, `tos_gasPrice`, `tos_syncing` | Some features unavailable | daemon adds compatible methods or pool removes dependency |

#### 5.1.2 Medium Issues (Needs Attention)

| Issue | Description | Recommendation |
|-------|-------------|----------------|
| **Height vs TopoHeight** | TOS uses DAG with both height and topoheight concepts | Use topoheight consistently |
| **Asset Parameter** | `get_balance` must specify asset | pool defaults to native token hash |
| **Tips vs ParentHash** | DAG structure has multiple parent blocks | Use tips[0] as primary parent |

#### 5.1.3 Minor Issues (Can Ignore)

| Issue | Description |
|-------|-------------|
| hex prefix | Ethereum style uses "0x" prefix, TOS may not |
| Case sensitivity | Method name case differences (camelCase vs snake_case) |

### 5.2 Recommended Solutions

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

### 5.3 Priority Ranking

| Priority | Issue | Action |
|----------|-------|--------|
| **P0 - Critical** | RPC method name mismatch | Implement adapter layer |
| **P0 - Critical** | Response structure difference | Implement response converter |
| **P1 - High** | Missing RPC methods | Evaluate if necessary |
| **P2 - Medium** | DAG related fields | Documentation |
| **P3 - Low** | Format differences (hex, case) | Gradual fix |

---

## 6. Consistency Matrix

### 6.1 Stratum Protocol Consistency (tosminer ↔ tos-pool)

| Feature | Status | Notes |
|---------|--------|-------|
| mining.subscribe | ✅ Fully consistent | |
| mining.authorize | ✅ Fully consistent | |
| mining.submit | ✅ Compatible | Supports 4/5 parameter formats |
| mining.notify | ✅ Compatible | TOS simplified format + standard format fallback |
| mining.set_difficulty | ✅ Fully consistent | |
| mining.ping | ✅ Fully consistent | |
| Connection management | ✅ Consistent | TLS support, reconnection mechanism |

**Overall Assessment:** ✅ **Highly Consistent** (95%+)

### 6.2 RPC Protocol Consistency (tos-pool ↔ tos daemon)

| Feature | Status | Notes |
|---------|--------|-------|
| Get work | ⚠️ Needs adaptation | Method name and response structure differ |
| Submit block | ⚠️ Needs adaptation | Parameter structure differs |
| Get block info | ⚠️ Needs adaptation | DAG characteristic differences |
| Get balance | ⚠️ Needs adaptation | Requires asset parameter |
| Get nonce | ✅ Semantically consistent | |
| Submit transaction | ✅ Semantically consistent | |
| Gas related | ❌ Not supported | TOS has no Gas concept |
| Sync status | ❌ Not supported | Needs to be added |

**Overall Assessment:** ⚠️ **Requires Adapter Layer** (60%)

### 6.3 Data Structure Consistency

| Structure | tosminer ↔ tos-pool | tos-pool ↔ tos daemon |
|-----------|---------------------|----------------------|
| WorkPackage | ✅ Consistent | ⚠️ Needs conversion |
| Share | ✅ Consistent | N/A |
| Block | N/A | ⚠️ DAG differences |
| Transaction | N/A | ⚠️ Field differences |

---

## 7. Conclusion

### 7.1 Overall Consistency Assessment

| Communication Link | Consistency Score | Status |
|--------------------|-------------------|--------|
| tosminer ↔ tos-pool | **95%** | ✅ Production Ready |
| tos-pool ↔ tos daemon | **60%** | ⚠️ Requires Adapter Layer |

### 7.2 Next Steps

1. **Immediate Action**: Implement TOS daemon adapter layer in tos-pool's `internal/rpc/` directory
2. **Short-term Goal**: Improve TOS simplified format support for Stratum protocol
3. **Medium-term Goal**: Evaluate whether to add compatible API in daemon
4. **Long-term Goal**: Unify data structure definitions across all three components; consider using shared protobuf/flatbuffers definitions

### 7.3 Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Adapter layer performance overhead | Medium | Implement efficient data conversion, use caching |
| API change compatibility | High | Version APIs, maintain backward compatibility |
| DAG characteristic misunderstanding | Medium | Thorough testing, especially fork scenarios |

---

*Document generated: 2025-12-18*
*Applicable versions: tos-pool v1.0, tosminer v1.0, tos daemon (latest)*
