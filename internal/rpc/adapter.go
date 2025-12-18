// Package rpc provides TOS node communication with adapter layer.
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tos-network/tos-pool/internal/util"
)

// TOSNativeClient communicates with TOS daemon using native JSON-RPC API.
// This adapter converts between pool's Ethereum-style calls and TOS daemon's native methods.
type TOSNativeClient struct {
	url       string
	timeout   time.Duration
	client    *http.Client
	requestID uint64

	// Miner address for get_block_template
	minerAddress string

	// Health tracking
	mu           sync.RWMutex
	healthy      bool
	lastCheck    time.Time
	successCount int
	failCount    int
}

// NewTOSNativeClient creates a new TOS native RPC client
func NewTOSNativeClient(url string, timeout time.Duration, minerAddress string) *TOSNativeClient {
	return &TOSNativeClient{
		url:          url,
		timeout:      timeout,
		minerAddress: minerAddress,
		client: &http.Client{
			Timeout: timeout,
		},
		healthy: true,
	}
}

// NativeRPCRequest represents a TOS daemon JSON-RPC request with object params
type NativeRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"` // Can be object or omitted
	ID      uint64      `json:"id"`
}

// TOS daemon native response structures

// GetBlockTemplateResult represents TOS daemon get_block_template response
type GetBlockTemplateResult struct {
	Template   string `json:"template"`   // hex encoded block header
	Algorithm  string `json:"algorithm"`  // "tos/v3"
	Height     uint64 `json:"height"`
	TopoHeight uint64 `json:"topoheight"`
	Difficulty string `json:"difficulty"` // decimal string
}

// GetMinerWorkResult represents TOS daemon get_miner_work response
type GetMinerWorkResult struct {
	Algorithm  string `json:"algorithm"`
	MinerWork  string `json:"miner_work"` // hex encoded miner job
	Height     uint64 `json:"height"`
	Difficulty string `json:"difficulty"`
	TopoHeight uint64 `json:"topoheight"`
}

// RPCBlockResponse represents TOS daemon block response
type RPCBlockResponse struct {
	Hash                 string   `json:"hash"`
	TopoHeight           uint64   `json:"topoheight"`
	BlockType            string   `json:"block_type"`
	Difficulty           string   `json:"difficulty"`
	Supply               uint64   `json:"supply"`
	Reward               uint64   `json:"reward"`
	MinerReward          uint64   `json:"miner_reward"`
	DevReward            uint64   `json:"dev_reward"`
	CumulativeDifficulty string   `json:"cumulative_difficulty"`
	TotalFees            uint64   `json:"total_fees"`
	TotalSizeInBytes     uint64   `json:"total_size_in_bytes"`
	Version              uint64   `json:"version"`
	Tips                 []string `json:"tips"`
	Timestamp            uint64   `json:"timestamp"` // milliseconds
	Height               uint64   `json:"height"`
	Nonce                uint64   `json:"nonce"`
	ExtraNonce           string   `json:"extra_nonce"`
	Miner                string   `json:"miner"`
	TxsHashes            []string `json:"txs_hashes"`
}

// GetInfoResult represents TOS daemon get_info response
type GetInfoResult struct {
	Height           uint64 `json:"height"`
	TopoHeight       uint64 `json:"topoheight"`
	StableHeight     uint64 `json:"stableheight"`
	StableTopoHeight uint64 `json:"stable_topoheight"`
	PrunedTopoHeight *uint64 `json:"pruned_topoheight"`
	TopBlockHash     string `json:"top_block_hash"`
	CirculatingSupply uint64 `json:"circulating_supply"`
	BurnedSupply     uint64 `json:"burned_supply"`
	EmittedSupply    uint64 `json:"emitted_supply"`
	MaximumSupply    uint64 `json:"maximum_supply"`
	Difficulty       string `json:"difficulty"`
	BlockTimeTarget  uint64 `json:"block_time_target"`
	AverageBlockTime uint64 `json:"average_block_time"`
	BlockReward      uint64 `json:"block_reward"`
	DevReward        uint64 `json:"dev_reward"`
	MinerReward      uint64 `json:"miner_reward"`
	MempoolSize      uint64 `json:"mempool_size"`
	Version          string `json:"version"`
	Network          string `json:"network"`
	BlockVersion     uint64 `json:"block_version"`
}

// P2pStatusResult represents TOS daemon p2p_status response
type P2pStatusResult struct {
	PeerCount       int    `json:"peer_count"`
	MaxPeers        int    `json:"max_peers"`
	Tag             *string `json:"tag"`
	OurTopoHeight   uint64 `json:"our_topoheight"`
	BestTopoHeight  uint64 `json:"best_topoheight"`
	MedianTopoHeight uint64 `json:"median_topoheight"`
	PeerID          uint64 `json:"peer_id"`
}

// GetBalanceResult represents TOS daemon get_balance response
type GetBalanceResult struct {
	Balance    uint64 `json:"balance"`
	TopoHeight uint64 `json:"topoheight"`
}

// GetNonceResult represents TOS daemon get_nonce response
type GetNonceResult struct {
	TopoHeight         uint64  `json:"topoheight"`
	Nonce              uint64  `json:"nonce"`
	PreviousTopoHeight *uint64 `json:"previous_topoheight"`
}

// Native TOS asset hash (64 zeros for native token)
const NativeAssetHash = "0000000000000000000000000000000000000000000000000000000000000000"

// callNative makes a native TOS daemon RPC call with object params
func (c *TOSNativeClient) callNative(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := atomic.AddUint64(&c.requestID, 1)

	req := NativeRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		c.recordFailure()
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.recordFailure()
		return nil, err
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		c.recordFailure()
		return nil, err
	}

	if rpcResp.Error != nil {
		c.recordFailure()
		return nil, rpcResp.Error
	}

	c.recordSuccess()
	return rpcResp.Result, nil
}

// recordSuccess records a successful RPC call
func (c *TOSNativeClient) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.successCount++
	c.failCount = 0
	c.healthy = true
	c.lastCheck = time.Now()
}

// recordFailure records a failed RPC call
func (c *TOSNativeClient) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failCount++
	if c.failCount >= 3 {
		c.healthy = false
		util.Warnf("TOS daemon marked unhealthy after %d failures", c.failCount)
	}
	c.lastCheck = time.Now()
}

// IsHealthy returns whether the node is healthy
func (c *TOSNativeClient) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthy
}

// SetMinerAddress sets the miner address for block templates
func (c *TOSNativeClient) SetMinerAddress(address string) {
	c.minerAddress = address
}

// GetWork returns the current mining work (adapter: tos_getWork -> get_block_template)
func (c *TOSNativeClient) GetWork(ctx context.Context) (*BlockTemplate, error) {
	params := map[string]string{
		"address": c.minerAddress,
	}

	result, err := c.callNative(ctx, "get_block_template", params)
	if err != nil {
		return nil, err
	}

	var template GetBlockTemplateResult
	if err := json.Unmarshal(result, &template); err != nil {
		return nil, err
	}

	// Convert difficulty string to target
	target := difficultyToTarget(template.Difficulty)

	return &BlockTemplate{
		HeaderHash: template.Template,
		Height:     template.Height,
		Target:     target,
		Difficulty: parseDifficulty(template.Difficulty),
	}, nil
}

// GetBlockTemplate returns a full block template (adapter: tos_getBlockTemplate -> get_block_template)
func (c *TOSNativeClient) GetBlockTemplate(ctx context.Context) (*BlockTemplate, error) {
	params := map[string]string{
		"address": c.minerAddress,
	}

	result, err := c.callNative(ctx, "get_block_template", params)
	if err != nil {
		return nil, err
	}

	var template GetBlockTemplateResult
	if err := json.Unmarshal(result, &template); err != nil {
		return nil, err
	}

	target := difficultyToTarget(template.Difficulty)

	return &BlockTemplate{
		HeaderHash: template.Template,
		Height:     template.Height,
		Target:     target,
		Difficulty: parseDifficulty(template.Difficulty),
		Timestamp:  uint64(time.Now().UnixMilli()),
	}, nil
}

// SubmitWork submits a mined block (adapter: tos_submitWork -> submit_block)
func (c *TOSNativeClient) SubmitWork(ctx context.Context, nonce, headerHash, mixDigest string) (bool, error) {
	// TOS daemon submit_block accepts block_template with nonce already set
	// The headerHash should be the complete solved block header
	params := map[string]string{
		"block_template": headerHash,
	}

	result, err := c.callNative(ctx, "submit_block", params)
	if err != nil {
		return false, err
	}

	var success bool
	if err := json.Unmarshal(result, &success); err != nil {
		return false, err
	}

	return success, nil
}

// SubmitBlock submits a complete block (adapter: tos_submitBlock -> submit_block)
func (c *TOSNativeClient) SubmitBlock(ctx context.Context, blockTemplate string, minerWork string) (bool, error) {
	params := map[string]string{
		"block_template": blockTemplate,
	}
	if minerWork != "" {
		params["miner_work"] = minerWork
	}

	result, err := c.callNative(ctx, "submit_block", params)
	if err != nil {
		return false, err
	}

	var success bool
	if err := json.Unmarshal(result, &success); err != nil {
		return false, err
	}

	return success, nil
}

// GetBlockByNumber returns block by number (adapter: tos_getBlockByNumber -> get_block_at_topoheight)
func (c *TOSNativeClient) GetBlockByNumber(ctx context.Context, number uint64) (*BlockInfo, error) {
	params := map[string]interface{}{
		"topoheight":  number,
		"include_txs": false,
	}

	result, err := c.callNative(ctx, "get_block_at_topoheight", params)
	if err != nil {
		return nil, err
	}

	if string(result) == "null" {
		return nil, nil
	}

	var block RPCBlockResponse
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, err
	}

	return convertBlockResponse(&block), nil
}

// GetBlockByHash returns block by hash (adapter: tos_getBlockByHash -> get_block_by_hash)
func (c *TOSNativeClient) GetBlockByHash(ctx context.Context, hash string) (*BlockInfo, error) {
	params := map[string]interface{}{
		"hash":        hash,
		"include_txs": false,
	}

	result, err := c.callNative(ctx, "get_block_by_hash", params)
	if err != nil {
		return nil, err
	}

	if string(result) == "null" {
		return nil, nil
	}

	var block RPCBlockResponse
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, err
	}

	return convertBlockResponse(&block), nil
}

// GetLatestBlock returns the latest block (adapter: latest -> get_top_block)
func (c *TOSNativeClient) GetLatestBlock(ctx context.Context) (*BlockInfo, error) {
	params := map[string]interface{}{
		"include_txs": false,
	}

	result, err := c.callNative(ctx, "get_top_block", params)
	if err != nil {
		return nil, err
	}

	var block RPCBlockResponse
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, err
	}

	return convertBlockResponse(&block), nil
}

// GetNetworkInfo returns network information (adapter: various -> get_info + p2p_status)
func (c *TOSNativeClient) GetNetworkInfo(ctx context.Context) (*NetworkInfo, error) {
	// Get daemon info
	infoResult, err := c.callNative(ctx, "get_info", nil)
	if err != nil {
		return nil, err
	}

	var info GetInfoResult
	if err := json.Unmarshal(infoResult, &info); err != nil {
		return nil, err
	}

	// Get peer count from p2p_status
	p2pResult, err := c.callNative(ctx, "p2p_status", nil)
	if err != nil {
		// P2P status might not be available, use default
		return &NetworkInfo{
			Height:     info.TopoHeight,
			Difficulty: parseDifficulty(info.Difficulty),
			PeerCount:  0,
			Syncing:    false,
			GasPrice:   0, // TOS has no gas
		}, nil
	}

	var p2p P2pStatusResult
	if err := json.Unmarshal(p2pResult, &p2p); err != nil {
		return nil, err
	}

	// Determine if syncing
	syncing := p2p.OurTopoHeight < p2p.BestTopoHeight

	return &NetworkInfo{
		Height:     info.TopoHeight,
		Difficulty: parseDifficulty(info.Difficulty),
		PeerCount:  p2p.PeerCount,
		Syncing:    syncing,
		GasPrice:   0, // TOS has no gas concept
	}, nil
}

// GetBalance returns account balance (adapter: tos_getBalance -> get_balance)
func (c *TOSNativeClient) GetBalance(ctx context.Context, address string) (uint64, error) {
	params := map[string]string{
		"address": address,
		"asset":   NativeAssetHash,
	}

	result, err := c.callNative(ctx, "get_balance", params)
	if err != nil {
		return 0, err
	}

	var balance GetBalanceResult
	if err := json.Unmarshal(result, &balance); err != nil {
		return 0, err
	}

	return balance.Balance, nil
}

// GetNonce returns account nonce (adapter: tos_getTransactionCount -> get_nonce)
func (c *TOSNativeClient) GetNonce(ctx context.Context, address string) (uint64, error) {
	params := map[string]string{
		"address": address,
	}

	result, err := c.callNative(ctx, "get_nonce", params)
	if err != nil {
		return 0, err
	}

	var nonce GetNonceResult
	if err := json.Unmarshal(result, &nonce); err != nil {
		return 0, err
	}

	return nonce.Nonce, nil
}

// GetTopoHeight returns current topoheight (adapter: tos_blockNumber -> get_topoheight)
func (c *TOSNativeClient) GetTopoHeight(ctx context.Context) (uint64, error) {
	result, err := c.callNative(ctx, "get_topoheight", nil)
	if err != nil {
		return 0, err
	}

	var topoheight uint64
	if err := json.Unmarshal(result, &topoheight); err != nil {
		return 0, err
	}

	return topoheight, nil
}

// GetHeight returns current height (adapter: alternative to topoheight)
func (c *TOSNativeClient) GetHeight(ctx context.Context) (uint64, error) {
	result, err := c.callNative(ctx, "get_height", nil)
	if err != nil {
		return 0, err
	}

	var height uint64
	if err := json.Unmarshal(result, &height); err != nil {
		return 0, err
	}

	return height, nil
}

// GetDifficulty returns current network difficulty
func (c *TOSNativeClient) GetDifficulty(ctx context.Context) (uint64, error) {
	result, err := c.callNative(ctx, "get_difficulty", nil)
	if err != nil {
		return 0, err
	}

	var diffStr string
	if err := json.Unmarshal(result, &diffStr); err != nil {
		return 0, err
	}

	return parseDifficulty(diffStr), nil
}

// GetVersion returns daemon version
func (c *TOSNativeClient) GetVersion(ctx context.Context) (string, error) {
	result, err := c.callNative(ctx, "get_version", nil)
	if err != nil {
		return "", err
	}

	var version string
	if err := json.Unmarshal(result, &version); err != nil {
		return "", err
	}

	return version, nil
}

// Helper functions

// convertBlockResponse converts TOS native block response to pool's BlockInfo
func convertBlockResponse(native *RPCBlockResponse) *BlockInfo {
	parentHash := ""
	if len(native.Tips) > 0 {
		parentHash = native.Tips[0]
	}

	return &BlockInfo{
		Hash:         native.Hash,
		ParentHash:   parentHash,
		Height:       native.TopoHeight, // Use topoheight as "number"
		Timestamp:    native.Timestamp / 1000, // Convert ms to seconds
		Difficulty:   parseDifficulty(native.Difficulty),
		TotalDiff:    native.CumulativeDifficulty,
		Nonce:        fmt.Sprintf("0x%x", native.Nonce),
		Miner:        native.Miner,
		Reward:       native.MinerReward, // Use miner reward, not total reward
		Size:         native.TotalSizeInBytes,
		GasUsed:      0, // TOS has no gas
		GasLimit:     0,
		Transactions: len(native.TxsHashes),
		TxFees:       native.TotalFees,
	}
}

// parseDifficulty converts difficulty string to uint64
func parseDifficulty(diffStr string) uint64 {
	diff := new(big.Int)
	diff.SetString(diffStr, 10)
	// Return lower 64 bits (sufficient for most practical difficulties)
	return diff.Uint64()
}

// difficultyToTarget converts difficulty to target hex string
func difficultyToTarget(diffStr string) string {
	diff := new(big.Int)
	diff.SetString(diffStr, 10)

	if diff.Sign() == 0 {
		return "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	}

	// target = maxTarget / difficulty
	// maxTarget = 2^256 - 1
	maxTarget := new(big.Int)
	maxTarget.SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)

	target := new(big.Int)
	target.Div(maxTarget, diff)

	// Return as 64-char hex string (32 bytes)
	return fmt.Sprintf("%064x", target)
}

// SearchBlockByHash searches for a block hash in a range of heights
func (c *TOSNativeClient) SearchBlockByHash(ctx context.Context, targetHash string, centerHeight uint64, searchRange int) (*BlockInfo, error) {
	// Search from center outward
	for offset := 0; offset <= searchRange; offset++ {
		// Check center + offset
		if offset >= 0 {
			height := centerHeight + uint64(offset)
			block, err := c.GetBlockByNumber(ctx, height)
			if err == nil && block != nil && block.Hash == targetHash {
				return block, nil
			}
		}

		// Check center - offset (skip 0 to avoid duplicate)
		if offset > 0 && centerHeight >= uint64(offset) {
			height := centerHeight - uint64(offset)
			block, err := c.GetBlockByNumber(ctx, height)
			if err == nil && block != nil && block.Hash == targetHash {
				return block, nil
			}
		}
	}

	return nil, nil
}

// GetBlockRewardWithFees returns block reward and fees for a block
func (c *TOSNativeClient) GetBlockRewardWithFees(ctx context.Context, blockNumber uint64) (uint64, uint64, error) {
	block, err := c.GetBlockByNumber(ctx, blockNumber)
	if err != nil || block == nil {
		return 0, 0, err
	}

	return block.Reward, block.TxFees, nil
}
