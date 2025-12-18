package storage

import (
	"testing"
	"time"
)

func TestWorkerStatsStruct(t *testing.T) {
	stats := WorkerStats{
		Name:     "worker1",
		Hashrate: 1000000.5,
		LastSeen: time.Now().Unix(),
	}

	if stats.Name != "worker1" {
		t.Errorf("WorkerStats.Name = %s, want worker1", stats.Name)
	}

	if stats.Hashrate != 1000000.5 {
		t.Errorf("WorkerStats.Hashrate = %f, want 1000000.5", stats.Hashrate)
	}

	if stats.LastSeen == 0 {
		t.Error("WorkerStats.LastSeen should be set")
	}
}

func TestBlockStatus(t *testing.T) {
	tests := []struct {
		status   BlockStatus
		expected string
	}{
		{BlockStatusCandidate, "candidate"},
		{BlockStatusImmature, "immature"},
		{BlockStatusMatured, "matured"},
		{BlockStatusOrphan, "orphan"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("BlockStatus = %s, want %s", tt.status, tt.expected)
		}
	}
}

func TestShareStruct(t *testing.T) {
	share := &Share{
		Address:    "tos1test",
		Worker:     "rig1",
		JobID:      "job123",
		Nonce:      "0xabcdef",
		Hash:       "0x123456",
		Difficulty: 1000000,
		Height:     12345,
		Timestamp:  time.Now().Unix(),
		Valid:      true,
	}

	if share.Address != "tos1test" {
		t.Errorf("Share.Address = %s, want tos1test", share.Address)
	}

	if !share.Valid {
		t.Error("Share.Valid should be true")
	}
}

func TestBlockStruct(t *testing.T) {
	block := &Block{
		Height:      12345,
		Hash:        "0xabcdef",
		Nonce:       "0x123456",
		Difficulty:  1000000,
		Finder:      "tos1finder",
		Worker:      "rig1",
		Reward:      500000000,
		Timestamp:   time.Now().Unix(),
		Status:      BlockStatusCandidate,
		RoundShares: 1000000,
		Shares:      map[string]uint64{"tos1miner": 500000},
	}

	if block.Height != 12345 {
		t.Errorf("Block.Height = %d, want 12345", block.Height)
	}

	if block.Status != BlockStatusCandidate {
		t.Errorf("Block.Status = %s, want candidate", block.Status)
	}
}

func TestMinerStruct(t *testing.T) {
	miner := &Miner{
		Address:         "tos1miner",
		Balance:         1000000,
		ImmatureBalance: 500000,
		PendingBalance:  100000,
		TotalPaid:       5000000,
		BlocksFound:     10,
		LastShare:       time.Now().Unix(),
	}

	if miner.Address != "tos1miner" {
		t.Errorf("Miner.Address = %s, want tos1miner", miner.Address)
	}

	if miner.BlocksFound != 10 {
		t.Errorf("Miner.BlocksFound = %d, want 10", miner.BlocksFound)
	}
}

func TestPaymentStruct(t *testing.T) {
	payment := &Payment{
		TxHash:    "0xtxhash",
		Address:   "tos1addr",
		Amount:    1000000,
		Timestamp: time.Now().Unix(),
		Status:    "confirmed",
	}

	if payment.TxHash != "0xtxhash" {
		t.Errorf("Payment.TxHash = %s, want 0xtxhash", payment.TxHash)
	}

	if payment.Status != "confirmed" {
		t.Errorf("Payment.Status = %s, want confirmed", payment.Status)
	}
}

func TestNetworkStatsStruct(t *testing.T) {
	stats := &NetworkStats{
		Height:     12345,
		Difficulty: 1000000,
		Hashrate:   5000000.5,
		LastBeat:   time.Now().Unix(),
	}

	if stats.Height != 12345 {
		t.Errorf("NetworkStats.Height = %d, want 12345", stats.Height)
	}
}

func TestPoolStatsStruct(t *testing.T) {
	stats := &PoolStats{
		Hashrate:        1000000.5,
		HashrateLarge:   900000.5,
		Miners:          100,
		Workers:         150,
		BlocksFound:     50,
		RoundShares:     1000000,
		LastBlockFound:  time.Now().Unix(),
		LastBlockHeight: 12345,
		TotalPaid:       50000000,
	}

	if stats.Miners != 100 {
		t.Errorf("PoolStats.Miners = %d, want 100", stats.Miners)
	}

	if stats.Workers != 150 {
		t.Errorf("PoolStats.Workers = %d, want 150", stats.Workers)
	}
}

func TestLuckStatsStruct(t *testing.T) {
	stats := &LuckStats{
		Luck24h: 100.5,
		Luck7d:  98.2,
		Luck30d: 101.3,
		LuckAll: 99.8,
		Blocks: []BlockLuck{
			{Height: 12345, Effort: 95.5, RoundShares: 100000, Difficulty: 105000, Timestamp: time.Now().Unix()},
		},
	}

	if stats.Luck24h != 100.5 {
		t.Errorf("LuckStats.Luck24h = %f, want 100.5", stats.Luck24h)
	}

	if len(stats.Blocks) != 1 {
		t.Errorf("LuckStats.Blocks len = %d, want 1", len(stats.Blocks))
	}
}

func TestDebtRecordStruct(t *testing.T) {
	record := DebtRecord{
		Address:   "tos1addr",
		Amount:    1000,
		Reason:    "fee adjustment",
		Timestamp: time.Now().Unix(),
	}

	if record.Address != "tos1addr" {
		t.Errorf("DebtRecord.Address = %s, want tos1addr", record.Address)
	}

	if record.Reason != "fee adjustment" {
		t.Errorf("DebtRecord.Reason = %s, want 'fee adjustment'", record.Reason)
	}
}

func TestHashratePointStruct(t *testing.T) {
	point := HashratePoint{
		Timestamp: time.Now().Unix(),
		Hashrate:  1000000.5,
	}

	if point.Hashrate != 1000000.5 {
		t.Errorf("HashratePoint.Hashrate = %f, want 1000000.5", point.Hashrate)
	}
}

func TestPoolBackupStruct(t *testing.T) {
	backup := &PoolBackup{
		Timestamp:       time.Now().Unix(),
		Version:         "1.0",
		Stats:           &PoolStats{Miners: 100},
		NetworkStats:    &NetworkStats{Height: 12345},
		Miners:          map[string]*Miner{"tos1test": {Address: "tos1test", Balance: 1000}},
		CandidateBlocks: []*Block{},
		ImmatureBlocks:  []*Block{},
		MaturedBlocks:   []*Block{},
		PendingPayments: []*Payment{},
		RecentPayments:  []*Payment{},
		Blacklist:       []string{"bad1"},
		Whitelist:       []string{"good1"},
	}

	if backup.Version != "1.0" {
		t.Errorf("PoolBackup.Version = %s, want 1.0", backup.Version)
	}

	if len(backup.Blacklist) != 1 {
		t.Errorf("PoolBackup.Blacklist len = %d, want 1", len(backup.Blacklist))
	}
}

func TestKeyPatterns(t *testing.T) {
	// Test that key patterns are properly formatted
	tests := []struct {
		pattern  string
		expected string
	}{
		{keyPrefix, "tos:"},
		{keyStats, "tos:stats"},
		{keyHashrate, "tos:hashrate"},
		{keyBlocksCandidates, "tos:blocks:candidates"},
		{keyBlocksImmature, "tos:blocks:immature"},
		{keyBlocksMatured, "tos:blocks:matured"},
		{keyPaymentsPending, "tos:payments:pending"},
		{keyPaymentsAll, "tos:payments:all"},
		{keyBlacklist, "tos:blacklist"},
		{keyWhitelist, "tos:whitelist"},
	}

	for _, tt := range tests {
		if tt.pattern != tt.expected {
			t.Errorf("key pattern = %s, want %s", tt.pattern, tt.expected)
		}
	}
}
