// Package master implements the pool coordinator.
package master

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/rpc"
	"github.com/tos-network/tos-pool/internal/storage"
	"github.com/tos-network/tos-pool/internal/toshash"
	"github.com/tos-network/tos-pool/internal/util"
)

// Payment safety constants
const (
	PayoutLockTTL     = 10 * time.Minute // Max time for payout lock
	TxConfirmTimeout  = 5 * time.Minute  // Max time to wait for TX confirmation
	TxConfirmPollRate = 5 * time.Second  // How often to check for confirmation
	MinPeersForPayout = 3                // Minimum peers required for payout
)

// Master is the pool coordinator
type Master struct {
	cfg     *config.Config
	redis   *storage.RedisClient
	node    *rpc.TOSClient

	// Current state
	currentHeight  uint64
	currentDiff    uint64
	lastBlockTime  time.Time

	// Job management
	currentJob     *Job
	jobMu          sync.RWMutex
	jobUpdateChan  chan struct{}

	// Share processing
	shareChan      chan *ShareSubmission

	// Block unlocker
	unlockerTicker *time.Ticker

	// Payment processor
	payoutTicker   *time.Ticker

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Job represents a mining job
type Job struct {
	ID         string
	Height     uint64
	HeaderHash []byte
	ParentHash []byte
	Target     []byte
	Difficulty uint64
	Timestamp  uint64
	CreatedAt  time.Time
}

// ShareSubmission represents a share from a miner
type ShareSubmission struct {
	Address    string
	Worker     string
	JobID      string
	Nonce      string
	Difficulty uint64
	Height     uint64
	ResultChan chan *ShareResult
}

// ShareResult is the result of share validation
type ShareResult struct {
	Valid   bool
	Block   bool
	Message string
}

// NewMaster creates a new pool master
func NewMaster(cfg *config.Config, redis *storage.RedisClient, node *rpc.TOSClient) *Master {
	ctx, cancel := context.WithCancel(context.Background())
	return &Master{
		cfg:           cfg,
		redis:         redis,
		node:          node,
		shareChan:     make(chan *ShareSubmission, 10000),
		jobUpdateChan: make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// GetJobUpdateChan returns a channel that signals when a new job is available
func (m *Master) GetJobUpdateChan() <-chan struct{} {
	return m.jobUpdateChan
}

// Start begins the master coordinator
func (m *Master) Start() error {
	util.Info("Starting pool master...")

	// Initial job fetch
	if err := m.refreshJob(); err != nil {
		return err
	}

	// Start job refresher
	m.wg.Add(1)
	go m.jobRefreshLoop()

	// Start share processor
	m.wg.Add(1)
	go m.shareProcessLoop()

	// Start block unlocker
	if m.cfg.Unlocker.Enabled {
		m.wg.Add(1)
		go m.unlockerLoop()
	}

	// Start payment processor
	if m.cfg.Payouts.Enabled {
		m.wg.Add(1)
		go m.payoutLoop()
	}

	// Start stats updater
	m.wg.Add(1)
	go m.statsUpdateLoop()

	util.Info("Pool master started")
	return nil
}

// Stop shuts down the master
func (m *Master) Stop() {
	util.Info("Stopping pool master...")
	m.cancel()
	m.wg.Wait()
	util.Info("Pool master stopped")
}

// jobRefreshLoop periodically fetches new jobs
func (m *Master) jobRefreshLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cfg.Mining.JobRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if err := m.refreshJob(); err != nil {
				util.Warnf("Job refresh failed: %v", err)
			}
		}
	}
}

// refreshJob fetches a new job from the node
func (m *Master) refreshJob() error {
	template, err := m.node.GetWork(m.ctx)
	if err != nil {
		return err
	}

	// Check if height changed
	if template.Height == m.currentHeight && m.currentJob != nil {
		return nil
	}

	// Parse target
	target, err := util.HexToBytes(template.Target)
	if err != nil {
		return err
	}

	// Parse header hash
	headerHash, err := util.HexToBytes(template.HeaderHash)
	if err != nil {
		return err
	}

	// Create new job
	job := &Job{
		ID:         util.BytesToHexNoPre(headerHash[:8]),
		Height:     template.Height,
		HeaderHash: headerHash,
		Target:     target,
		Difficulty: template.Difficulty,
		Timestamp:  template.Timestamp,
		CreatedAt:  time.Now(),
	}

	m.jobMu.Lock()
	m.currentJob = job
	m.currentHeight = template.Height
	m.currentDiff = template.Difficulty
	m.jobMu.Unlock()

	// Signal new job available
	select {
	case m.jobUpdateChan <- struct{}{}:
	default:
		// Channel full, skip
	}

	util.Debugf("New job %s at height %d, diff %d", job.ID, job.Height, job.Difficulty)

	return nil
}

// GetCurrentJob returns the current mining job
func (m *Master) GetCurrentJob() *Job {
	m.jobMu.RLock()
	defer m.jobMu.RUnlock()
	return m.currentJob
}

// SubmitShare queues a share for validation
func (m *Master) SubmitShare(share *ShareSubmission) *ShareResult {
	share.ResultChan = make(chan *ShareResult, 1)

	select {
	case m.shareChan <- share:
		return <-share.ResultChan
	case <-m.ctx.Done():
		return &ShareResult{Valid: false, Message: "Pool shutting down"}
	}
}

// shareProcessLoop handles share validation
func (m *Master) shareProcessLoop() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case share := <-m.shareChan:
			result := m.processShare(share)
			share.ResultChan <- result
		}
	}
}

// processShare validates a submitted share
func (m *Master) processShare(share *ShareSubmission) *ShareResult {
	m.jobMu.RLock()
	job := m.currentJob
	m.jobMu.RUnlock()

	if job == nil {
		return &ShareResult{Valid: false, Message: "No active job"}
	}

	// Check job ID
	if share.JobID != job.ID {
		return &ShareResult{Valid: false, Message: "Stale job"}
	}

	// Parse nonce
	nonce, err := util.HexToBytes(share.Nonce)
	if err != nil {
		return &ShareResult{Valid: false, Message: "Invalid nonce"}
	}

	// Build header with nonce
	header := make([]byte, toshash.InputSize)
	copy(header, job.HeaderHash)
	copy(header[72:80], nonce)

	// Compute hash
	hash := toshash.Hash(header)
	if hash == nil {
		return &ShareResult{Valid: false, Message: "Hash computation failed"}
	}

	// Check share difficulty
	actualDiff := toshash.HashToDifficulty(hash)
	if actualDiff < share.Difficulty {
		return &ShareResult{Valid: false, Message: "Low difficulty share"}
	}

	// Store share
	dbShare := &storage.Share{
		Address:    share.Address,
		Worker:     share.Worker,
		JobID:      share.JobID,
		Nonce:      share.Nonce,
		Hash:       util.BytesToHex(hash),
		Difficulty: share.Difficulty,
		Height:     job.Height,
		Timestamp:  time.Now().Unix(),
		Valid:      true,
	}

	if err := m.redis.WriteShare(dbShare, m.cfg.Validation.HashrateWindow); err != nil {
		util.Warnf("Failed to store share: %v", err)
	}

	// Check if block found
	if actualDiff >= job.Difficulty {
		util.Infof("BLOCK FOUND! Height: %d, Hash: %s, Finder: %s",
			job.Height, util.BytesToHex(hash), share.Address)

		// Submit to node
		success, err := m.node.SubmitWork(m.ctx, share.Nonce, util.BytesToHex(job.HeaderHash), util.BytesToHex(hash))
		if err != nil {
			util.Errorf("Block submission failed: %v", err)
		} else if success {
			// Store block
			block := &storage.Block{
				Height:     job.Height,
				Hash:       util.BytesToHex(hash),
				Nonce:      share.Nonce,
				Difficulty: job.Difficulty,
				Finder:     share.Address,
				Worker:     share.Worker,
				Timestamp:  time.Now().Unix(),
				Status:     storage.BlockStatusCandidate,
			}

			if err := m.redis.WriteBlock(block); err != nil {
				util.Errorf("Failed to store block: %v", err)
			}

			m.lastBlockTime = time.Now()
		}

		return &ShareResult{Valid: true, Block: true, Message: "Block found!"}
	}

	return &ShareResult{Valid: true, Block: false, Message: "Share accepted"}
}

// unlockerLoop processes block maturation
func (m *Master) unlockerLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cfg.Unlocker.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.processBlocks()
		}
	}
}

// processBlocks handles block maturation
func (m *Master) processBlocks() {
	// Get current height
	netInfo, err := m.node.GetNetworkInfo(m.ctx)
	if err != nil {
		util.Warnf("Failed to get network info: %v", err)
		return
	}

	currentHeight := netInfo.Height

	// Get candidate blocks
	candidates, err := m.redis.GetCandidateBlocks()
	if err != nil {
		util.Warnf("Failed to get candidate blocks: %v", err)
		return
	}

	for _, block := range candidates {
		confirmations := currentHeight - block.Height

		// Check if block is still in main chain
		nodeBlock, err := m.node.GetBlockByNumber(m.ctx, block.Height)
		if err != nil {
			continue
		}

		if nodeBlock == nil || nodeBlock.Hash != block.Hash {
			// Orphan block
			util.Warnf("Block %d orphaned: %s", block.Height, block.Hash)
			m.redis.RemoveOrphanBlock(block)
			continue
		}

		// Update reward from node
		block.Reward = nodeBlock.Reward

		if confirmations >= m.cfg.Unlocker.MatureDepth {
			// Move to matured
			util.Infof("Block %d matured with %d confirmations", block.Height, confirmations)
			m.redis.MoveBlockToMatured(block)
		} else if confirmations >= m.cfg.Unlocker.ImmatureDepth {
			// Move to immature
			m.redis.MoveBlockToImmature(block)
		}
	}
}

// payoutLoop processes payments
func (m *Master) payoutLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cfg.Payouts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.processPayouts()
		}
	}
}

// processPayouts sends payments to miners
func (m *Master) processPayouts() {
	// 1. Check node connectivity and peer count
	netInfo, err := m.node.GetNetworkInfo(m.ctx)
	if err != nil {
		util.Warnf("Payout skipped: cannot get network info: %v", err)
		return
	}

	if netInfo.PeerCount < MinPeersForPayout {
		util.Warnf("Payout skipped: insufficient peers (%d < %d)", netInfo.PeerCount, MinPeersForPayout)
		return
	}

	// 2. Acquire payout lock to prevent double payments
	lockID := fmt.Sprintf("payout-%d", time.Now().UnixNano())
	locked, err := m.redis.LockPayouts(lockID, PayoutLockTTL)
	if err != nil {
		util.Warnf("Payout skipped: failed to acquire lock: %v", err)
		return
	}
	if !locked {
		util.Debug("Payout skipped: another payout in progress")
		return
	}

	// Ensure lock is released when done
	defer func() {
		if err := m.redis.UnlockPayouts(lockID); err != nil {
			util.Warnf("Failed to release payout lock: %v", err)
		}
	}()

	// 3. Get miners with sufficient balance
	miners, err := m.redis.GetMinersWithBalance(m.cfg.Payouts.Threshold)
	if err != nil {
		util.Warnf("Failed to get miners for payout: %v", err)
		return
	}

	if len(miners) == 0 {
		return
	}

	util.Infof("Processing payouts for %d miners", len(miners))

	// 4. Process in batches
	for i := 0; i < len(miners); i += m.cfg.Payouts.MaxAddressesPerTx {
		end := i + m.cfg.Payouts.MaxAddressesPerTx
		if end > len(miners) {
			end = len(miners)
		}

		batch := miners[i:end]
		m.processBatchPayout(batch)
	}
}

// processBatchPayout sends a batch of payments
func (m *Master) processBatchPayout(miners []*storage.Miner) {
	// Calculate pool fee
	feePercent := m.cfg.Pool.Fee / 100.0

	for _, miner := range miners {
		amount := miner.Balance
		fee := uint64(float64(amount) * feePercent)
		payout := amount - fee

		// 1. Move balance to pending (pre-deduct before sending)
		if err := m.redis.MoveToPending(miner.Address, amount); err != nil {
			util.Errorf("Failed to move balance to pending for %s: %v", miner.Address, err)
			continue
		}

		util.Infof("Paying %d to %s (fee: %d)", payout, miner.Address[:16], fee)

		// 2. Send transaction
		txHash, err := m.node.SendTransaction(m.ctx, miner.Address, payout)
		if err != nil {
			util.Errorf("Failed to send payment to %s: %v", miner.Address, err)
			// Rollback: restore balance from pending
			if rbErr := m.redis.RollbackPayment(miner.Address, amount); rbErr != nil {
				util.Errorf("CRITICAL: Failed to rollback payment for %s: %v (original error: %v)",
					miner.Address, rbErr, err)
			} else {
				util.Infof("Rolled back payment for %s", miner.Address[:16])
			}
			continue
		}

		// 3. Wait for TX confirmation
		confirmed, err := m.waitForTxConfirmation(txHash)
		if err != nil || !confirmed {
			util.Errorf("TX %s not confirmed for %s: %v", txHash, miner.Address, err)
			// Rollback: restore balance from pending
			if rbErr := m.redis.RollbackPayment(miner.Address, amount); rbErr != nil {
				util.Errorf("CRITICAL: Failed to rollback payment for %s: %v (tx: %s)",
					miner.Address, rbErr, txHash)
			} else {
				util.Warnf("Rolled back unconfirmed payment for %s (tx: %s)", miner.Address[:16], txHash)
			}
			continue
		}

		// 4. TX confirmed - finalize payment record
		if err := m.redis.UpdateMinerBalance(miner.Address, amount, txHash); err != nil {
			util.Errorf("Failed to finalize payment record for %s: %v", miner.Address, err)
		} else {
			util.Infof("Payment confirmed: %d to %s (tx: %s)", payout, miner.Address[:16], txHash)
		}
	}
}

// waitForTxConfirmation waits for a transaction to be mined
func (m *Master) waitForTxConfirmation(txHash string) (bool, error) {
	deadline := time.Now().Add(TxConfirmTimeout)
	ticker := time.NewTicker(TxConfirmPollRate)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return false, m.ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return false, fmt.Errorf("timeout waiting for TX confirmation")
			}

			// Check if TX is mined
			receipt, err := m.node.GetTransactionReceipt(m.ctx, txHash)
			if err != nil {
				// TX not yet mined, continue waiting
				util.Debugf("Waiting for TX %s...", txHash[:16])
				continue
			}

			if receipt == nil {
				continue
			}

			// Check TX status (1 = success, 0 = failure)
			if receipt.Status == 1 {
				return true, nil
			}

			return false, fmt.Errorf("transaction failed with status %d", receipt.Status)
		}
	}
}

// statsUpdateLoop updates network statistics
func (m *Master) statsUpdateLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateStats()
		}
	}
}

// updateStats updates network statistics in Redis
func (m *Master) updateStats() {
	netInfo, err := m.node.GetNetworkInfo(m.ctx)
	if err != nil {
		return
	}

	stats := &storage.NetworkStats{
		Height:     netInfo.Height,
		Difficulty: netInfo.Difficulty,
		Hashrate:   float64(netInfo.Hashrate),
		LastBeat:   time.Now().Unix(),
	}

	m.redis.SetNetworkStats(stats)
}

// GetStats returns current pool statistics
func (m *Master) GetStats() (*storage.PoolStats, error) {
	return m.redis.GetPoolStats(
		m.cfg.Validation.HashrateWindow,
		m.cfg.Validation.HashrateLargeWindow,
	)
}

// GetNetworkStats returns network statistics
func (m *Master) GetNetworkStats() (*storage.NetworkStats, error) {
	return m.redis.GetNetworkStats()
}
