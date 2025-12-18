package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tos-network/tos-pool/internal/util"
)

const (
	keyPrefix = "tos:"

	// Key patterns
	keyNodes            = keyPrefix + "nodes"
	keyStats            = keyPrefix + "stats"
	keyFinders          = keyPrefix + "finders"
	keyHashrate         = keyPrefix + "hashrate"
	keyHashrateAddr     = keyPrefix + "hashrate:%s"
	keySharesRound      = keyPrefix + "shares:roundCurrent"
	keySharesRoundBlock = keyPrefix + "shares:round:%d:%s"
	keyBlocksCandidates = keyPrefix + "blocks:candidates"
	keyBlocksImmature   = keyPrefix + "blocks:immature"
	keyBlocksMatured    = keyPrefix + "blocks:matured"
	keyMiner            = keyPrefix + "miners:%s"
	keyMinerWorkers     = keyPrefix + "miners:%s:workers"
	keyPaymentsPending  = keyPrefix + "payments:pending"
	keyPaymentsAll      = keyPrefix + "payments:all"
	keyPaymentsAddr     = keyPrefix + "payments:%s"
	keyBlacklist        = keyPrefix + "blacklist"
	keyWhitelist        = keyPrefix + "whitelist"
)

// RedisClient wraps Redis operations for the pool
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisClient creates a new Redis client
func NewRedisClient(url, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     url,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	util.Info("Connected to Redis at ", url)
	return &RedisClient{client: client, ctx: ctx}, nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// WriteShare stores a submitted share
func (r *RedisClient) WriteShare(share *Share, hashrateWindow time.Duration) error {
	now := time.Now().Unix()
	ms := time.Now().UnixMilli()

	pipe := r.client.Pipeline()

	// Increment round shares for this address
	pipe.HIncrBy(r.ctx, keySharesRound, share.Address, int64(share.Difficulty))

	// Add to global hashrate sorted set
	// Format: "difficulty:address:worker:ms"
	member := fmt.Sprintf("%d:%s:%s:%d", share.Difficulty, share.Address, share.Worker, ms)
	pipe.ZAdd(r.ctx, keyHashrate, &redis.Z{
		Score:  float64(now),
		Member: member,
	})

	// Add to per-address hashrate
	addrKey := fmt.Sprintf(keyHashrateAddr, share.Address)
	pipe.ZAdd(r.ctx, addrKey, &redis.Z{
		Score:  float64(now),
		Member: member,
	})

	// Set expiration on per-address hashrate
	pipe.Expire(r.ctx, addrKey, hashrateWindow)

	// Update miner last share time
	minerKey := fmt.Sprintf(keyMiner, share.Address)
	pipe.HSet(r.ctx, minerKey, "lastShare", now)

	// Update worker last seen
	workerKey := fmt.Sprintf(keyMinerWorkers, share.Address)
	pipe.HSet(r.ctx, workerKey, share.Worker, now)

	_, err := pipe.Exec(r.ctx)
	return err
}

// WriteBlock stores a found block
func (r *RedisClient) WriteBlock(block *Block) error {
	now := time.Now().Unix()

	// Get current round shares
	shares, err := r.client.HGetAll(r.ctx, keySharesRound).Result()
	if err != nil {
		return err
	}

	// Store shares with the block
	block.Shares = make(map[string]uint64)
	var totalShares uint64
	for addr, count := range shares {
		c, _ := strconv.ParseUint(count, 10, 64)
		block.Shares[addr] = c
		totalShares += c
	}
	block.RoundShares = totalShares

	// Serialize block
	blockJSON, err := json.Marshal(block)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()

	// Add to candidates
	pipe.ZAdd(r.ctx, keyBlocksCandidates, &redis.Z{
		Score:  float64(block.Height),
		Member: string(blockJSON),
	})

	// Archive round shares
	roundKey := fmt.Sprintf(keySharesRoundBlock, block.Height, block.Hash[:16])
	for addr, count := range shares {
		pipe.HSet(r.ctx, roundKey, addr, count)
	}

	// Reset round shares
	pipe.Del(r.ctx, keySharesRound)

	// Increment finder's block count
	pipe.ZIncrBy(r.ctx, keyFinders, 1, block.Finder)

	// Update pool stats
	pipe.HSet(r.ctx, keyStats, "lastBlockFound", now)
	pipe.HSet(r.ctx, keyStats, "lastBlockHeight", block.Height)
	pipe.HIncrBy(r.ctx, keyStats, "blocksFound", 1)

	// Update finder's stats
	minerKey := fmt.Sprintf(keyMiner, block.Finder)
	pipe.HIncrBy(r.ctx, minerKey, "blocksFound", 1)

	_, err = pipe.Exec(r.ctx)
	return err
}

// GetCandidateBlocks returns all candidate blocks
func (r *RedisClient) GetCandidateBlocks() ([]*Block, error) {
	results, err := r.client.ZRange(r.ctx, keyBlocksCandidates, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	blocks := make([]*Block, 0, len(results))
	for _, result := range results {
		var block Block
		if err := json.Unmarshal([]byte(result), &block); err != nil {
			continue
		}
		blocks = append(blocks, &block)
	}
	return blocks, nil
}

// MoveBlockToImmature moves a block from candidates to immature
func (r *RedisClient) MoveBlockToImmature(block *Block) error {
	block.Status = BlockStatusImmature
	blockJSON, err := json.Marshal(block)
	if err != nil {
		return err
	}

	// Find and remove from candidates
	candidates, err := r.client.ZRange(r.ctx, keyBlocksCandidates, 0, -1).Result()
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()

	for _, candidate := range candidates {
		var b Block
		if err := json.Unmarshal([]byte(candidate), &b); err != nil {
			continue
		}
		if b.Hash == block.Hash {
			pipe.ZRem(r.ctx, keyBlocksCandidates, candidate)
			break
		}
	}

	// Add to immature
	pipe.ZAdd(r.ctx, keyBlocksImmature, &redis.Z{
		Score:  float64(block.Height),
		Member: string(blockJSON),
	})

	// Credit immature balance to miners
	for addr, shares := range block.Shares {
		minerKey := fmt.Sprintf(keyMiner, addr)
		reward := uint64(float64(block.Reward) * float64(shares) / float64(block.RoundShares))
		pipe.HIncrBy(r.ctx, minerKey, "immature", int64(reward))
	}

	_, err = pipe.Exec(r.ctx)
	return err
}

// MoveBlockToMatured moves a block from immature to matured
func (r *RedisClient) MoveBlockToMatured(block *Block) error {
	block.Status = BlockStatusMatured
	blockJSON, err := json.Marshal(block)
	if err != nil {
		return err
	}

	// Find and remove from immature
	immature, err := r.client.ZRange(r.ctx, keyBlocksImmature, 0, -1).Result()
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()

	for _, im := range immature {
		var b Block
		if err := json.Unmarshal([]byte(im), &b); err != nil {
			continue
		}
		if b.Hash == block.Hash {
			pipe.ZRem(r.ctx, keyBlocksImmature, im)
			break
		}
	}

	// Add to matured
	pipe.ZAdd(r.ctx, keyBlocksMatured, &redis.Z{
		Score:  float64(block.Height),
		Member: string(blockJSON),
	})

	// Move immature balance to confirmed balance
	for addr, shares := range block.Shares {
		minerKey := fmt.Sprintf(keyMiner, addr)
		reward := uint64(float64(block.Reward) * float64(shares) / float64(block.RoundShares))
		pipe.HIncrBy(r.ctx, minerKey, "immature", -int64(reward))
		pipe.HIncrBy(r.ctx, minerKey, "balance", int64(reward))
	}

	_, err = pipe.Exec(r.ctx)
	return err
}

// RemoveOrphanBlock removes an orphaned block
func (r *RedisClient) RemoveOrphanBlock(block *Block) error {
	// Find and remove from candidates or immature
	for _, key := range []string{keyBlocksCandidates, keyBlocksImmature} {
		results, err := r.client.ZRange(r.ctx, key, 0, -1).Result()
		if err != nil {
			continue
		}

		for _, result := range results {
			var b Block
			if err := json.Unmarshal([]byte(result), &b); err != nil {
				continue
			}
			if b.Hash == block.Hash {
				r.client.ZRem(r.ctx, key, result)

				// Remove immature balance if was immature
				if key == keyBlocksImmature {
					pipe := r.client.Pipeline()
					for addr, shares := range block.Shares {
						minerKey := fmt.Sprintf(keyMiner, addr)
						reward := uint64(float64(block.Reward) * float64(shares) / float64(block.RoundShares))
						pipe.HIncrBy(r.ctx, minerKey, "immature", -int64(reward))
					}
					pipe.Exec(r.ctx)
				}
				return nil
			}
		}
	}
	return nil
}

// GetMiner returns miner data
func (r *RedisClient) GetMiner(address string) (*Miner, error) {
	minerKey := fmt.Sprintf(keyMiner, address)
	data, err := r.client.HGetAll(r.ctx, minerKey).Result()
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	miner := &Miner{Address: address}
	if v, ok := data["balance"]; ok {
		miner.Balance, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["immature"]; ok {
		miner.ImmatureBalance, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["pending"]; ok {
		miner.PendingBalance, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["paid"]; ok {
		miner.TotalPaid, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["blocksFound"]; ok {
		miner.BlocksFound, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["lastShare"]; ok {
		miner.LastShare, _ = strconv.ParseInt(v, 10, 64)
	}

	return miner, nil
}

// GetMinersWithBalance returns all miners with balance >= threshold
func (r *RedisClient) GetMinersWithBalance(threshold uint64) ([]*Miner, error) {
	// Scan for all miner keys
	var miners []*Miner
	var cursor uint64

	for {
		keys, newCursor, err := r.client.Scan(r.ctx, cursor, keyPrefix+"miners:*", 1000).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			// Skip worker keys
			if strings.Contains(key, ":workers") {
				continue
			}

			address := strings.TrimPrefix(key, keyPrefix+"miners:")
			miner, err := r.GetMiner(address)
			if err != nil || miner == nil {
				continue
			}

			if miner.Balance >= threshold {
				miners = append(miners, miner)
			}
		}

		cursor = newCursor
		if cursor == 0 {
			break
		}
	}

	return miners, nil
}

// UpdateMinerBalance updates miner balance after payment
func (r *RedisClient) UpdateMinerBalance(address string, amount uint64, txHash string) error {
	minerKey := fmt.Sprintf(keyMiner, address)
	now := time.Now().Unix()

	pipe := r.client.Pipeline()

	// Deduct from balance, add to pending
	pipe.HIncrBy(r.ctx, minerKey, "balance", -int64(amount))
	pipe.HIncrBy(r.ctx, minerKey, "pending", int64(amount))

	// Store payment
	payment := Payment{
		TxHash:    txHash,
		Address:   address,
		Amount:    amount,
		Timestamp: now,
		Status:    "pending",
	}
	paymentJSON, _ := json.Marshal(payment)

	pipe.ZAdd(r.ctx, keyPaymentsPending, &redis.Z{
		Score:  float64(now),
		Member: string(paymentJSON),
	})

	_, err := pipe.Exec(r.ctx)
	return err
}

// ConfirmPayment confirms a pending payment
func (r *RedisClient) ConfirmPayment(txHash string) error {
	// Find pending payment
	payments, err := r.client.ZRange(r.ctx, keyPaymentsPending, 0, -1).Result()
	if err != nil {
		return err
	}

	for _, p := range payments {
		var payment Payment
		if err := json.Unmarshal([]byte(p), &payment); err != nil {
			continue
		}

		if payment.TxHash == txHash {
			now := time.Now().Unix()

			pipe := r.client.Pipeline()

			// Remove from pending
			pipe.ZRem(r.ctx, keyPaymentsPending, p)

			// Update miner stats
			minerKey := fmt.Sprintf(keyMiner, payment.Address)
			pipe.HIncrBy(r.ctx, minerKey, "pending", -int64(payment.Amount))
			pipe.HIncrBy(r.ctx, minerKey, "paid", int64(payment.Amount))

			// Update payment status
			payment.Status = "confirmed"
			paymentJSON, _ := json.Marshal(payment)

			// Add to all payments
			pipe.ZAdd(r.ctx, keyPaymentsAll, &redis.Z{
				Score:  float64(now),
				Member: string(paymentJSON),
			})

			// Add to address payments
			addrKey := fmt.Sprintf(keyPaymentsAddr, payment.Address)
			pipe.LPush(r.ctx, addrKey, string(paymentJSON))
			pipe.LTrim(r.ctx, addrKey, 0, 99) // Keep last 100

			// Update pool stats
			pipe.HIncrBy(r.ctx, keyStats, "totalPaid", int64(payment.Amount))

			_, err = pipe.Exec(r.ctx)
			return err
		}
	}

	return nil
}

// GetHashrate calculates hashrate from recent shares
func (r *RedisClient) GetHashrate(window time.Duration) (float64, error) {
	minTime := time.Now().Add(-window).Unix()

	// Get all shares in window
	results, err := r.client.ZRangeByScore(r.ctx, keyHashrate, &redis.ZRangeBy{
		Min: strconv.FormatInt(minTime, 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return 0, err
	}

	var totalDiff uint64
	for _, result := range results {
		parts := strings.Split(result, ":")
		if len(parts) >= 1 {
			diff, _ := strconv.ParseUint(parts[0], 10, 64)
			totalDiff += diff
		}
	}

	// Hashrate = total difficulty / window seconds
	return float64(totalDiff) / window.Seconds(), nil
}

// GetMinerHashrate calculates hashrate for a specific miner
func (r *RedisClient) GetMinerHashrate(address string, window time.Duration) (float64, error) {
	minTime := time.Now().Add(-window).Unix()
	addrKey := fmt.Sprintf(keyHashrateAddr, address)

	results, err := r.client.ZRangeByScore(r.ctx, addrKey, &redis.ZRangeBy{
		Min: strconv.FormatInt(minTime, 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return 0, err
	}

	var totalDiff uint64
	for _, result := range results {
		parts := strings.Split(result, ":")
		if len(parts) >= 1 {
			diff, _ := strconv.ParseUint(parts[0], 10, 64)
			totalDiff += diff
		}
	}

	return float64(totalDiff) / window.Seconds(), nil
}

// PurgeStaleHashrate removes old hashrate entries
func (r *RedisClient) PurgeStaleHashrate(window time.Duration) error {
	maxTime := time.Now().Add(-window).Unix()
	_, err := r.client.ZRemRangeByScore(r.ctx, keyHashrate, "-inf", strconv.FormatInt(maxTime, 10)).Result()
	return err
}

// SetNetworkStats updates network statistics
func (r *RedisClient) SetNetworkStats(stats *NetworkStats) error {
	pipe := r.client.Pipeline()
	pipe.HSet(r.ctx, keyNodes, "height", stats.Height)
	pipe.HSet(r.ctx, keyNodes, "difficulty", stats.Difficulty)
	pipe.HSet(r.ctx, keyNodes, "hashrate", stats.Hashrate)
	pipe.HSet(r.ctx, keyNodes, "lastBeat", stats.LastBeat)
	_, err := pipe.Exec(r.ctx)
	return err
}

// GetNetworkStats returns network statistics
func (r *RedisClient) GetNetworkStats() (*NetworkStats, error) {
	data, err := r.client.HGetAll(r.ctx, keyNodes).Result()
	if err != nil {
		return nil, err
	}

	stats := &NetworkStats{}
	if v, ok := data["height"]; ok {
		stats.Height, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["difficulty"]; ok {
		stats.Difficulty, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["hashrate"]; ok {
		stats.Hashrate, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := data["lastBeat"]; ok {
		stats.LastBeat, _ = strconv.ParseInt(v, 10, 64)
	}

	return stats, nil
}

// IsBlacklisted checks if an address is blacklisted
func (r *RedisClient) IsBlacklisted(address string) (bool, error) {
	return r.client.SIsMember(r.ctx, keyBlacklist, address).Result()
}

// IsWhitelisted checks if an address is whitelisted
func (r *RedisClient) IsWhitelisted(address string) (bool, error) {
	return r.client.SIsMember(r.ctx, keyWhitelist, address).Result()
}

// AddToBlacklist adds an address to the blacklist
func (r *RedisClient) AddToBlacklist(address string) error {
	return r.client.SAdd(r.ctx, keyBlacklist, address).Err()
}

// RemoveFromBlacklist removes an address from the blacklist
func (r *RedisClient) RemoveFromBlacklist(address string) error {
	return r.client.SRem(r.ctx, keyBlacklist, address).Err()
}

// GetBlacklist returns all blacklisted addresses
func (r *RedisClient) GetBlacklist() ([]string, error) {
	return r.client.SMembers(r.ctx, keyBlacklist).Result()
}

// GetWhitelist returns all whitelisted IPs
func (r *RedisClient) GetWhitelist() ([]string, error) {
	return r.client.SMembers(r.ctx, keyWhitelist).Result()
}

// AddToWhitelist adds an IP to the whitelist
func (r *RedisClient) AddToWhitelist(ip string) error {
	return r.client.SAdd(r.ctx, keyWhitelist, ip).Err()
}

// RemoveFromWhitelist removes an IP from the whitelist
func (r *RedisClient) RemoveFromWhitelist(ip string) error {
	return r.client.SRem(r.ctx, keyWhitelist, ip).Err()
}

// Payment locking for payout safety

const keyPayoutLock = keyPrefix + "payout:lock"

// LockPayouts acquires a lock for payment processing
func (r *RedisClient) LockPayouts(lockID string, ttl time.Duration) (bool, error) {
	return r.client.SetNX(r.ctx, keyPayoutLock, lockID, ttl).Result()
}

// UnlockPayouts releases the payment lock
func (r *RedisClient) UnlockPayouts(lockID string) error {
	// Only unlock if we own the lock
	current, err := r.client.Get(r.ctx, keyPayoutLock).Result()
	if err != nil {
		return err
	}
	if current == lockID {
		return r.client.Del(r.ctx, keyPayoutLock).Err()
	}
	return nil
}

// IsPayoutsLocked checks if payouts are locked
func (r *RedisClient) IsPayoutsLocked() (bool, error) {
	exists, err := r.client.Exists(r.ctx, keyPayoutLock).Result()
	return exists > 0, err
}

// RollbackPayment reverts a failed payment
func (r *RedisClient) RollbackPayment(address string, amount uint64) error {
	minerKey := fmt.Sprintf(keyMiner, address)

	pipe := r.client.Pipeline()
	pipe.HIncrBy(r.ctx, minerKey, "pending", -int64(amount))
	pipe.HIncrBy(r.ctx, minerKey, "balance", int64(amount))
	_, err := pipe.Exec(r.ctx)
	return err
}

// MoveToPending moves balance to pending before sending payment
func (r *RedisClient) MoveToPending(address string, amount uint64) error {
	minerKey := fmt.Sprintf(keyMiner, address)

	// Atomically move balance to pending
	pipe := r.client.Pipeline()
	pipe.HIncrBy(r.ctx, minerKey, "balance", -int64(amount))
	pipe.HIncrBy(r.ctx, minerKey, "pending", int64(amount))
	_, err := pipe.Exec(r.ctx)
	return err
}

// GetPendingPayments returns all pending payments for rollback
func (r *RedisClient) GetPendingPayments() ([]*Payment, error) {
	results, err := r.client.ZRange(r.ctx, keyPaymentsPending, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	payments := make([]*Payment, 0, len(results))
	for _, result := range results {
		var payment Payment
		if err := json.Unmarshal([]byte(result), &payment); err == nil {
			payments = append(payments, &payment)
		}
	}
	return payments, nil
}

// RemovePendingPayment removes a specific pending payment
func (r *RedisClient) RemovePendingPayment(txHash string) error {
	payments, err := r.client.ZRange(r.ctx, keyPaymentsPending, 0, -1).Result()
	if err != nil {
		return err
	}

	for _, p := range payments {
		var payment Payment
		if err := json.Unmarshal([]byte(p), &payment); err != nil {
			continue
		}
		if payment.TxHash == txHash {
			return r.client.ZRem(r.ctx, keyPaymentsPending, p).Err()
		}
	}
	return nil
}

// GetPoolStats returns pool-wide statistics
func (r *RedisClient) GetPoolStats(window, largeWindow time.Duration) (*PoolStats, error) {
	data, err := r.client.HGetAll(r.ctx, keyStats).Result()
	if err != nil {
		return nil, err
	}

	stats := &PoolStats{}
	if v, ok := data["roundShares"]; ok {
		stats.RoundShares, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["lastBlockFound"]; ok {
		stats.LastBlockFound, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := data["lastBlockHeight"]; ok {
		stats.LastBlockHeight, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["blocksFound"]; ok {
		stats.BlocksFound, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, ok := data["totalPaid"]; ok {
		stats.TotalPaid, _ = strconv.ParseUint(v, 10, 64)
	}

	// Calculate hashrates
	stats.Hashrate, _ = r.GetHashrate(window)
	stats.HashrateLarge, _ = r.GetHashrate(largeWindow)

	// Count miners and workers
	stats.Miners, _ = r.countActiveMiners(window)
	stats.Workers, _ = r.countActiveWorkers(window)

	return stats, nil
}

// countActiveMiners counts miners with recent activity
func (r *RedisClient) countActiveMiners(window time.Duration) (int64, error) {
	minTime := time.Now().Add(-window).Unix()
	var count int64
	var cursor uint64

	for {
		keys, newCursor, err := r.client.Scan(r.ctx, cursor, keyPrefix+"miners:*", 1000).Result()
		if err != nil {
			return 0, err
		}

		for _, key := range keys {
			if strings.Contains(key, ":workers") {
				continue
			}

			lastShare, err := r.client.HGet(r.ctx, key, "lastShare").Int64()
			if err == nil && lastShare >= minTime {
				count++
			}
		}

		cursor = newCursor
		if cursor == 0 {
			break
		}
	}

	return count, nil
}

// countActiveWorkers counts workers with recent activity
func (r *RedisClient) countActiveWorkers(window time.Duration) (int64, error) {
	minTime := time.Now().Add(-window).Unix()
	var count int64
	var cursor uint64

	for {
		keys, newCursor, err := r.client.Scan(r.ctx, cursor, keyPrefix+"miners:*:workers", 1000).Result()
		if err != nil {
			return 0, err
		}

		for _, key := range keys {
			workers, err := r.client.HGetAll(r.ctx, key).Result()
			if err != nil {
				continue
			}

			for _, lastSeenStr := range workers {
				lastSeen, err := strconv.ParseInt(lastSeenStr, 10, 64)
				if err == nil && lastSeen >= minTime {
					count++
				}
			}
		}

		cursor = newCursor
		if cursor == 0 {
			break
		}
	}

	return count, nil
}

// GetRecentBlocks returns recently found blocks
func (r *RedisClient) GetRecentBlocks(limit int64) ([]*Block, error) {
	var blocks []*Block

	// Get from matured
	matured, _ := r.client.ZRevRange(r.ctx, keyBlocksMatured, 0, limit-1).Result()
	for _, b := range matured {
		var block Block
		if err := json.Unmarshal([]byte(b), &block); err == nil {
			blocks = append(blocks, &block)
		}
	}

	// Get from immature
	immature, _ := r.client.ZRevRange(r.ctx, keyBlocksImmature, 0, limit-1).Result()
	for _, b := range immature {
		var block Block
		if err := json.Unmarshal([]byte(b), &block); err == nil {
			blocks = append(blocks, &block)
		}
	}

	// Get from candidates
	candidates, _ := r.client.ZRevRange(r.ctx, keyBlocksCandidates, 0, limit-1).Result()
	for _, b := range candidates {
		var block Block
		if err := json.Unmarshal([]byte(b), &block); err == nil {
			blocks = append(blocks, &block)
		}
	}

	return blocks, nil
}

// GetRecentPayments returns recent payments
func (r *RedisClient) GetRecentPayments(limit int64) ([]*Payment, error) {
	results, err := r.client.ZRevRange(r.ctx, keyPaymentsAll, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}

	payments := make([]*Payment, 0, len(results))
	for _, result := range results {
		var payment Payment
		if err := json.Unmarshal([]byte(result), &payment); err == nil {
			payments = append(payments, &payment)
		}
	}

	return payments, nil
}

// GetMinerPayments returns payment history for a miner
func (r *RedisClient) GetMinerPayments(address string, limit int64) ([]*Payment, error) {
	addrKey := fmt.Sprintf(keyPaymentsAddr, address)
	results, err := r.client.LRange(r.ctx, addrKey, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}

	payments := make([]*Payment, 0, len(results))
	for _, result := range results {
		var payment Payment
		if err := json.Unmarshal([]byte(result), &payment); err == nil {
			payments = append(payments, &payment)
		}
	}

	return payments, nil
}

// LuckStats represents mining luck statistics
type LuckStats struct {
	Luck24h  float64      `json:"luck_24h"`
	Luck7d   float64      `json:"luck_7d"`
	Luck30d  float64      `json:"luck_30d"`
	LuckAll  float64      `json:"luck_all"`
	Blocks   []BlockLuck  `json:"recent_blocks"`
}

// BlockLuck represents luck info for a single block
type BlockLuck struct {
	Height      uint64  `json:"height"`
	Effort      float64 `json:"effort"`
	RoundShares uint64  `json:"round_shares"`
	Difficulty  uint64  `json:"difficulty"`
	Timestamp   int64   `json:"timestamp"`
}

// GetLuckStats calculates mining luck over various time windows
func (r *RedisClient) GetLuckStats() (*LuckStats, error) {
	now := time.Now().Unix()
	day := int64(24 * 60 * 60)

	// Get all matured blocks
	blocks, err := r.GetRecentBlocks(1000)
	if err != nil {
		return nil, err
	}

	stats := &LuckStats{
		Blocks: make([]BlockLuck, 0),
	}

	var shares24h, diff24h uint64
	var shares7d, diff7d uint64
	var shares30d, diff30d uint64
	var sharesAll, diffAll uint64

	for i, block := range blocks {
		// Calculate effort for this block
		var effort float64
		if block.Difficulty > 0 {
			effort = (float64(block.RoundShares) / float64(block.Difficulty)) * 100
		}

		// Add to recent blocks (max 50)
		if i < 50 {
			stats.Blocks = append(stats.Blocks, BlockLuck{
				Height:      block.Height,
				Effort:      effort,
				RoundShares: block.RoundShares,
				Difficulty:  block.Difficulty,
				Timestamp:   block.Timestamp,
			})
		}

		// Aggregate by time window
		age := now - block.Timestamp

		if age <= day {
			shares24h += block.RoundShares
			diff24h += block.Difficulty
		}
		if age <= 7*day {
			shares7d += block.RoundShares
			diff7d += block.Difficulty
		}
		if age <= 30*day {
			shares30d += block.RoundShares
			diff30d += block.Difficulty
		}
		sharesAll += block.RoundShares
		diffAll += block.Difficulty
	}

	// Calculate luck percentages (100% = expected, <100% = lucky, >100% = unlucky)
	if diff24h > 0 {
		stats.Luck24h = (float64(shares24h) / float64(diff24h)) * 100
	}
	if diff7d > 0 {
		stats.Luck7d = (float64(shares7d) / float64(diff7d)) * 100
	}
	if diff30d > 0 {
		stats.Luck30d = (float64(shares30d) / float64(diff30d)) * 100
	}
	if diffAll > 0 {
		stats.LuckAll = (float64(sharesAll) / float64(diffAll)) * 100
	}

	return stats, nil
}

// PoolBackup represents a full pool data backup
type PoolBackup struct {
	Timestamp       int64                  `json:"timestamp"`
	Version         string                 `json:"version"`
	Stats           *PoolStats             `json:"stats"`
	NetworkStats    *NetworkStats          `json:"network_stats"`
	Miners          map[string]*Miner      `json:"miners"`
	CandidateBlocks []*Block               `json:"candidate_blocks"`
	ImmatureBlocks  []*Block               `json:"immature_blocks"`
	MaturedBlocks   []*Block               `json:"matured_blocks"`
	PendingPayments []*Payment             `json:"pending_payments"`
	RecentPayments  []*Payment             `json:"recent_payments"`
	Blacklist       []string               `json:"blacklist"`
	Whitelist       []string               `json:"whitelist"`
}

// CreateBackup creates a JSON backup of all pool data
func (r *RedisClient) CreateBackup() (*PoolBackup, error) {
	backup := &PoolBackup{
		Timestamp: time.Now().Unix(),
		Version:   "1.0",
		Miners:    make(map[string]*Miner),
	}

	// Get stats
	backup.Stats, _ = r.GetPoolStats(10*time.Minute, 3*time.Hour)
	backup.NetworkStats, _ = r.GetNetworkStats()

	// Get blocks
	backup.CandidateBlocks, _ = r.GetCandidateBlocks()
	backup.ImmatureBlocks, _ = r.getBlocksByKey(keyBlocksImmature)
	backup.MaturedBlocks, _ = r.getBlocksByKey(keyBlocksMatured)

	// Get payments
	backup.PendingPayments, _ = r.GetPendingPayments()
	backup.RecentPayments, _ = r.GetRecentPayments(500)

	// Get lists
	backup.Blacklist, _ = r.GetBlacklist()
	backup.Whitelist, _ = r.GetWhitelist()

	// Get all miners
	var cursor uint64
	for {
		keys, newCursor, err := r.client.Scan(r.ctx, cursor, keyPrefix+"miners:*", 1000).Result()
		if err != nil {
			break
		}

		for _, key := range keys {
			if strings.Contains(key, ":workers") {
				continue
			}
			address := strings.TrimPrefix(key, keyPrefix+"miners:")
			miner, err := r.GetMiner(address)
			if err == nil && miner != nil {
				backup.Miners[address] = miner
			}
		}

		cursor = newCursor
		if cursor == 0 {
			break
		}
	}

	return backup, nil
}

// getBlocksByKey retrieves blocks from a specific sorted set
func (r *RedisClient) getBlocksByKey(key string) ([]*Block, error) {
	results, err := r.client.ZRange(r.ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	blocks := make([]*Block, 0, len(results))
	for _, result := range results {
		var block Block
		if err := json.Unmarshal([]byte(result), &block); err == nil {
			blocks = append(blocks, &block)
		}
	}
	return blocks, nil
}

// Debt tracking

const keyDebt = keyPrefix + "debt:%s"
const keyDebtTotal = keyPrefix + "debt:total"

// DebtRecord represents a debt entry for a miner
type DebtRecord struct {
	Address   string `json:"address"`
	Amount    int64  `json:"amount"` // Positive = pool owes miner, Negative = miner owes pool
	Reason    string `json:"reason"`
	Timestamp int64  `json:"timestamp"`
}

// AddDebt records a debt (positive = pool owes miner)
func (r *RedisClient) AddDebt(address string, amount int64, reason string) error {
	pipe := r.client.Pipeline()

	// Update miner's debt balance
	debtKey := fmt.Sprintf(keyDebt, address)
	pipe.IncrBy(r.ctx, debtKey, amount)

	// Update total debt
	pipe.IncrBy(r.ctx, keyDebtTotal, amount)

	// Store debt record for audit
	record := DebtRecord{
		Address:   address,
		Amount:    amount,
		Reason:    reason,
		Timestamp: time.Now().Unix(),
	}
	recordJSON, _ := json.Marshal(record)
	debtHistoryKey := keyPrefix + "debt:history:" + address
	pipe.LPush(r.ctx, debtHistoryKey, string(recordJSON))
	pipe.LTrim(r.ctx, debtHistoryKey, 0, 99) // Keep last 100 records

	_, err := pipe.Exec(r.ctx)
	return err
}

// GetDebt returns the current debt for a miner
func (r *RedisClient) GetDebt(address string) (int64, error) {
	debtKey := fmt.Sprintf(keyDebt, address)
	return r.client.Get(r.ctx, debtKey).Int64()
}

// GetTotalDebt returns the total pool debt
func (r *RedisClient) GetTotalDebt() (int64, error) {
	return r.client.Get(r.ctx, keyDebtTotal).Int64()
}

// GetDebtHistory returns debt history for a miner
func (r *RedisClient) GetDebtHistory(address string, limit int64) ([]DebtRecord, error) {
	debtHistoryKey := keyPrefix + "debt:history:" + address
	results, err := r.client.LRange(r.ctx, debtHistoryKey, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}

	records := make([]DebtRecord, 0, len(results))
	for _, result := range results {
		var record DebtRecord
		if err := json.Unmarshal([]byte(result), &record); err == nil {
			records = append(records, record)
		}
	}
	return records, nil
}

// SettleDebt settles a miner's debt by adjusting their balance
func (r *RedisClient) SettleDebt(address string) error {
	debt, err := r.GetDebt(address)
	if err != nil || debt == 0 {
		return err
	}

	minerKey := fmt.Sprintf(keyMiner, address)
	debtKey := fmt.Sprintf(keyDebt, address)

	pipe := r.client.Pipeline()

	if debt > 0 {
		// Pool owes miner: add to balance
		pipe.HIncrBy(r.ctx, minerKey, "balance", debt)
	} else {
		// Miner owes pool: deduct from balance (ensure non-negative)
		pipe.HIncrBy(r.ctx, minerKey, "balance", debt) // debt is already negative
	}

	// Clear debt
	pipe.Del(r.ctx, debtKey)
	pipe.IncrBy(r.ctx, keyDebtTotal, -debt)

	// Record settlement
	record := DebtRecord{
		Address:   address,
		Amount:    -debt, // Inverse of settled amount
		Reason:    "settlement",
		Timestamp: time.Now().Unix(),
	}
	recordJSON, _ := json.Marshal(record)
	debtHistoryKey := keyPrefix + "debt:history:" + address
	pipe.LPush(r.ctx, debtHistoryKey, string(recordJSON))

	_, err = pipe.Exec(r.ctx)
	return err
}

// Hashrate history keys
const (
	keyHashratePool   = keyPrefix + "hashrate:pool"
	keyHashrateMiner  = keyPrefix + "hashrate:miner:%s"
	keyHashrateWorker = keyPrefix + "hashrate:worker:%s:%s"
	keyWorkersHistory = keyPrefix + "workers:history"
)

// HashratePoint represents a single hashrate data point
type HashratePoint struct {
	Timestamp int64   `json:"timestamp"`
	Hashrate  float64 `json:"hashrate"`
}

// WorkersPoint represents a single workers count data point
type WorkersPoint struct {
	Timestamp int64 `json:"timestamp"`
	Count     int   `json:"count"`
}

// StorePoolHashrate stores a pool hashrate sample
func (r *RedisClient) StorePoolHashrate(hashrate float64) error {
	point := HashratePoint{
		Timestamp: time.Now().Unix(),
		Hashrate:  hashrate,
	}
	data, _ := json.Marshal(point)

	pipe := r.client.Pipeline()
	pipe.ZAdd(r.ctx, keyHashratePool, &redis.Z{
		Score:  float64(point.Timestamp),
		Member: string(data),
	})
	// Keep last 7 days of data (604800 seconds)
	cutoff := time.Now().Unix() - 604800
	pipe.ZRemRangeByScore(r.ctx, keyHashratePool, "0", fmt.Sprintf("%d", cutoff))

	_, err := pipe.Exec(r.ctx)
	return err
}

// StoreMinerHashrate stores a miner hashrate sample
func (r *RedisClient) StoreMinerHashrate(address string, hashrate float64) error {
	point := HashratePoint{
		Timestamp: time.Now().Unix(),
		Hashrate:  hashrate,
	}
	data, _ := json.Marshal(point)

	key := fmt.Sprintf(keyHashrateMiner, address)
	pipe := r.client.Pipeline()
	pipe.ZAdd(r.ctx, key, &redis.Z{
		Score:  float64(point.Timestamp),
		Member: string(data),
	})
	// Keep last 24 hours of data
	cutoff := time.Now().Unix() - 86400
	pipe.ZRemRangeByScore(r.ctx, key, "0", fmt.Sprintf("%d", cutoff))

	_, err := pipe.Exec(r.ctx)
	return err
}

// StoreWorkerHashrate stores a worker hashrate sample
func (r *RedisClient) StoreWorkerHashrate(address, worker string, hashrate float64) error {
	point := HashratePoint{
		Timestamp: time.Now().Unix(),
		Hashrate:  hashrate,
	}
	data, _ := json.Marshal(point)

	key := fmt.Sprintf(keyHashrateWorker, address, worker)
	pipe := r.client.Pipeline()
	pipe.ZAdd(r.ctx, key, &redis.Z{
		Score:  float64(point.Timestamp),
		Member: string(data),
	})
	// Keep last 24 hours
	cutoff := time.Now().Unix() - 86400
	pipe.ZRemRangeByScore(r.ctx, key, "0", fmt.Sprintf("%d", cutoff))

	_, err := pipe.Exec(r.ctx)
	return err
}

// GetPoolHashrateHistory returns pool hashrate history
func (r *RedisClient) GetPoolHashrateHistory(hours int) ([]HashratePoint, error) {
	cutoff := time.Now().Unix() - int64(hours*3600)
	return r.getHashrateHistory(keyHashratePool, cutoff)
}

// GetMinerHashrateHistory returns miner hashrate history
func (r *RedisClient) GetMinerHashrateHistory(address string, hours int) ([]HashratePoint, error) {
	key := fmt.Sprintf(keyHashrateMiner, address)
	cutoff := time.Now().Unix() - int64(hours*3600)
	return r.getHashrateHistory(key, cutoff)
}

// GetWorkerHashrateHistory returns worker hashrate history
func (r *RedisClient) GetWorkerHashrateHistory(address, worker string, hours int) ([]HashratePoint, error) {
	key := fmt.Sprintf(keyHashrateWorker, address, worker)
	cutoff := time.Now().Unix() - int64(hours*3600)
	return r.getHashrateHistory(key, cutoff)
}

// getHashrateHistory retrieves hashrate history from a sorted set
func (r *RedisClient) getHashrateHistory(key string, since int64) ([]HashratePoint, error) {
	results, err := r.client.ZRangeByScore(r.ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", since),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, err
	}

	points := make([]HashratePoint, 0, len(results))
	for _, result := range results {
		var point HashratePoint
		if err := json.Unmarshal([]byte(result), &point); err == nil {
			points = append(points, point)
		}
	}

	return points, nil
}

// StoreWorkersCount stores a workers count sample
func (r *RedisClient) StoreWorkersCount(count int) error {
	point := WorkersPoint{
		Timestamp: time.Now().Unix(),
		Count:     count,
	}
	data, _ := json.Marshal(point)

	pipe := r.client.Pipeline()
	pipe.ZAdd(r.ctx, keyWorkersHistory, &redis.Z{
		Score:  float64(point.Timestamp),
		Member: string(data),
	})
	// Keep last 7 days of data
	cutoff := time.Now().Unix() - 604800
	pipe.ZRemRangeByScore(r.ctx, keyWorkersHistory, "0", fmt.Sprintf("%d", cutoff))

	_, err := pipe.Exec(r.ctx)
	return err
}

// GetWorkersHistory returns workers count history
func (r *RedisClient) GetWorkersHistory(hours int) ([]WorkersPoint, error) {
	cutoff := time.Now().Unix() - int64(hours*3600)

	results, err := r.client.ZRangeByScore(r.ctx, keyWorkersHistory, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", cutoff),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, err
	}

	points := make([]WorkersPoint, 0, len(results))
	for _, result := range results {
		var point WorkersPoint
		if err := json.Unmarshal([]byte(result), &point); err == nil {
			points = append(points, point)
		}
	}

	return points, nil
}

// WorkerStats represents statistics for a single worker
type WorkerStats struct {
	Name     string  `json:"name"`
	Hashrate float64 `json:"hashrate"`
	LastSeen int64   `json:"last_seen"`
}

// GetMinerWorkers returns worker statistics for a miner
func (r *RedisClient) GetMinerWorkers(address string, window time.Duration) ([]WorkerStats, error) {
	// Get worker last seen times
	workerKey := fmt.Sprintf(keyMinerWorkers, address)
	workers, err := r.client.HGetAll(r.ctx, workerKey).Result()
	if err != nil {
		return nil, err
	}

	if len(workers) == 0 {
		return []WorkerStats{}, nil
	}

	// Get all shares in the hashrate window for this miner
	minTime := time.Now().Add(-window).Unix()
	addrKey := fmt.Sprintf(keyHashrateAddr, address)

	results, err := r.client.ZRangeByScore(r.ctx, addrKey, &redis.ZRangeBy{
		Min: strconv.FormatInt(minTime, 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil, err
	}

	// Aggregate difficulty by worker
	workerDiff := make(map[string]uint64)
	for _, result := range results {
		// Format: "difficulty:address:worker:ms"
		parts := strings.Split(result, ":")
		if len(parts) >= 3 {
			diff, _ := strconv.ParseUint(parts[0], 10, 64)
			worker := parts[2]
			workerDiff[worker] += diff
		}
	}

	// Build worker stats
	stats := make([]WorkerStats, 0, len(workers))
	windowSecs := window.Seconds()

	for name, lastSeenStr := range workers {
		lastSeen, _ := strconv.ParseInt(lastSeenStr, 10, 64)

		// Calculate hashrate for this worker
		hashrate := float64(0)
		if diff, ok := workerDiff[name]; ok && windowSecs > 0 {
			hashrate = float64(diff) / windowSecs
		}

		stats = append(stats, WorkerStats{
			Name:     name,
			Hashrate: hashrate,
			LastSeen: lastSeen,
		})
	}

	return stats, nil
}
