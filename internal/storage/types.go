// Package storage provides data persistence for TOS Pool.
package storage

import "time"

// Share represents a submitted mining share
type Share struct {
	Address    string  `json:"address"`
	Worker     string  `json:"worker"`
	JobID      string  `json:"job_id"`
	Nonce      string  `json:"nonce"`
	Hash       string  `json:"hash"`
	Difficulty uint64  `json:"difficulty"`
	Height     uint64  `json:"height"`
	Timestamp  int64   `json:"timestamp"`
	Valid      bool    `json:"valid"`
	BlockHash  string  `json:"block_hash,omitempty"`
}

// Block represents a found block
type Block struct {
	Height       uint64            `json:"height"`
	Hash         string            `json:"hash"`
	ParentHash   string            `json:"parent_hash"`
	Nonce        string            `json:"nonce"`
	Difficulty   uint64            `json:"difficulty"`
	TotalDiff    string            `json:"total_difficulty"`
	Reward       uint64            `json:"reward"`
	Timestamp    int64             `json:"timestamp"`
	Finder       string            `json:"finder"`
	Worker       string            `json:"worker"`
	RoundShares  uint64            `json:"round_shares"`
	RoundHeight  uint64            `json:"round_height"`
	Status       BlockStatus       `json:"status"`
	Confirmations uint64           `json:"confirmations"`
	Shares       map[string]uint64 `json:"shares,omitempty"` // address -> share count
}

// BlockStatus represents block maturation status
type BlockStatus string

const (
	BlockStatusCandidate BlockStatus = "candidate"
	BlockStatusImmature  BlockStatus = "immature"
	BlockStatusMatured   BlockStatus = "matured"
	BlockStatusOrphan    BlockStatus = "orphan"
)

// Miner represents a miner's account
type Miner struct {
	Address        string            `json:"address"`
	Balance        uint64            `json:"balance"`
	ImmatureBalance uint64           `json:"immature"`
	PendingBalance uint64            `json:"pending"`
	TotalPaid      uint64            `json:"paid"`
	BlocksFound    uint64            `json:"blocks_found"`
	LastShare      int64             `json:"last_share"`
	Workers        map[string]Worker `json:"workers,omitempty"`
}

// Worker represents a mining worker
type Worker struct {
	Name      string  `json:"name"`
	Hashrate  float64 `json:"hashrate"`
	LastSeen  int64   `json:"last_seen"`
	Accepted  uint64  `json:"accepted"`
	Rejected  uint64  `json:"rejected"`
	Stale     uint64  `json:"stale"`
}

// Payment represents a payout transaction
type Payment struct {
	TxHash    string `json:"tx_hash"`
	Address   string `json:"address"`
	Amount    uint64 `json:"amount"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"` // pending, confirmed, failed
}

// PoolStats represents pool-wide statistics
type PoolStats struct {
	Hashrate       float64 `json:"hashrate"`
	HashrateLarge  float64 `json:"hashrate_large"`
	Miners         int64   `json:"miners"`
	Workers        int64   `json:"workers"`
	RoundShares    uint64  `json:"round_shares"`
	LastBlockFound int64   `json:"last_block_found"`
	LastBlockHeight uint64 `json:"last_block_height"`
	BlocksFound    uint64  `json:"blocks_found"`
	TotalPaid      uint64  `json:"total_paid"`
}

// NetworkStats represents blockchain network statistics
type NetworkStats struct {
	Height     uint64  `json:"height"`
	Difficulty uint64  `json:"difficulty"`
	Hashrate   float64 `json:"hashrate"`
	LastBeat   int64   `json:"last_beat"`
}

// HashrateEntry represents a hashrate sample
type HashrateEntry struct {
	Difficulty uint64 `json:"diff"`
	Address    string `json:"addr"`
	Worker     string `json:"worker"`
	Timestamp  int64  `json:"ts"`
}

// MinerStats holds computed statistics for a miner
type MinerStats struct {
	Address         string    `json:"address"`
	Hashrate        float64   `json:"hashrate"`
	HashrateLarge   float64   `json:"hashrate_large"`
	SharesValid     uint64    `json:"shares_valid"`
	SharesInvalid   uint64    `json:"shares_invalid"`
	SharesStale     uint64    `json:"shares_stale"`
	Balance         uint64    `json:"balance"`
	ImmatureBalance uint64    `json:"immature"`
	PendingBalance  uint64    `json:"pending"`
	TotalPaid       uint64    `json:"paid"`
	BlocksFound     uint64    `json:"blocks_found"`
	LastShare       time.Time `json:"last_share"`
	Workers         []Worker  `json:"workers"`
}
