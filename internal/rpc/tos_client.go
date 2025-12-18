// Package rpc provides TOS node communication using native TOS daemon API.
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tos-network/tos-pool/internal/util"
)

// NativeAssetHash is the asset hash for native TOS token (64 zeros)
const NativeAssetHash = "0000000000000000000000000000000000000000000000000000000000000000"

// TOSClient handles communication with a TOS node using native TOS daemon API
type TOSClient struct {
	url          string
	timeout      time.Duration
	client       *http.Client
	requestID    uint64
	minerAddress string // Address for get_block_template

	// Health tracking
	mu           sync.RWMutex
	healthy      bool
	lastCheck    time.Time
	successCount int
	failCount    int
}

// NewTOSClient creates a new TOS RPC client
func NewTOSClient(url string, timeout time.Duration) *TOSClient {
	return &TOSClient{
		url:     url,
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
		healthy: true,
	}
}

// SetMinerAddress sets the miner address for get_block_template calls
func (c *TOSClient) SetMinerAddress(address string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.minerAddress = address
}

// NativeRPCRequest represents a TOS native JSON-RPC request with object params
type NativeRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"` // Can be object or nil
	ID      uint64      `json:"id"`
}

// RPCResponse represents a JSON-RPC response
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      uint64          `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// BlockTemplate represents a mining block template
type BlockTemplate struct {
	HeaderHash   string `json:"headerHash"`
	ParentHash   string `json:"parentHash"`
	Height       uint64 `json:"height"`
	Timestamp    uint64 `json:"timestamp"`
	Difficulty   uint64 `json:"difficulty"`
	Target       string `json:"target"`
	ExtraNonce   string `json:"extraNonce"`
	Transactions []byte `json:"transactions,omitempty"`
}

// BlockInfo represents block information
type BlockInfo struct {
	Hash         string `json:"hash"`
	ParentHash   string `json:"parentHash"`
	Height       uint64 `json:"number"`
	Timestamp    uint64 `json:"timestamp"`
	Difficulty   uint64 `json:"difficulty"`
	TotalDiff    string `json:"totalDifficulty"`
	Nonce        string `json:"nonce"`
	Miner        string `json:"miner"`
	Reward       uint64 `json:"reward"`
	Size         uint64 `json:"size"`
	GasUsed      uint64 `json:"gasUsed"`
	GasLimit     uint64 `json:"gasLimit"`
	Transactions int    `json:"transactionCount"`
	TxFees       uint64 `json:"txFees"`
}

// NetworkInfo represents network statistics
type NetworkInfo struct {
	ChainID    uint64 `json:"chainId"`
	Height     uint64 `json:"height"`
	Difficulty uint64 `json:"difficulty"`
	Hashrate   uint64 `json:"hashrate"`
	PeerCount  int    `json:"peerCount"`
	Syncing    bool   `json:"syncing"`
	GasPrice   uint64 `json:"gasPrice"` // Always 0 for TOS (no gas)
}

// TxReceipt represents a transaction receipt
type TxReceipt struct {
	TxHash      string `json:"transactionHash"`
	BlockHash   string `json:"blockHash"`
	BlockNumber uint64 `json:"blockNumber"`
	Status      uint64 `json:"status"`
	GasUsed     uint64 `json:"gasUsed"`
}

// === TOS Native API Response Structures ===

// GetBlockTemplateResult represents get_block_template response
type GetBlockTemplateResult struct {
	Template   string `json:"template"`
	Algorithm  string `json:"algorithm"`
	Height     uint64 `json:"height"`
	TopoHeight uint64 `json:"topoheight"`
	Difficulty string `json:"difficulty"`
}

// GetInfoResult represents get_info response
type GetInfoResult struct {
	Height           uint64 `json:"height"`
	TopoHeight       uint64 `json:"topoheight"`
	StableHeight     uint64 `json:"stableheight"`
	StableTopoHeight uint64 `json:"stable_topoheight"`
	TopBlockHash     string `json:"top_block_hash"`
	Difficulty       string `json:"difficulty"`
	BlockTimeTarget  uint64 `json:"block_time_target"`
	AverageBlockTime uint64 `json:"average_block_time"`
	BlockReward      uint64 `json:"block_reward"`
	DevReward        uint64 `json:"dev_reward"`
	MinerReward      uint64 `json:"miner_reward"`
	MempoolSize      uint64 `json:"mempool_size"`
	Version          string `json:"version"`
	Network          string `json:"network"`
}

// P2pStatusResult represents p2p_status response
type P2pStatusResult struct {
	PeerCount        uint64  `json:"peer_count"`
	MaxPeers         uint64  `json:"max_peers"`
	Tag              *string `json:"tag"`
	OurTopoHeight    uint64  `json:"our_topoheight"`
	BestTopoHeight   uint64  `json:"best_topoheight"`
	MedianTopoHeight uint64  `json:"median_topoheight"`
	PeerID           uint64  `json:"peer_id"`
}

// RPCBlockResponse represents get_block_at_topoheight/get_block_by_hash response
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
	Timestamp            uint64   `json:"timestamp"`
	Height               uint64   `json:"height"`
	Nonce                uint64   `json:"nonce"`
	ExtraNonce           string   `json:"extra_nonce"`
	Miner                string   `json:"miner"`
	TxsHashes            []string `json:"txs_hashes"`
}

// GetBalanceResult represents get_balance response
type GetBalanceResult struct {
	Balance    uint64 `json:"balance"`
	TopoHeight uint64 `json:"topoheight"`
}

// rpcURL returns the full RPC endpoint URL with /json_rpc path
func (c *TOSClient) rpcURL() string {
	// TOS daemon expects RPC calls at /json_rpc endpoint
	url := c.url
	if !strings.HasSuffix(url, "/json_rpc") {
		url = strings.TrimSuffix(url, "/") + "/json_rpc"
	}
	return url
}

// call makes an RPC call using TOS native format (object params)
func (c *TOSClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL(), bytes.NewReader(body))
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
func (c *TOSClient) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.successCount++
	c.failCount = 0
	c.healthy = true
	c.lastCheck = time.Now()
}

// recordFailure records a failed RPC call
func (c *TOSClient) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failCount++
	if c.failCount >= 3 {
		c.healthy = false
		util.Warnf("TOS node marked unhealthy after %d failures", c.failCount)
	}
	c.lastCheck = time.Now()
}

// IsHealthy returns whether the node is healthy
func (c *TOSClient) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthy
}

// parseDifficulty converts string difficulty to uint64
func parseDifficulty(diff string) uint64 {
	val, err := strconv.ParseUint(diff, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// difficultyToTarget converts difficulty to target hex string
// Target = MaxTarget / Difficulty
func difficultyToTarget(difficulty string) string {
	diff, err := strconv.ParseUint(difficulty, 10, 64)
	if err != nil || diff == 0 {
		// Return max target if difficulty is invalid
		return "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	}

	// MaxTarget = 2^256 - 1
	maxTarget := new(big.Int)
	maxTarget.SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)

	// Target = MaxTarget / Difficulty
	diffBig := new(big.Int).SetUint64(diff)
	target := new(big.Int).Div(maxTarget, diffBig)

	// Convert to 64-char hex string (32 bytes, left-padded)
	return fmt.Sprintf("%064x", target)
}

// GetWork returns the current mining work using get_block_template
func (c *TOSClient) GetWork(ctx context.Context) (*BlockTemplate, error) {
	c.mu.RLock()
	minerAddr := c.minerAddress
	c.mu.RUnlock()

	params := map[string]interface{}{
		"address": minerAddr,
	}

	result, err := c.call(ctx, "get_block_template", params)
	if err != nil {
		return nil, err
	}

	var templateResult GetBlockTemplateResult
	if err := json.Unmarshal(result, &templateResult); err != nil {
		return nil, fmt.Errorf("failed to parse block template: %w", err)
	}

	return &BlockTemplate{
		HeaderHash: templateResult.Template,
		Height:     templateResult.Height,
		Difficulty: parseDifficulty(templateResult.Difficulty),
		Target:     difficultyToTarget(templateResult.Difficulty),
	}, nil
}

// GetBlockTemplate returns a full block template (alias for GetWork)
func (c *TOSClient) GetBlockTemplate(ctx context.Context) (*BlockTemplate, error) {
	return c.GetWork(ctx)
}

// SubmitWork submits a mined block using submit_block
func (c *TOSClient) SubmitWork(ctx context.Context, nonce, headerHash, mixDigest string) (bool, error) {
	// For TOS, we submit the block template with nonce embedded
	// The headerHash contains the template, mixDigest is unused in TOS
	return c.SubmitBlock(ctx, headerHash, nonce)
}

// SubmitBlock submits a complete block
func (c *TOSClient) SubmitBlock(ctx context.Context, blockTemplate string, minerWork string) (bool, error) {
	params := map[string]interface{}{
		"block_template": blockTemplate,
	}
	if minerWork != "" {
		params["miner_work"] = minerWork
	}

	result, err := c.call(ctx, "submit_block", params)
	if err != nil {
		return false, err
	}

	var success bool
	if err := json.Unmarshal(result, &success); err != nil {
		// Some implementations return the block hash on success
		return result != nil && string(result) != "null", nil
	}

	return success, nil
}

// convertBlockResponse converts TOS native block response to BlockInfo
func convertBlockResponse(native *RPCBlockResponse) *BlockInfo {
	parentHash := ""
	if len(native.Tips) > 0 {
		parentHash = native.Tips[0]
	}

	return &BlockInfo{
		Hash:         native.Hash,
		ParentHash:   parentHash,
		Height:       native.Height,
		Timestamp:    native.Timestamp / 1000, // Convert ms to seconds
		Difficulty:   parseDifficulty(native.Difficulty),
		TotalDiff:    native.CumulativeDifficulty,
		Nonce:        fmt.Sprintf("%d", native.Nonce),
		Miner:        native.Miner,
		Reward:       native.MinerReward,
		Size:         native.TotalSizeInBytes,
		GasUsed:      0, // TOS has no gas
		GasLimit:     0,
		Transactions: len(native.TxsHashes),
		TxFees:       native.TotalFees,
	}
}

// GetBlockByNumber returns block by topoheight using get_block_at_topoheight
func (c *TOSClient) GetBlockByNumber(ctx context.Context, number uint64) (*BlockInfo, error) {
	params := map[string]interface{}{
		"topoheight": number,
	}

	result, err := c.call(ctx, "get_block_at_topoheight", params)
	if err != nil {
		return nil, err
	}

	if string(result) == "null" {
		return nil, nil
	}

	var blockResp RPCBlockResponse
	if err := json.Unmarshal(result, &blockResp); err != nil {
		return nil, err
	}

	return convertBlockResponse(&blockResp), nil
}

// GetBlockByHash returns block by hash using get_block_by_hash
func (c *TOSClient) GetBlockByHash(ctx context.Context, hash string) (*BlockInfo, error) {
	params := map[string]interface{}{
		"hash": hash,
	}

	result, err := c.call(ctx, "get_block_by_hash", params)
	if err != nil {
		return nil, err
	}

	if string(result) == "null" {
		return nil, nil
	}

	var blockResp RPCBlockResponse
	if err := json.Unmarshal(result, &blockResp); err != nil {
		return nil, err
	}

	return convertBlockResponse(&blockResp), nil
}

// GetLatestBlock returns the latest block using get_info + get_block_at_topoheight
func (c *TOSClient) GetLatestBlock(ctx context.Context) (*BlockInfo, error) {
	// First get current topoheight
	result, err := c.call(ctx, "get_info", nil)
	if err != nil {
		return nil, err
	}

	var info GetInfoResult
	if err := json.Unmarshal(result, &info); err != nil {
		return nil, err
	}

	// Get block at that topoheight
	return c.GetBlockByNumber(ctx, info.TopoHeight)
}

// GetNetworkInfo returns network information using get_info and p2p_status
func (c *TOSClient) GetNetworkInfo(ctx context.Context) (*NetworkInfo, error) {
	// Get node info
	infoResult, err := c.call(ctx, "get_info", nil)
	if err != nil {
		return nil, err
	}

	var info GetInfoResult
	if err := json.Unmarshal(infoResult, &info); err != nil {
		return nil, err
	}

	// Get p2p status
	p2pResult, err := c.call(ctx, "p2p_status", nil)
	if err != nil {
		return nil, err
	}

	var p2p P2pStatusResult
	if err := json.Unmarshal(p2pResult, &p2p); err != nil {
		return nil, err
	}

	// Determine if syncing
	syncing := p2p.OurTopoHeight < p2p.BestTopoHeight

	// Calculate network hashrate: difficulty / average_block_time_seconds
	// AverageBlockTime is in milliseconds, so we multiply difficulty by 1000
	var hashrate uint64
	if info.AverageBlockTime > 0 {
		hashrate = parseDifficulty(info.Difficulty) * 1000 / info.AverageBlockTime
	}

	return &NetworkInfo{
		Height:     info.TopoHeight,
		Difficulty: parseDifficulty(info.Difficulty),
		Hashrate:   hashrate,
		PeerCount:  int(p2p.PeerCount),
		Syncing:    syncing,
		GasPrice:   0, // TOS has no gas
	}, nil
}

// GetBalance returns account balance using get_balance
func (c *TOSClient) GetBalance(ctx context.Context, address string) (uint64, error) {
	params := map[string]interface{}{
		"address": address,
		"asset":   NativeAssetHash,
	}

	result, err := c.call(ctx, "get_balance", params)
	if err != nil {
		return 0, err
	}

	var balanceResult GetBalanceResult
	if err := json.Unmarshal(result, &balanceResult); err != nil {
		return 0, err
	}

	return balanceResult.Balance, nil
}

// GetTransactionReceipt returns transaction receipt
// Note: TOS uses different transaction model, this is a compatibility stub
func (c *TOSClient) GetTransactionReceipt(ctx context.Context, txHash string) (*TxReceipt, error) {
	params := map[string]interface{}{
		"hash": txHash,
	}

	result, err := c.call(ctx, "get_transaction", params)
	if err != nil {
		return nil, err
	}

	if string(result) == "null" {
		return nil, nil
	}

	// Parse TOS transaction format and convert to receipt
	var txData struct {
		Hash       string `json:"hash"`
		InBlock    string `json:"in_block_hash"`
		TopoHeight uint64 `json:"topoheight"`
	}
	if err := json.Unmarshal(result, &txData); err != nil {
		return nil, err
	}

	return &TxReceipt{
		TxHash:      txData.Hash,
		BlockHash:   txData.InBlock,
		BlockNumber: txData.TopoHeight,
		Status:      1, // Assume success if we found it
		GasUsed:     0, // TOS has no gas
	}, nil
}

// SendRawTransaction broadcasts a signed transaction
// Note: TOS uses different transaction format
func (c *TOSClient) SendRawTransaction(ctx context.Context, signedTx string) (string, error) {
	params := map[string]interface{}{
		"data": signedTx,
	}

	result, err := c.call(ctx, "submit_transaction", params)
	if err != nil {
		return "", err
	}

	var success bool
	if err := json.Unmarshal(result, &success); err != nil {
		// Might return tx hash instead
		var txHash string
		if json.Unmarshal(result, &txHash) == nil {
			return txHash, nil
		}
		return "", err
	}

	if success {
		return "submitted", nil
	}
	return "", fmt.Errorf("transaction rejected")
}

// GetNonce returns account nonce
// Note: TOS uses different nonce model
func (c *TOSClient) GetNonce(ctx context.Context, address string) (uint64, error) {
	params := map[string]interface{}{
		"address": address,
	}

	result, err := c.call(ctx, "get_nonce", params)
	if err != nil {
		return 0, err
	}

	var nonceResult struct {
		Nonce uint64 `json:"nonce"`
	}
	if err := json.Unmarshal(result, &nonceResult); err != nil {
		return 0, err
	}

	return nonceResult.Nonce, nil
}

// EstimateGas estimates gas for a transaction
// Note: TOS has no gas, returns 0
func (c *TOSClient) EstimateGas(ctx context.Context, from, to string, value uint64) (uint64, error) {
	return 0, nil // TOS has no gas
}

// SendTransaction creates and sends a payment transaction
// Note: TOS requires proper wallet integration for transactions
func (c *TOSClient) SendTransaction(ctx context.Context, to string, amount uint64) (string, error) {
	// TOS transactions require wallet signing
	// This is a placeholder - real implementation needs wallet integration
	return "", fmt.Errorf("SendTransaction requires wallet integration - use external wallet for payouts")
}

// GetGasPrice returns current gas price
// Note: TOS has no gas, returns 0
func (c *TOSClient) GetGasPrice(ctx context.Context) (uint64, error) {
	return 0, nil // TOS has no gas
}

// GetBlockTxFees returns total transaction fees for a block
func (c *TOSClient) GetBlockTxFees(ctx context.Context, blockNumber uint64) (uint64, error) {
	block, err := c.GetBlockByNumber(ctx, blockNumber)
	if err != nil || block == nil {
		return 0, err
	}
	return block.TxFees, nil
}

// SearchBlockByHash searches for a block hash in a range of heights
// Used for deep orphan detection - searches ±searchRange blocks
func (c *TOSClient) SearchBlockByHash(ctx context.Context, targetHash string, centerHeight uint64, searchRange int) (*BlockInfo, error) {
	// First try direct hash lookup
	block, err := c.GetBlockByHash(ctx, targetHash)
	if err == nil && block != nil {
		return block, nil
	}

	// Fall back to range search
	for offset := 0; offset <= searchRange; offset++ {
		if offset >= 0 {
			height := centerHeight + uint64(offset)
			block, err := c.GetBlockByNumber(ctx, height)
			if err == nil && block != nil && block.Hash == targetHash {
				return block, nil
			}
		}

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

// GetBlockRewardWithFees returns block reward including transaction fees
func (c *TOSClient) GetBlockRewardWithFees(ctx context.Context, blockNumber uint64) (uint64, uint64, error) {
	block, err := c.GetBlockByNumber(ctx, blockNumber)
	if err != nil || block == nil {
		return 0, 0, err
	}

	return block.Reward, block.TxFees, nil
}
