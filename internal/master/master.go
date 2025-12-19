// Package master implements the pool coordinator.
package master

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/notify"
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
	OrphanSearchRange = 16               // Search ±16 blocks for orphan detection
	MaxJobBacklog     = 3                // Number of previous jobs to keep for stale share prevention
)

// Master is the pool coordinator
type Master struct {
	cfg      *config.Config
	redis    *storage.RedisClient
	upstream *rpc.UpstreamManager
	notifier *notify.Notifier
	wallet   *rpc.WalletClient // Wallet RPC client for payouts

	// Current state
	currentHeight  uint64
	currentDiff    uint64
	lastBlockTime  time.Time

	// Job management
	currentJob     *Job
	jobBacklog     map[string]*Job // Job ID -> Job for stale share prevention
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
	ID             string
	Height         uint64
	HeaderHash     []byte // MinerWork format (112 bytes) for miners
	OriginalHeader []byte // Original BlockHeader for block submission
	ParentHash     []byte
	Target         []byte
	Difficulty     uint64
	Timestamp      uint64
	CreatedAt      time.Time
}

// ShareSubmission represents a share from a miner
type ShareSubmission struct {
	Address        string
	Worker         string
	JobID          string
	Nonce          string
	Difficulty     uint64
	Height         uint64
	TrustScore     int  // Trust score of the submitting miner
	SkipValidation bool // If true, skip PoW validation (for trusted miners)
	ResultChan     chan *ShareResult
}

// ShareResult is the result of share validation
type ShareResult struct {
	Valid   bool
	Block   bool
	Message string
}

// NewMaster creates a new pool master
func NewMaster(cfg *config.Config, redis *storage.RedisClient, upstream *rpc.UpstreamManager) *Master {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize notifier
	notifyCfg := &notify.WebhookConfig{
		Enabled:      cfg.Notify.Enabled,
		DiscordURL:   cfg.Notify.DiscordURL,
		TelegramBot:  cfg.Notify.TelegramBot,
		TelegramChat: cfg.Notify.TelegramChat,
		PoolName:     cfg.Pool.Name,
		PoolURL:      cfg.Notify.PoolURL,
	}

	// Initialize wallet client for payouts
	var walletClient *rpc.WalletClient
	if cfg.Payouts.Enabled && cfg.Payouts.WalletRPC != "" {
		walletEndpoint := "http://" + cfg.Payouts.WalletRPC
		walletClient = rpc.NewWalletClient(walletEndpoint, cfg.Payouts.WalletUser, cfg.Payouts.WalletPassword)
		util.Infof("Wallet RPC client initialized: %s", walletEndpoint)
	}

	return &Master{
		cfg:           cfg,
		redis:         redis,
		upstream:      upstream,
		notifier:      notify.NewNotifier(notifyCfg),
		wallet:        walletClient,
		shareChan:     make(chan *ShareSubmission, 10000),
		jobBacklog:    make(map[string]*Job),
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

	// Start hashrate history recorder (every minute)
	m.wg.Add(1)
	go m.hashrateHistoryLoop()

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
	node := m.upstream.GetClient()
	if node == nil {
		return fmt.Errorf("no upstream available")
	}

	template, err := node.GetWork(m.ctx)
	if err != nil {
		m.upstream.RecordFailure()
		return err
	}
	m.upstream.RecordSuccess()

	// Check if height changed
	if template.Height == m.currentHeight && m.currentJob != nil {
		return nil
	}

	// Parse target
	target, err := util.HexToBytes(template.Target)
	if err != nil {
		return err
	}

	// Parse original block header from daemon
	originalHeader, err := util.HexToBytes(template.HeaderHash)
	if err != nil {
		return err
	}

	// Convert BlockHeader to MinerWork format for miners
	minerWork, err := toshash.BlockHeaderToMinerWork(originalHeader)
	if err != nil {
		util.Warnf("Failed to convert BlockHeader to MinerWork: %v (headerLen=%d)", err, len(originalHeader))
		return err
	}

	util.Debugf("Job conversion: BlockHeader(%d bytes) -> MinerWork(%d bytes)", len(originalHeader), len(minerWork))

	// Create new job
	job := &Job{
		ID:             util.BytesToHexNoPre(minerWork[:8]),
		Height:         template.Height,
		HeaderHash:     minerWork,        // MinerWork format for miners
		OriginalHeader: originalHeader,   // Original BlockHeader for block submission
		Target:         target,
		Difficulty:     template.Difficulty,
		Timestamp:      template.Timestamp,
		CreatedAt:      time.Now(),
	}

	m.jobMu.Lock()
	// Add current job to backlog before replacing
	if m.currentJob != nil {
		m.jobBacklog[m.currentJob.ID] = m.currentJob
	}

	m.currentJob = job
	m.currentHeight = template.Height
	m.currentDiff = template.Difficulty

	// Clean up old jobs from backlog (keep only last N at current or recent heights)
	m.pruneJobBacklog()
	m.jobMu.Unlock()

	// Signal new job available
	select {
	case m.jobUpdateChan <- struct{}{}:
	default:
		// Channel full, skip
	}

	util.Debugf("New job %s at height %d, diff %d (backlog: %d jobs)",
		job.ID, job.Height, job.Difficulty, len(m.jobBacklog))

	return nil
}

// pruneJobBacklog removes old jobs from the backlog
// Must be called with jobMu held
func (m *Master) pruneJobBacklog() {
	if len(m.jobBacklog) <= MaxJobBacklog {
		return
	}

	// Find the minimum height we should keep (current height - MaxJobBacklog)
	minHeight := m.currentHeight
	if minHeight > MaxJobBacklog {
		minHeight -= MaxJobBacklog
	} else {
		minHeight = 0
	}

	// Remove jobs older than minHeight
	for id, job := range m.jobBacklog {
		if job.Height < minHeight {
			delete(m.jobBacklog, id)
		}
	}
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
	// First check current job
	job := m.currentJob
	if job == nil {
		m.jobMu.RUnlock()
		return &ShareResult{Valid: false, Message: "No active job"}
	}

	// Check if share matches current job
	if share.JobID != job.ID {
		// Check job backlog for stale share prevention
		if backlogJob, ok := m.jobBacklog[share.JobID]; ok {
			// Share is for a recent job - accept it
			job = backlogJob
			util.Debugf("Accepting share for backlog job %s (current: %s)", share.JobID, m.currentJob.ID)
		} else {
			m.jobMu.RUnlock()
			return &ShareResult{Valid: false, Message: "Stale job"}
		}
	}
	m.jobMu.RUnlock()

	// Parse nonce
	nonce, err := util.HexToBytes(share.Nonce)
	if err != nil {
		return &ShareResult{Valid: false, Message: "Invalid nonce"}
	}

	var hash []byte
	var actualDiff uint64
	var minerWork []byte // MinerWork with nonce for block submission

	// Trust-based validation: skip expensive PoW computation for trusted miners
	// BUT always validate if share claims to meet block difficulty
	if share.SkipValidation && share.Difficulty < job.Difficulty {
		// Trusted miner, non-block share - skip PoW validation
		// Trust the claimed difficulty and accept the share
		util.Debugf("Trust-based skip: miner %s (trust=%d) share at diff %d",
			share.Address[:12], share.TrustScore, share.Difficulty)
		actualDiff = share.Difficulty
		hash = nil      // Hash not computed
		minerWork = nil // Not needed for non-block shares
	} else {
		// Full PoW validation required:
		// - New/untrusted miner, OR
		// - Trusted miner selected for random validation, OR
		// - Share claims to meet block difficulty (potential block)

		// Build MinerWork header with nonce at NonceOffset (bytes 40-47, big-endian)
		minerWork = make([]byte, toshash.InputSize)
		copy(minerWork, job.HeaderHash)
		copy(minerWork[toshash.NonceOffset:toshash.NonceOffset+8], nonce)

		// Debug: log header and nonce details
		util.Debugf("Share validation: addr=%s nonce=%s minerWorkLen=%d jobHeaderLen=%d",
			share.Address[:16], share.Nonce, len(minerWork), len(job.HeaderHash))
		util.Debugf("Nonce bytes: %x, nonce region in minerWork: %x",
			nonce, minerWork[toshash.NonceOffset:toshash.NonceOffset+8])

		// Compute hash
		hash = toshash.Hash(minerWork)
		if hash == nil {
			return &ShareResult{Valid: false, Message: "Hash computation failed"}
		}

		// Check share difficulty
		actualDiff = toshash.HashToDifficulty(hash)
		util.Debugf("Hash result: %x, actualDiff=%d, shareDiff=%d",
			hash[:16], actualDiff, share.Difficulty)
		if actualDiff < share.Difficulty {
			return &ShareResult{Valid: false, Message: "Low difficulty share"}
		}
	}

	// Determine hash string for storage
	hashStr := ""
	if hash != nil {
		hashStr = util.BytesToHex(hash)
	}

	// Store share
	dbShare := &storage.Share{
		Address:    share.Address,
		Worker:     share.Worker,
		JobID:      share.JobID,
		Nonce:      share.Nonce,
		Hash:       hashStr,
		Difficulty: share.Difficulty,
		Height:     job.Height,
		Timestamp:  time.Now().Unix(),
		Valid:      true,
	}

	if err := m.redis.WriteShare(dbShare, m.cfg.Validation.HashrateWindow); err != nil {
		util.Warnf("Failed to store share: %v", err)
	}

	// Check if block found
	if actualDiff >= job.Difficulty && hash != nil {
		util.Infof("BLOCK FOUND! Height: %d, Hash: %s, Finder: %s",
			job.Height, util.BytesToHex(hash), share.Address)

		// Submit to node with failover
		node := m.upstream.GetClient()
		if node == nil {
			util.Error("Block submission failed: no upstream available")
			return &ShareResult{Valid: true, Block: true, Message: "Block found but submission failed"}
		}

		// Submit block with original BlockHeader and MinerWork (with nonce)
		// daemon expects: block_template (BlockHeader) + miner_work (MinerWork with nonce)
		// NOTE: Use BytesToHexNoPre - Rust hex::decode doesn't accept 0x prefix
		success, err := node.SubmitWork(m.ctx,
			util.BytesToHexNoPre(minerWork),           // miner_work (MinerWork with nonce)
			util.BytesToHexNoPre(job.OriginalHeader),  // block_template (original BlockHeader)
			util.BytesToHexNoPre(hash))                // unused mixDigest
		if err != nil {
			util.Errorf("Block submission failed: %v", err)
		} else if !success {
			util.Warnf("Block submission rejected by daemon (height: %d)", job.Height)
		}

		if success {
			// Store block - use no 0x prefix to match daemon format
			block := &storage.Block{
				Height:     job.Height,
				Hash:       util.BytesToHexNoPre(hash),
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

			// Send notification
			m.notifier.NotifyBlockFound(block, m.currentDiff)

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
	node := m.upstream.GetClient()
	if node == nil {
		util.Warn("Block processing skipped: no upstream available")
		return
	}

	// Get current height
	netInfo, err := node.GetNetworkInfo(m.ctx)
	if err != nil {
		m.upstream.RecordFailure()
		util.Warnf("Failed to get network info: %v", err)
		return
	}
	m.upstream.RecordSuccess()

	currentHeight := netInfo.Height

	// Get candidate blocks
	candidates, err := m.redis.GetCandidateBlocks()
	if err != nil {
		util.Warnf("Failed to get candidate blocks: %v", err)
		return
	}

	for _, block := range candidates {
		confirmations := currentHeight - block.Height

		// TOS uses DAG structure - the PoW hash we store is different from the block ID hash
		// returned by the daemon. We verify by checking if our miner address is the block's miner.
		foundBlock, err := node.GetBlockByNumber(m.ctx, block.Height)
		if err != nil {
			util.Warnf("Error getting block %d: %v", block.Height, err)
			continue
		}

		if foundBlock == nil {
			// Block height doesn't exist yet (shouldn't happen for candidates)
			util.Warnf("Block %d not found on chain", block.Height)
			continue
		}

		// Check if the block at this height was mined by our finder
		// Note: In TOS, multiple blocks can exist at same height in DAG, but only one is canonical
		if foundBlock.Miner != block.Finder {
			// Different miner won this height - our block is orphaned
			util.Warnf("Block %d orphaned (miner mismatch: chain has %s, we have %s)",
				block.Height, foundBlock.Miner[:20], block.Finder[:20])
			m.notifier.NotifyOrphanBlock(block)
			m.redis.RemoveOrphanBlock(block)
			continue
		}

		util.Debugf("Block %d verified: miner matches %s", block.Height, block.Finder[:20])

		// Update reward from node (base reward)
		block.Reward = foundBlock.Reward

		// Calculate and add transaction fees to reward
		txFees, err := node.GetBlockTxFees(m.ctx, block.Height)
		if err == nil && txFees > 0 {
			block.Reward += txFees
			util.Debugf("Block %d: base reward %d + tx fees %d = %d",
				block.Height, foundBlock.Reward, txFees, block.Reward)
		}

		// Recalculate confirmations based on actual height
		confirmations = currentHeight - block.Height

		if confirmations >= m.cfg.Unlocker.MatureDepth {
			// Move to matured
			util.Infof("Block %d matured with %d confirmations (reward: %d)",
				block.Height, confirmations, block.Reward)
			m.redis.MoveBlockToMatured(block)
		} else if confirmations >= m.cfg.Unlocker.ImmatureDepth {
			// Move to immature
			m.redis.MoveBlockToImmature(block)
		}
	}

	// Also check immature blocks for maturation
	immatureBlocks, err := m.redis.GetImmatureBlocks()
	if err != nil {
		util.Warnf("Failed to get immature blocks: %v", err)
		return
	}

	for _, block := range immatureBlocks {
		confirmations := currentHeight - block.Height
		if confirmations >= m.cfg.Unlocker.MatureDepth {
			util.Infof("Block %d matured with %d confirmations (reward: %d)",
				block.Height, confirmations, block.Reward)
			m.redis.MoveBlockToMatured(block)
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
	node := m.upstream.GetClient()
	if node == nil {
		util.Warn("Payout skipped: no upstream available")
		return
	}

	// 1. Check node connectivity and peer count
	netInfo, err := node.GetNetworkInfo(m.ctx)
	if err != nil {
		m.upstream.RecordFailure()
		util.Warnf("Payout skipped: cannot get network info: %v", err)
		return
	}
	m.upstream.RecordSuccess()

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

// processBatchPayout sends a batch of payments using the wallet RPC
func (m *Master) processBatchPayout(miners []*storage.Miner) {
	// Check wallet client is available
	if m.wallet == nil {
		util.Warn("Batch payout skipped: wallet client not configured")
		return
	}

	// Verify wallet is online
	online, err := m.wallet.IsOnline(m.ctx)
	if err != nil {
		util.Warnf("Batch payout skipped: wallet connectivity check failed: %v", err)
		return
	}
	if !online {
		util.Warn("Batch payout skipped: wallet is offline")
		return
	}

	// Calculate pool fee
	feePercent := m.cfg.Pool.Fee / 100.0

	// Build batch transfer destinations
	var destinations []rpc.TransferDestination
	var payoutInfo []struct {
		address string
		amount  uint64
		payout  uint64
	}

	for _, miner := range miners {
		amount := miner.Balance
		fee := uint64(float64(amount) * feePercent)
		payout := amount - fee

		// Move balance to pending (pre-deduct before sending)
		if err := m.redis.MoveToPending(miner.Address, amount); err != nil {
			util.Errorf("Failed to move balance to pending for %s: %v", miner.Address, err)
			continue
		}

		destinations = append(destinations, rpc.TransferDestination{
			Address: miner.Address,
			Amount:  payout,
		})
		payoutInfo = append(payoutInfo, struct {
			address string
			amount  uint64
			payout  uint64
		}{miner.Address, amount, payout})

		util.Infof("Queued payout %d to %s (fee: %d)", payout, miner.Address[:16], fee)
	}

	if len(destinations) == 0 {
		return
	}

	// Send batch transaction via wallet RPC
	util.Infof("Sending batch payout to %d miners", len(destinations))
	txHash, err := m.wallet.BatchTransfer(m.ctx, destinations)
	if err != nil {
		util.Errorf("Batch payout failed: %v", err)
		// Rollback all pending balances
		for _, info := range payoutInfo {
			if rbErr := m.redis.RollbackPayment(info.address, info.amount); rbErr != nil {
				util.Errorf("CRITICAL: Failed to rollback payment for %s: %v", info.address, rbErr)
			} else {
				util.Infof("Rolled back payment for %s", info.address[:16])
			}
		}
		return
	}

	util.Infof("Batch payout TX submitted: %s", txHash)

	// Wait for TX confirmation
	confirmed, err := m.waitForTxConfirmation(txHash)
	if err != nil || !confirmed {
		util.Errorf("Batch TX %s not confirmed: %v", txHash, err)
		// Rollback all pending balances
		for _, info := range payoutInfo {
			if rbErr := m.redis.RollbackPayment(info.address, info.amount); rbErr != nil {
				util.Errorf("CRITICAL: Failed to rollback payment for %s: %v (tx: %s)",
					info.address, rbErr, txHash)
			} else {
				util.Warnf("Rolled back unconfirmed payment for %s (tx: %s)", info.address[:16], txHash)
			}
		}
		return
	}

	// TX confirmed - finalize all payment records
	for _, info := range payoutInfo {
		if err := m.redis.UpdateMinerBalance(info.address, info.amount, txHash); err != nil {
			util.Errorf("Failed to finalize payment record for %s: %v", info.address, err)
		} else {
			util.Infof("Payment confirmed: %d to %s (tx: %s)", info.payout, info.address[:16], txHash)
		}
	}

	util.Infof("Batch payout completed: %d miners, TX: %s", len(payoutInfo), txHash)
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
			node := m.upstream.GetClient()
			if node == nil {
				continue
			}
			receipt, err := node.GetTransactionReceipt(m.ctx, txHash)
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
	node := m.upstream.GetClient()
	if node == nil {
		return
	}

	netInfo, err := node.GetNetworkInfo(m.ctx)
	if err != nil {
		m.upstream.RecordFailure()
		return
	}
	m.upstream.RecordSuccess()

	stats := &storage.NetworkStats{
		Height:     netInfo.Height,
		Difficulty: netInfo.Difficulty,
		Hashrate:   float64(netInfo.Hashrate),
		LastBeat:   time.Now().Unix(),
	}

	m.redis.SetNetworkStats(stats)
}

// hashrateHistoryLoop stores hashrate snapshots for charting
func (m *Master) hashrateHistoryLoop() {
	defer m.wg.Done()

	// Store initial snapshot
	m.storeHashrateSnapshot()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.storeHashrateSnapshot()
		}
	}
}

// storeHashrateSnapshot stores current hashrate data for history charts
func (m *Master) storeHashrateSnapshot() {
	// Get pool stats
	poolStats, err := m.redis.GetPoolStats(m.cfg.Validation.HashrateWindow, m.cfg.Validation.HashrateLargeWindow)
	if err != nil {
		util.Warnf("Failed to get pool stats for history: %v", err)
		return
	}

	// Store pool hashrate and workers count
	if poolStats != nil {
		if err := m.redis.StorePoolHashrate(poolStats.Hashrate); err != nil {
			util.Warnf("Failed to store pool hashrate: %v", err)
		}

		if err := m.redis.StoreWorkersCount(int(poolStats.Workers)); err != nil {
			util.Warnf("Failed to store workers count: %v", err)
		}
	}
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

// GetDynamicPPLNSWindow calculates the optimal PPLNS window based on pool/network hashrate ratio
// This helps adjust rewards distribution to match the pool's actual mining power
func (m *Master) GetDynamicPPLNSWindow() float64 {
	if !m.cfg.PPLNS.DynamicWindow {
		return m.cfg.PPLNS.Window
	}

	// Get pool and network hashrates
	poolStats, err := m.redis.GetPoolStats(m.cfg.Validation.HashrateWindow, m.cfg.Validation.HashrateLargeWindow)
	if err != nil || poolStats == nil || poolStats.Hashrate == 0 {
		return m.cfg.PPLNS.Window
	}

	netStats, err := m.redis.GetNetworkStats()
	if err != nil || netStats == nil || netStats.Hashrate == 0 {
		return m.cfg.PPLNS.Window
	}

	// Calculate pool's share of network hashrate
	poolRatio := poolStats.Hashrate / netStats.Hashrate

	// Dynamic window calculation:
	// - If pool has 50% of network: window = base window
	// - If pool has 10% of network: window = base window * 5 (more shares needed)
	// - If pool has 100% of network: window = base window * 0.5 (fewer shares needed)
	//
	// Formula: window = baseWindow / (poolRatio * 2)
	// Clamped to min/max bounds

	var dynamicWindow float64
	if poolRatio > 0 {
		dynamicWindow = m.cfg.PPLNS.Window / (poolRatio * 2)
	} else {
		dynamicWindow = m.cfg.PPLNS.MaxWindow
	}

	// Clamp to configured bounds
	if dynamicWindow < m.cfg.PPLNS.MinWindow {
		dynamicWindow = m.cfg.PPLNS.MinWindow
	}
	if dynamicWindow > m.cfg.PPLNS.MaxWindow {
		dynamicWindow = m.cfg.PPLNS.MaxWindow
	}

	util.Debugf("Dynamic PPLNS: pool ratio=%.4f%%, window=%.2f", poolRatio*100, dynamicWindow)

	return dynamicWindow
}

// GetPPLNSShareWindow returns the number of shares to include in PPLNS calculation
func (m *Master) GetPPLNSShareWindow() uint64 {
	window := m.GetDynamicPPLNSWindow()

	// Window is multiplied by network difficulty to get share count
	// This means if window=2.0 and diff=1000000, we look at last 2000000 worth of shares
	return uint64(window * float64(m.currentDiff))
}

// GetUpstreamStates returns the health status of all upstream nodes
func (m *Master) GetUpstreamStates() []rpc.UpstreamState {
	return m.upstream.GetUpstreamStates()
}

// GetActiveUpstream returns the name of the currently active upstream
func (m *Master) GetActiveUpstream() string {
	return m.upstream.GetActiveUpstream()
}

// HasHealthyUpstream returns true if at least one upstream is healthy
func (m *Master) HasHealthyUpstream() bool {
	return m.upstream.HasHealthyUpstream()
}

// UpstreamCount returns the number of configured upstreams
func (m *Master) UpstreamCount() int {
	return m.upstream.UpstreamCount()
}

// HealthyUpstreamCount returns the number of healthy upstreams
func (m *Master) HealthyUpstreamCount() int {
	return m.upstream.HealthyCount()
}
