// Package rpc provides TOS node communication.
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tos-network/tos-pool/internal/util"
)

// TOSClient handles communication with a TOS node
type TOSClient struct {
	url       string
	timeout   time.Duration
	client    *http.Client
	requestID uint64

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

// RPCRequest represents a JSON-RPC request
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params,omitempty"`
	ID      uint64        `json:"id"`
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
	TxFees       uint64 `json:"txFees"` // Total transaction fees in block
}

// NetworkInfo represents network statistics
type NetworkInfo struct {
	ChainID     uint64 `json:"chainId"`
	Height      uint64 `json:"height"`
	Difficulty  uint64 `json:"difficulty"`
	Hashrate    uint64 `json:"hashrate"`
	PeerCount   int    `json:"peerCount"`
	Syncing     bool   `json:"syncing"`
	GasPrice    uint64 `json:"gasPrice"`
}

// TxReceipt represents a transaction receipt
type TxReceipt struct {
	TxHash      string `json:"transactionHash"`
	BlockHash   string `json:"blockHash"`
	BlockNumber uint64 `json:"blockNumber"`
	Status      uint64 `json:"status"`
	GasUsed     uint64 `json:"gasUsed"`
}

// call makes an RPC call
func (c *TOSClient) call(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	id := atomic.AddUint64(&c.requestID, 1)

	req := RPCRequest{
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

// GetWork returns the current mining work
func (c *TOSClient) GetWork(ctx context.Context) (*BlockTemplate, error) {
	result, err := c.call(ctx, "tos_getWork")
	if err != nil {
		return nil, err
	}

	// GetWork returns [headerHash, seedHash, target, height]
	var work []string
	if err := json.Unmarshal(result, &work); err != nil {
		return nil, err
	}

	if len(work) < 4 {
		return nil, fmt.Errorf("invalid work response")
	}

	height := uint64(0)
	fmt.Sscanf(work[3], "0x%x", &height)

	return &BlockTemplate{
		HeaderHash: work[0],
		Target:     work[2],
		Height:     height,
	}, nil
}

// GetBlockTemplate returns a full block template
func (c *TOSClient) GetBlockTemplate(ctx context.Context) (*BlockTemplate, error) {
	result, err := c.call(ctx, "tos_getBlockTemplate")
	if err != nil {
		return nil, err
	}

	var template BlockTemplate
	if err := json.Unmarshal(result, &template); err != nil {
		return nil, err
	}

	return &template, nil
}

// SubmitWork submits a mined block
func (c *TOSClient) SubmitWork(ctx context.Context, nonce, headerHash, mixDigest string) (bool, error) {
	result, err := c.call(ctx, "tos_submitWork", nonce, headerHash, mixDigest)
	if err != nil {
		return false, err
	}

	var success bool
	if err := json.Unmarshal(result, &success); err != nil {
		return false, err
	}

	return success, nil
}

// SubmitBlock submits a complete block
func (c *TOSClient) SubmitBlock(ctx context.Context, block interface{}) (bool, error) {
	result, err := c.call(ctx, "tos_submitBlock", block)
	if err != nil {
		return false, err
	}

	var success bool
	if err := json.Unmarshal(result, &success); err != nil {
		return false, err
	}

	return success, nil
}

// GetBlockByNumber returns block by number
func (c *TOSClient) GetBlockByNumber(ctx context.Context, number uint64) (*BlockInfo, error) {
	result, err := c.call(ctx, "tos_getBlockByNumber", fmt.Sprintf("0x%x", number), true)
	if err != nil {
		return nil, err
	}

	if string(result) == "null" {
		return nil, nil
	}

	var block BlockInfo
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, err
	}

	return &block, nil
}

// GetBlockByHash returns block by hash
func (c *TOSClient) GetBlockByHash(ctx context.Context, hash string) (*BlockInfo, error) {
	result, err := c.call(ctx, "tos_getBlockByHash", hash, true)
	if err != nil {
		return nil, err
	}

	if string(result) == "null" {
		return nil, nil
	}

	var block BlockInfo
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, err
	}

	return &block, nil
}

// GetLatestBlock returns the latest block
func (c *TOSClient) GetLatestBlock(ctx context.Context) (*BlockInfo, error) {
	result, err := c.call(ctx, "tos_getBlockByNumber", "latest", true)
	if err != nil {
		return nil, err
	}

	var block BlockInfo
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, err
	}

	return &block, nil
}

// GetNetworkInfo returns network information
func (c *TOSClient) GetNetworkInfo(ctx context.Context) (*NetworkInfo, error) {
	// Get block number
	heightResult, err := c.call(ctx, "tos_blockNumber")
	if err != nil {
		return nil, err
	}

	var heightHex string
	json.Unmarshal(heightResult, &heightHex)
	var height uint64
	fmt.Sscanf(heightHex, "0x%x", &height)

	// Get latest block for difficulty
	block, err := c.GetLatestBlock(ctx)
	if err != nil {
		return nil, err
	}

	// Get peer count
	peerResult, err := c.call(ctx, "net_peerCount")
	if err != nil {
		return nil, err
	}
	var peerHex string
	json.Unmarshal(peerResult, &peerHex)
	var peerCount int
	fmt.Sscanf(peerHex, "0x%x", &peerCount)

	// Check syncing status
	syncResult, err := c.call(ctx, "tos_syncing")
	if err != nil {
		return nil, err
	}
	syncing := string(syncResult) != "false"

	// Get gas price
	gasPriceResult, _ := c.call(ctx, "tos_gasPrice")
	var gasPriceHex string
	json.Unmarshal(gasPriceResult, &gasPriceHex)
	var gasPrice uint64
	fmt.Sscanf(gasPriceHex, "0x%x", &gasPrice)

	return &NetworkInfo{
		Height:     height,
		Difficulty: block.Difficulty,
		PeerCount:  peerCount,
		Syncing:    syncing,
		GasPrice:   gasPrice,
	}, nil
}

// GetBalance returns account balance
func (c *TOSClient) GetBalance(ctx context.Context, address string) (uint64, error) {
	result, err := c.call(ctx, "tos_getBalance", address, "latest")
	if err != nil {
		return 0, err
	}

	var balanceHex string
	if err := json.Unmarshal(result, &balanceHex); err != nil {
		return 0, err
	}

	var balance uint64
	fmt.Sscanf(balanceHex, "0x%x", &balance)
	return balance, nil
}

// GetTransactionReceipt returns transaction receipt
func (c *TOSClient) GetTransactionReceipt(ctx context.Context, txHash string) (*TxReceipt, error) {
	result, err := c.call(ctx, "tos_getTransactionReceipt", txHash)
	if err != nil {
		return nil, err
	}

	if string(result) == "null" {
		return nil, nil
	}

	var receipt TxReceipt
	if err := json.Unmarshal(result, &receipt); err != nil {
		return nil, err
	}

	return &receipt, nil
}

// SendRawTransaction broadcasts a signed transaction
func (c *TOSClient) SendRawTransaction(ctx context.Context, signedTx string) (string, error) {
	result, err := c.call(ctx, "tos_sendRawTransaction", signedTx)
	if err != nil {
		return "", err
	}

	var txHash string
	if err := json.Unmarshal(result, &txHash); err != nil {
		return "", err
	}

	return txHash, nil
}

// GetNonce returns account nonce
func (c *TOSClient) GetNonce(ctx context.Context, address string) (uint64, error) {
	result, err := c.call(ctx, "tos_getTransactionCount", address, "pending")
	if err != nil {
		return 0, err
	}

	var nonceHex string
	if err := json.Unmarshal(result, &nonceHex); err != nil {
		return 0, err
	}

	var nonce uint64
	fmt.Sscanf(nonceHex, "0x%x", &nonce)
	return nonce, nil
}

// EstimateGas estimates gas for a transaction
func (c *TOSClient) EstimateGas(ctx context.Context, from, to string, value uint64) (uint64, error) {
	tx := map[string]string{
		"from":  from,
		"to":    to,
		"value": fmt.Sprintf("0x%x", value),
	}

	result, err := c.call(ctx, "tos_estimateGas", tx)
	if err != nil {
		return 0, err
	}

	var gasHex string
	if err := json.Unmarshal(result, &gasHex); err != nil {
		return 0, err
	}

	var gas uint64
	fmt.Sscanf(gasHex, "0x%x", &gas)
	return gas, nil
}

// SendTransaction creates and sends a payment transaction
// Note: In production, this requires proper wallet key management
// and transaction signing. This implementation uses eth_sendTransaction
// which requires the node to have the pool's account unlocked.
func (c *TOSClient) SendTransaction(ctx context.Context, to string, amount uint64) (string, error) {
	// Get gas price
	gasPrice, err := c.GetGasPrice(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get gas price: %w", err)
	}

	// Estimate gas
	// For simple transfers, use default gas limit
	gasLimit := uint64(21000)

	tx := map[string]string{
		"to":       to,
		"value":    fmt.Sprintf("0x%x", amount),
		"gas":      fmt.Sprintf("0x%x", gasLimit),
		"gasPrice": fmt.Sprintf("0x%x", gasPrice),
	}

	result, err := c.call(ctx, "tos_sendTransaction", tx)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	var txHash string
	if err := json.Unmarshal(result, &txHash); err != nil {
		return "", fmt.Errorf("failed to parse tx hash: %w", err)
	}

	return txHash, nil
}

// GetGasPrice returns current gas price
func (c *TOSClient) GetGasPrice(ctx context.Context) (uint64, error) {
	result, err := c.call(ctx, "tos_gasPrice")
	if err != nil {
		return 0, err
	}

	var gasPriceHex string
	if err := json.Unmarshal(result, &gasPriceHex); err != nil {
		return 0, err
	}

	var gasPrice uint64
	fmt.Sscanf(gasPriceHex, "0x%x", &gasPrice)
	return gasPrice, nil
}

// GetBlockTxFees calculates total transaction fees for a block
func (c *TOSClient) GetBlockTxFees(ctx context.Context, blockNumber uint64) (uint64, error) {
	// Get block with full transactions
	result, err := c.call(ctx, "tos_getBlockByNumber", fmt.Sprintf("0x%x", blockNumber), true)
	if err != nil {
		return 0, err
	}

	if string(result) == "null" {
		return 0, nil
	}

	// Parse block with transactions
	var blockData struct {
		Transactions []struct {
			Hash     string `json:"hash"`
			GasPrice string `json:"gasPrice"`
		} `json:"transactions"`
	}
	if err := json.Unmarshal(result, &blockData); err != nil {
		return 0, err
	}

	var totalFees uint64
	for _, tx := range blockData.Transactions {
		// Get receipt for actual gas used
		receipt, err := c.GetTransactionReceipt(ctx, tx.Hash)
		if err != nil || receipt == nil {
			continue
		}

		var gasPrice uint64
		fmt.Sscanf(tx.GasPrice, "0x%x", &gasPrice)

		totalFees += receipt.GasUsed * gasPrice
	}

	return totalFees, nil
}

// SearchBlockByHash searches for a block hash in a range of heights
// Used for deep orphan detection - searches ±searchRange blocks
func (c *TOSClient) SearchBlockByHash(ctx context.Context, targetHash string, centerHeight uint64, searchRange int) (*BlockInfo, error) {
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

// GetBlockRewardWithFees returns block reward including transaction fees
func (c *TOSClient) GetBlockRewardWithFees(ctx context.Context, blockNumber uint64) (uint64, uint64, error) {
	block, err := c.GetBlockByNumber(ctx, blockNumber)
	if err != nil || block == nil {
		return 0, 0, err
	}

	txFees, _ := c.GetBlockTxFees(ctx, blockNumber)

	return block.Reward, txFees, nil
}
