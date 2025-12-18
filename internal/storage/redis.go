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
