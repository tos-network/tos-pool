package storage

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func setupTestRedis(t *testing.T) (*RedisClient, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	client, err := NewRedisClient(mr.Addr(), "", 0)
	if err != nil {
		mr.Close()
		t.Fatalf("Failed to create Redis client: %v", err)
	}

	return client, mr
}

func TestNewRedisClient(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client, err := NewRedisClient(mr.Addr(), "", 0)
	if err != nil {
		t.Fatalf("NewRedisClient() error = %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("NewRedisClient returned nil")
	}
}

func TestNewRedisClientInvalid(t *testing.T) {
	_, err := NewRedisClient("invalid:9999", "", 0)
	if err == nil {
		t.Error("NewRedisClient should return error for invalid address")
	}
}

func TestWriteShare(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	share := &Share{
		Address:    "tos1testaddress",
		Worker:     "rig1",
		JobID:      "job123",
		Nonce:      "0x12345678",
		Difficulty: 1000000,
		Height:     12345,
		Timestamp:  time.Now().Unix(),
		Valid:      true,
	}

	err := client.WriteShare(share, 10*time.Minute)
	if err != nil {
		t.Fatalf("WriteShare() error = %v", err)
	}
}

func TestWriteAndGetMiner(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write a share first to create miner data
	share := &Share{
		Address:    "tos1testaddress",
		Worker:     "rig1",
		Difficulty: 1000000,
	}
	client.WriteShare(share, 10*time.Minute)

	// Get miner
	miner, err := client.GetMiner("tos1testaddress")
	if err != nil {
		t.Fatalf("GetMiner() error = %v", err)
	}

	if miner == nil {
		t.Fatal("GetMiner returned nil")
	}

	if miner.Address != "tos1testaddress" {
		t.Errorf("Miner.Address = %s, want tos1testaddress", miner.Address)
	}
}

func TestGetMinerNotFound(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	miner, err := client.GetMiner("nonexistent")
	if err != nil {
		t.Fatalf("GetMiner() error = %v", err)
	}

	if miner != nil {
		t.Error("GetMiner should return nil for non-existent miner")
	}
}

func TestWriteBlock(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write some shares first
	share := &Share{
		Address:    "tos1finder",
		Worker:     "rig1",
		Difficulty: 1000000,
	}
	client.WriteShare(share, 10*time.Minute)

	block := &Block{
		Height:     12345,
		Hash:       "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		Finder:     "tos1finder",
		Worker:     "rig1",
		Reward:     5000000000,
		Timestamp:  time.Now().Unix(),
		Difficulty: 1000000,
		Status:     BlockStatusCandidate,
	}

	err := client.WriteBlock(block)
	if err != nil {
		t.Fatalf("WriteBlock() error = %v", err)
	}
}

func TestGetCandidateBlocks(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write a block
	share := &Share{Address: "tos1finder", Difficulty: 1000000}
	client.WriteShare(share, 10*time.Minute)

	block := &Block{
		Height:     12345,
		Hash:       "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		Finder:     "tos1finder",
		Reward:     5000000000,
		Timestamp:  time.Now().Unix(),
		Difficulty: 1000000,
		Status:     BlockStatusCandidate,
	}
	client.WriteBlock(block)

	// Get candidates
	blocks, err := client.GetCandidateBlocks()
	if err != nil {
		t.Fatalf("GetCandidateBlocks() error = %v", err)
	}

	if len(blocks) != 1 {
		t.Errorf("GetCandidateBlocks() returned %d blocks, want 1", len(blocks))
	}
}

func TestMoveBlockToImmature(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write shares and block
	share := &Share{Address: "tos1finder", Difficulty: 1000000}
	client.WriteShare(share, 10*time.Minute)

	block := &Block{
		Height:      12345,
		Hash:        "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		Finder:      "tos1finder",
		Reward:      5000000000,
		Timestamp:   time.Now().Unix(),
		Difficulty:  1000000,
		Status:      BlockStatusCandidate,
		RoundShares: 1000000,
		Shares:      map[string]uint64{"tos1finder": 1000000},
	}
	client.WriteBlock(block)

	// Move to immature
	err := client.MoveBlockToImmature(block)
	if err != nil {
		t.Fatalf("MoveBlockToImmature() error = %v", err)
	}

	// Verify candidates is empty
	candidates, _ := client.GetCandidateBlocks()
	if len(candidates) != 0 {
		t.Error("Candidates should be empty after move")
	}
}

func TestMoveBlockToMatured(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Create block with shares
	block := &Block{
		Height:      12345,
		Hash:        "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		Finder:      "tos1finder",
		Reward:      5000000000,
		Timestamp:   time.Now().Unix(),
		Difficulty:  1000000,
		Status:      BlockStatusImmature,
		RoundShares: 1000000,
		Shares:      map[string]uint64{"tos1finder": 1000000},
	}

	// Move to immature first
	client.MoveBlockToImmature(block)

	// Move to matured
	err := client.MoveBlockToMatured(block)
	if err != nil {
		t.Fatalf("MoveBlockToMatured() error = %v", err)
	}
}

func TestRemoveOrphanBlock(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write shares and block
	share := &Share{Address: "tos1finder", Difficulty: 1000000}
	client.WriteShare(share, 10*time.Minute)

	block := &Block{
		Height:      12345,
		Hash:        "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		Finder:      "tos1finder",
		Reward:      5000000000,
		Timestamp:   time.Now().Unix(),
		Difficulty:  1000000,
		Status:      BlockStatusCandidate,
		RoundShares: 1000000,
		Shares:      map[string]uint64{"tos1finder": 1000000},
	}
	client.WriteBlock(block)

	// Remove orphan
	err := client.RemoveOrphanBlock(block)
	if err != nil {
		t.Fatalf("RemoveOrphanBlock() error = %v", err)
	}
}

func TestGetHashrate(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write some shares
	for i := 0; i < 10; i++ {
		share := &Share{
			Address:    "tos1testaddress",
			Worker:     "rig1",
			Difficulty: 1000000,
		}
		client.WriteShare(share, 10*time.Minute)
	}

	hashrate, err := client.GetHashrate(10 * time.Minute)
	if err != nil {
		t.Fatalf("GetHashrate() error = %v", err)
	}

	if hashrate <= 0 {
		t.Error("GetHashrate() should return positive value after shares")
	}
}

func TestGetMinerHashrate(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write some shares
	for i := 0; i < 5; i++ {
		share := &Share{
			Address:    "tos1testaddress",
			Worker:     "rig1",
			Difficulty: 1000000,
		}
		client.WriteShare(share, 10*time.Minute)
	}

	hashrate, err := client.GetMinerHashrate("tos1testaddress", 10*time.Minute)
	if err != nil {
		t.Fatalf("GetMinerHashrate() error = %v", err)
	}

	if hashrate <= 0 {
		t.Error("GetMinerHashrate() should return positive value after shares")
	}
}

func TestPurgeStaleHashrate(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write a share
	share := &Share{
		Address:    "tos1testaddress",
		Worker:     "rig1",
		Difficulty: 1000000,
	}
	client.WriteShare(share, 10*time.Minute)

	// Purge (with current time, nothing should be purged)
	err := client.PurgeStaleHashrate(10 * time.Minute)
	if err != nil {
		t.Fatalf("PurgeStaleHashrate() error = %v", err)
	}
}

func TestSetAndGetNetworkStats(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	stats := &NetworkStats{
		Height:     12345,
		Difficulty: 1000000,
		Hashrate:   5000000.5,
		LastBeat:   time.Now().Unix(),
	}

	err := client.SetNetworkStats(stats)
	if err != nil {
		t.Fatalf("SetNetworkStats() error = %v", err)
	}

	got, err := client.GetNetworkStats()
	if err != nil {
		t.Fatalf("GetNetworkStats() error = %v", err)
	}

	if got.Height != stats.Height {
		t.Errorf("NetworkStats.Height = %d, want %d", got.Height, stats.Height)
	}

	if got.Difficulty != stats.Difficulty {
		t.Errorf("NetworkStats.Difficulty = %d, want %d", got.Difficulty, stats.Difficulty)
	}
}

func TestBlacklist(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	address := "tos1blacklisted"

	// Initially not blacklisted
	blacklisted, err := client.IsBlacklisted(address)
	if err != nil {
		t.Fatalf("IsBlacklisted() error = %v", err)
	}
	if blacklisted {
		t.Error("Address should not be blacklisted initially")
	}

	// Add to blacklist
	err = client.AddToBlacklist(address)
	if err != nil {
		t.Fatalf("AddToBlacklist() error = %v", err)
	}

	// Check blacklisted
	blacklisted, err = client.IsBlacklisted(address)
	if err != nil {
		t.Fatalf("IsBlacklisted() error = %v", err)
	}
	if !blacklisted {
		t.Error("Address should be blacklisted")
	}

	// Get blacklist
	list, err := client.GetBlacklist()
	if err != nil {
		t.Fatalf("GetBlacklist() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("GetBlacklist() returned %d items, want 1", len(list))
	}

	// Remove from blacklist
	err = client.RemoveFromBlacklist(address)
	if err != nil {
		t.Fatalf("RemoveFromBlacklist() error = %v", err)
	}

	blacklisted, _ = client.IsBlacklisted(address)
	if blacklisted {
		t.Error("Address should not be blacklisted after removal")
	}
}

func TestWhitelist(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ip := "192.168.1.100"

	// Initially not whitelisted
	whitelisted, err := client.IsWhitelisted(ip)
	if err != nil {
		t.Fatalf("IsWhitelisted() error = %v", err)
	}
	if whitelisted {
		t.Error("IP should not be whitelisted initially")
	}

	// Add to whitelist
	err = client.AddToWhitelist(ip)
	if err != nil {
		t.Fatalf("AddToWhitelist() error = %v", err)
	}

	// Check whitelisted
	whitelisted, err = client.IsWhitelisted(ip)
	if err != nil {
		t.Fatalf("IsWhitelisted() error = %v", err)
	}
	if !whitelisted {
		t.Error("IP should be whitelisted")
	}

	// Get whitelist
	list, err := client.GetWhitelist()
	if err != nil {
		t.Fatalf("GetWhitelist() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("GetWhitelist() returned %d items, want 1", len(list))
	}

	// Remove from whitelist
	err = client.RemoveFromWhitelist(ip)
	if err != nil {
		t.Fatalf("RemoveFromWhitelist() error = %v", err)
	}

	whitelisted, _ = client.IsWhitelisted(ip)
	if whitelisted {
		t.Error("IP should not be whitelisted after removal")
	}
}

func TestPayoutLock(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	lockID := "lock123"

	// Initially not locked
	locked, err := client.IsPayoutsLocked()
	if err != nil {
		t.Fatalf("IsPayoutsLocked() error = %v", err)
	}
	if locked {
		t.Error("Payouts should not be locked initially")
	}

	// Acquire lock
	acquired, err := client.LockPayouts(lockID, 1*time.Minute)
	if err != nil {
		t.Fatalf("LockPayouts() error = %v", err)
	}
	if !acquired {
		t.Error("Should acquire lock")
	}

	// Check locked
	locked, err = client.IsPayoutsLocked()
	if err != nil {
		t.Fatalf("IsPayoutsLocked() error = %v", err)
	}
	if !locked {
		t.Error("Payouts should be locked")
	}

	// Try to acquire again (should fail)
	acquired, _ = client.LockPayouts("another_lock", 1*time.Minute)
	if acquired {
		t.Error("Should not acquire lock when already locked")
	}

	// Release lock
	err = client.UnlockPayouts(lockID)
	if err != nil {
		t.Fatalf("UnlockPayouts() error = %v", err)
	}

	locked, _ = client.IsPayoutsLocked()
	if locked {
		t.Error("Payouts should not be locked after unlock")
	}
}

func TestGetPoolStats(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	stats, err := client.GetPoolStats(10*time.Minute, 3*time.Hour)
	if err != nil {
		t.Fatalf("GetPoolStats() error = %v", err)
	}

	if stats == nil {
		t.Fatal("GetPoolStats returned nil")
	}
}

func TestGetRecentBlocks(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Initially empty
	blocks, err := client.GetRecentBlocks(10)
	if err != nil {
		t.Fatalf("GetRecentBlocks() error = %v", err)
	}

	// Empty list is fine
	if len(blocks) != 0 {
		t.Errorf("GetRecentBlocks() should return empty initially, got %d", len(blocks))
	}
}

func TestGetRecentPayments(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	payments, err := client.GetRecentPayments(10)
	if err != nil {
		t.Fatalf("GetRecentPayments() error = %v", err)
	}

	// Empty list is fine initially
	if len(payments) != 0 {
		t.Errorf("GetRecentPayments() should return empty initially, got %d", len(payments))
	}
}

func TestGetMinerPayments(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	payments, err := client.GetMinerPayments("tos1testaddress", 10)
	if err != nil {
		t.Fatalf("GetMinerPayments() error = %v", err)
	}

	// Empty list is fine for non-existent miner
	if len(payments) != 0 {
		t.Errorf("GetMinerPayments() should return empty for non-existent miner, got %d", len(payments))
	}
}

func TestGetLuckStats(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	stats, err := client.GetLuckStats()
	if err != nil {
		t.Fatalf("GetLuckStats() error = %v", err)
	}

	if stats == nil {
		t.Fatal("GetLuckStats returned nil")
	}
}

func TestCreateBackup(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	backup, err := client.CreateBackup()
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	if backup == nil {
		t.Fatal("CreateBackup returned nil")
	}

	if backup.Timestamp == 0 {
		t.Error("Backup.Timestamp should be set")
	}

	if backup.Version != "1.0" {
		t.Errorf("Backup.Version = %s, want 1.0", backup.Version)
	}
}

func TestDebt(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	address := "tos1testaddress"

	// Add debt
	err := client.AddDebt(address, 1000000, "test debt")
	if err != nil {
		t.Fatalf("AddDebt() error = %v", err)
	}

	// Get debt
	debt, err := client.GetDebt(address)
	if err != nil {
		t.Fatalf("GetDebt() error = %v", err)
	}
	if debt != 1000000 {
		t.Errorf("GetDebt() = %d, want 1000000", debt)
	}

	// Get total debt
	total, err := client.GetTotalDebt()
	if err != nil {
		t.Fatalf("GetTotalDebt() error = %v", err)
	}
	if total != 1000000 {
		t.Errorf("GetTotalDebt() = %d, want 1000000", total)
	}

	// Get debt history
	history, err := client.GetDebtHistory(address, 10)
	if err != nil {
		t.Fatalf("GetDebtHistory() error = %v", err)
	}
	if len(history) != 1 {
		t.Errorf("GetDebtHistory() returned %d records, want 1", len(history))
	}
}

func TestSettleDebt(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	address := "tos1testaddress"

	// Add debt
	client.AddDebt(address, 1000000, "test debt")

	// Settle debt
	err := client.SettleDebt(address)
	if err != nil {
		t.Fatalf("SettleDebt() error = %v", err)
	}
}

func TestStorePoolHashrate(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	err := client.StorePoolHashrate(1500000.5)
	if err != nil {
		t.Fatalf("StorePoolHashrate() error = %v", err)
	}
}

func TestStoreMinerHashrate(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	err := client.StoreMinerHashrate("tos1testaddress", 500000.5)
	if err != nil {
		t.Fatalf("StoreMinerHashrate() error = %v", err)
	}
}

func TestStoreWorkerHashrate(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	err := client.StoreWorkerHashrate("tos1testaddress", "rig1", 250000.5)
	if err != nil {
		t.Fatalf("StoreWorkerHashrate() error = %v", err)
	}
}

func TestGetPoolHashrateHistory(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Store some data
	client.StorePoolHashrate(1500000.5)

	history, err := client.GetPoolHashrateHistory(24)
	if err != nil {
		t.Fatalf("GetPoolHashrateHistory() error = %v", err)
	}

	if history == nil {
		t.Error("GetPoolHashrateHistory should not return nil")
	}
}

func TestGetMinerHashrateHistory(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Store some data
	client.StoreMinerHashrate("tos1testaddress", 500000.5)

	history, err := client.GetMinerHashrateHistory("tos1testaddress", 24)
	if err != nil {
		t.Fatalf("GetMinerHashrateHistory() error = %v", err)
	}

	if history == nil {
		t.Error("GetMinerHashrateHistory should not return nil")
	}
}

func TestGetWorkerHashrateHistory(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Store some data
	client.StoreWorkerHashrate("tos1testaddress", "rig1", 250000.5)

	history, err := client.GetWorkerHashrateHistory("tos1testaddress", "rig1", 24)
	if err != nil {
		t.Fatalf("GetWorkerHashrateHistory() error = %v", err)
	}

	if history == nil {
		t.Error("GetWorkerHashrateHistory should not return nil")
	}
}

func TestGetMinerWorkers(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Write shares with workers
	share := &Share{
		Address:    "tos1testaddress",
		Worker:     "rig1",
		Difficulty: 1000000,
	}
	client.WriteShare(share, 10*time.Minute)

	share2 := &Share{
		Address:    "tos1testaddress",
		Worker:     "rig2",
		Difficulty: 500000,
	}
	client.WriteShare(share2, 10*time.Minute)

	workers, err := client.GetMinerWorkers("tos1testaddress", 10*time.Minute)
	if err != nil {
		t.Fatalf("GetMinerWorkers() error = %v", err)
	}

	if len(workers) != 2 {
		t.Errorf("GetMinerWorkers() returned %d workers, want 2", len(workers))
	}
}

func TestGetMinerWorkersEmpty(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	workers, err := client.GetMinerWorkers("nonexistent", 10*time.Minute)
	if err != nil {
		t.Fatalf("GetMinerWorkers() error = %v", err)
	}

	if len(workers) != 0 {
		t.Errorf("GetMinerWorkers() should return empty for non-existent miner")
	}
}

// Test struct definitions
func TestShareStruct(t *testing.T) {
	share := Share{
		Address:    "tos1test",
		Worker:     "rig1",
		JobID:      "job123",
		Nonce:      "0x12345678",
		Hash:       "0xhash",
		Difficulty: 1000000,
		Height:     12345,
		Timestamp:  time.Now().Unix(),
		Valid:      true,
		BlockHash:  "0xblockhash",
	}

	if share.Address != "tos1test" {
		t.Errorf("Share.Address = %s, want tos1test", share.Address)
	}
}

func TestBlockStruct(t *testing.T) {
	block := Block{
		Height:        12345,
		Hash:          "0xhash",
		ParentHash:    "0xparent",
		Nonce:         "0xnonce",
		Difficulty:    1000000,
		TotalDiff:     "100000000",
		Reward:        5000000000,
		Timestamp:     time.Now().Unix(),
		Finder:        "tos1finder",
		Worker:        "rig1",
		RoundShares:   100000,
		RoundHeight:   12340,
		Status:        BlockStatusCandidate,
		Confirmations: 10,
		Shares:        map[string]uint64{"tos1finder": 100000},
	}

	if block.Status != BlockStatusCandidate {
		t.Errorf("Block.Status = %s, want candidate", block.Status)
	}
}

func TestMinerStruct(t *testing.T) {
	miner := Miner{
		Address:         "tos1test",
		Balance:         1000000000,
		ImmatureBalance: 500000000,
		PendingBalance:  100000000,
		TotalPaid:       5000000000,
		BlocksFound:     5,
		LastShare:       time.Now().Unix(),
	}

	if miner.Balance != 1000000000 {
		t.Errorf("Miner.Balance = %d, want 1000000000", miner.Balance)
	}
}

func TestPaymentStruct(t *testing.T) {
	payment := Payment{
		TxHash:    "0xtxhash",
		Address:   "tos1test",
		Amount:    1000000000,
		Timestamp: time.Now().Unix(),
		Status:    "confirmed",
	}

	if payment.Status != "confirmed" {
		t.Errorf("Payment.Status = %s, want confirmed", payment.Status)
	}
}

func TestPoolStatsStruct(t *testing.T) {
	stats := PoolStats{
		Hashrate:        1500000.5,
		HashrateLarge:   1400000.5,
		Miners:          100,
		Workers:         250,
		RoundShares:     1000000,
		LastBlockFound:  time.Now().Unix(),
		LastBlockHeight: 12345,
		BlocksFound:     50,
		TotalPaid:       100000000000,
	}

	if stats.Miners != 100 {
		t.Errorf("PoolStats.Miners = %d, want 100", stats.Miners)
	}
}

func TestNetworkStatsStruct(t *testing.T) {
	stats := NetworkStats{
		Height:     12345,
		Difficulty: 1000000,
		Hashrate:   5000000.5,
		LastBeat:   time.Now().Unix(),
	}

	if stats.Height != 12345 {
		t.Errorf("NetworkStats.Height = %d, want 12345", stats.Height)
	}
}

func TestBlockStatusConstants(t *testing.T) {
	if BlockStatusCandidate != "candidate" {
		t.Error("BlockStatusCandidate should be 'candidate'")
	}
	if BlockStatusImmature != "immature" {
		t.Error("BlockStatusImmature should be 'immature'")
	}
	if BlockStatusMatured != "matured" {
		t.Error("BlockStatusMatured should be 'matured'")
	}
	if BlockStatusOrphan != "orphan" {
		t.Error("BlockStatusOrphan should be 'orphan'")
	}
}
