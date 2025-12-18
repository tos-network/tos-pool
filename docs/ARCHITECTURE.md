# TOS Mining Pool Architecture

## Overview

A mining pool aggregates hashpower from multiple miners to find blocks more consistently, then distributes rewards proportionally based on contributed shares.

## System Architecture

```
                                    ┌─────────────────┐
                                    │   TOS Node      │
                                    │  (Blockchain)   │
                                    └────────┬────────┘
                                             │ RPC
                                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          TOS Mining Pool                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │
│  │   Stratum    │    │     Job      │    │    Share     │          │
│  │   Server     │◄──►│  Dispatcher  │◄──►│  Validator   │          │
│  │  (TCP/TLS)   │    │              │    │              │          │
│  └──────┬───────┘    └──────────────┘    └──────┬───────┘          │
│         │                                        │                   │
│         │            ┌──────────────┐           │                   │
│         │            │    Block     │           │                   │
│         └───────────►│  Submitter   │◄──────────┘                   │
│                      └──────────────┘                                │
│                             │                                        │
│         ┌───────────────────┼───────────────────┐                   │
│         ▼                   ▼                   ▼                   │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │
│  │   Database   │    │   Payment    │    │  Statistics  │          │
│  │  (PostgreSQL)│    │   System     │    │   Service    │          │
│  └──────────────┘    └──────────────┘    └──────────────┘          │
│                                                                      │
├─────────────────────────────────────────────────────────────────────┤
│                          Web Services                                │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │
│  │   REST API   │    │  WebSocket   │    │   Web UI     │          │
│  │   Server     │    │   Server     │    │  (Dashboard) │          │
│  └──────────────┘    └──────────────┘    └──────────────┘          │
└─────────────────────────────────────────────────────────────────────┘
         ▲                    ▲                    ▲
         │                    │                    │
    ┌────┴────┐          ┌────┴────┐         ┌────┴────┐
    │ Miner 1 │          │ Miner 2 │         │ Miner N │
    │tosminer │          │tosminer │         │tosminer │
    └─────────┘          └─────────┘         └─────────┘
```

## Core Components

### 1. Stratum Server
- Handles miner connections (TCP/TLS)
- Implements Stratum protocol
- Manages worker sessions
- Rate limiting and DDoS protection

### 2. Job Dispatcher
- Fetches block templates from TOS node
- Creates mining jobs with unique extranonce
- Distributes jobs to connected miners
- Handles job difficulty adjustment (vardiff)

### 3. Share Validator
- Verifies submitted shares against target
- Validates TOS Hash V3 proof-of-work
- Detects duplicate/stale shares
- Records valid shares to database

### 4. Block Submitter
- Detects when share meets block target
- Submits valid blocks to TOS node
- Handles orphan block detection
- Triggers payment processing

### 5. Payment System
- Calculates miner rewards (PPLNS/PPS/PROP)
- Manages payout thresholds
- Creates and broadcasts transactions
- Tracks payment history

### 6. Database
- Stores shares, blocks, payments
- Worker statistics and history
- Pool configuration

### 7. Web Interface
- Miner dashboard
- Pool statistics
- Payment history
- API for third-party integrations

## Payment Schemes

### PPLNS (Pay Per Last N Shares)
- Rewards based on shares in last N shares window
- Discourages pool hopping
- Most common for small/medium pools

### PPS (Pay Per Share)
- Fixed payment per valid share
- Pool absorbs variance risk
- Requires larger pool reserves

### PROP (Proportional)
- Simple proportional distribution
- Vulnerable to pool hopping
- Good for trusted communities

## Difficulty Adjustment (Vardiff)

```
Target: 10-20 shares per minute per worker

Algorithm:
1. Track shares over rolling window (5 minutes)
2. Calculate actual share rate
3. If rate > 25/min: increase difficulty
4. If rate < 8/min: decrease difficulty
5. Clamp to [min_diff, max_diff] range
```

## Security Considerations

1. **Share Validation**: Always verify TOS Hash V3 on pool side
2. **DDoS Protection**: Rate limiting, connection limits
3. **Authentication**: Worker authentication, API keys
4. **TLS**: Encrypt stratum connections
5. **Database Security**: Encrypted credentials, backups
