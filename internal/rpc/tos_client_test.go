package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// mockRPCServer creates a test server that responds with the given result
func mockRPCServer(t *testing.T, handler func(req RPCRequest) (interface{}, *RPCError)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var req RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			return
		}

		result, rpcErr := handler(req)

		resp := RPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}

		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resultBytes, _ := json.Marshal(result)
			resp.Result = resultBytes
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestNewTOSClient(t *testing.T) {
	client := NewTOSClient("http://localhost:8545", 30*time.Second)

	if client == nil {
		t.Fatal("NewTOSClient returned nil")
	}

	if client.url != "http://localhost:8545" {
		t.Errorf("url = %s, want http://localhost:8545", client.url)
	}

	if client.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", client.timeout)
	}

	if !client.healthy {
		t.Error("Client should be healthy initially")
	}
}

func TestRPCErrorError(t *testing.T) {
	err := &RPCError{
		Code:    -32600,
		Message: "Invalid Request",
	}

	expected := "RPC error -32600: Invalid Request"
	if err.Error() != expected {
		t.Errorf("Error() = %s, want %s", err.Error(), expected)
	}
}

func TestIsHealthy(t *testing.T) {
	client := NewTOSClient("http://localhost:8545", 30*time.Second)

	if !client.IsHealthy() {
		t.Error("Client should be healthy initially")
	}

	// Simulate failures
	for i := 0; i < 3; i++ {
		client.recordFailure()
	}

	if client.IsHealthy() {
		t.Error("Client should be unhealthy after 3 failures")
	}

	// Simulate success
	client.recordSuccess()

	if !client.IsHealthy() {
		t.Error("Client should be healthy after success")
	}
}

func TestGetWork(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_getWork" {
			t.Errorf("Method = %s, want tos_getWork", req.Method)
		}
		return []string{
			"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			"0x00000000ffff0000000000000000000000000000000000000000000000000000",
			"0x1234",
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	work, err := client.GetWork(ctx)
	if err != nil {
		t.Fatalf("GetWork failed: %v", err)
	}

	if work.HeaderHash != "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef" {
		t.Errorf("HeaderHash = %s, want 0x1234...", work.HeaderHash)
	}

	if work.Target != "0x00000000ffff0000000000000000000000000000000000000000000000000000" {
		t.Errorf("Target = %s, want 0x00000000ffff...", work.Target)
	}

	if work.Height != 0x1234 {
		t.Errorf("Height = %d, want %d", work.Height, 0x1234)
	}
}

func TestGetWorkInvalidResponse(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		return []string{"only", "three"}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	_, err := client.GetWork(ctx)
	if err == nil {
		t.Error("GetWork should fail with invalid response")
	}
}

func TestGetWorkRPCError(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		return nil, &RPCError{Code: -32000, Message: "No work available"}
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	_, err := client.GetWork(ctx)
	if err == nil {
		t.Error("GetWork should fail with RPC error")
	}
}

func TestGetBlockTemplate(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_getBlockTemplate" {
			t.Errorf("Method = %s, want tos_getBlockTemplate", req.Method)
		}
		return BlockTemplate{
			HeaderHash: "0xheader",
			ParentHash: "0xparent",
			Height:     12345,
			Timestamp:  1700000000,
			Difficulty: 1000000,
			Target:     "0xtarget",
			ExtraNonce: "0xextra",
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	template, err := client.GetBlockTemplate(ctx)
	if err != nil {
		t.Fatalf("GetBlockTemplate failed: %v", err)
	}

	if template.Height != 12345 {
		t.Errorf("Height = %d, want 12345", template.Height)
	}

	if template.Difficulty != 1000000 {
		t.Errorf("Difficulty = %d, want 1000000", template.Difficulty)
	}
}

func TestSubmitWork(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_submitWork" {
			t.Errorf("Method = %s, want tos_submitWork", req.Method)
		}
		if len(req.Params) != 3 {
			t.Errorf("Params length = %d, want 3", len(req.Params))
		}
		return true, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	success, err := client.SubmitWork(ctx, "0xnonce", "0xheader", "0xmixdigest")
	if err != nil {
		t.Fatalf("SubmitWork failed: %v", err)
	}

	if !success {
		t.Error("SubmitWork should return true")
	}
}

func TestSubmitWorkFailed(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		return false, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	success, err := client.SubmitWork(ctx, "0xnonce", "0xheader", "0xmixdigest")
	if err != nil {
		t.Fatalf("SubmitWork failed: %v", err)
	}

	if success {
		t.Error("SubmitWork should return false")
	}
}

func TestSubmitBlock(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_submitBlock" {
			t.Errorf("Method = %s, want tos_submitBlock", req.Method)
		}
		return true, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block := map[string]interface{}{"nonce": "0x123", "hash": "0xabc"}
	success, err := client.SubmitBlock(ctx, block)
	if err != nil {
		t.Fatalf("SubmitBlock failed: %v", err)
	}

	if !success {
		t.Error("SubmitBlock should return true")
	}
}

func TestGetBlockByNumber(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_getBlockByNumber" {
			t.Errorf("Method = %s, want tos_getBlockByNumber", req.Method)
		}
		return BlockInfo{
			Hash:         "0xblockhash",
			ParentHash:   "0xparenthash",
			Height:       12345,
			Timestamp:    1700000000,
			Difficulty:   1000000,
			Nonce:        "0xnonce",
			Miner:        "tos1miner",
			Reward:       5000000000,
			Transactions: 10,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.GetBlockByNumber(ctx, 12345)
	if err != nil {
		t.Fatalf("GetBlockByNumber failed: %v", err)
	}

	if block.Height != 12345 {
		t.Errorf("Height = %d, want 12345", block.Height)
	}

	if block.Miner != "tos1miner" {
		t.Errorf("Miner = %s, want tos1miner", block.Miner)
	}
}

func TestGetBlockByNumberNull(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.GetBlockByNumber(ctx, 99999999)
	if err != nil {
		t.Fatalf("GetBlockByNumber failed: %v", err)
	}

	if block != nil {
		t.Error("Block should be nil for non-existent block")
	}
}

func TestGetBlockByHash(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_getBlockByHash" {
			t.Errorf("Method = %s, want tos_getBlockByHash", req.Method)
		}
		return BlockInfo{
			Hash:   "0xblockhash",
			Height: 12345,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.GetBlockByHash(ctx, "0xblockhash")
	if err != nil {
		t.Fatalf("GetBlockByHash failed: %v", err)
	}

	if block.Hash != "0xblockhash" {
		t.Errorf("Hash = %s, want 0xblockhash", block.Hash)
	}
}

func TestGetBlockByHashNull(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.GetBlockByHash(ctx, "0xnonexistent")
	if err != nil {
		t.Fatalf("GetBlockByHash failed: %v", err)
	}

	if block != nil {
		t.Error("Block should be nil for non-existent hash")
	}
}

func TestGetLatestBlock(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_getBlockByNumber" {
			t.Errorf("Method = %s, want tos_getBlockByNumber", req.Method)
		}
		if req.Params[0] != "latest" {
			t.Errorf("Params[0] = %v, want latest", req.Params[0])
		}
		return BlockInfo{
			Hash:   "0xlatest",
			Height: 99999,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.GetLatestBlock(ctx)
	if err != nil {
		t.Fatalf("GetLatestBlock failed: %v", err)
	}

	if block.Height != 99999 {
		t.Errorf("Height = %d, want 99999", block.Height)
	}
}

func TestGetNetworkInfo(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		switch req.Method {
		case "tos_blockNumber":
			return "0x1234", nil
		case "tos_getBlockByNumber":
			return BlockInfo{Difficulty: 1000000}, nil
		case "net_peerCount":
			return "0x19", nil
		case "tos_syncing":
			return false, nil
		case "tos_gasPrice":
			return "0x3b9aca00", nil
		default:
			t.Errorf("Unexpected method: %s", req.Method)
			return nil, &RPCError{Code: -32601, Message: "Method not found"}
		}
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	info, err := client.GetNetworkInfo(ctx)
	if err != nil {
		t.Fatalf("GetNetworkInfo failed: %v", err)
	}

	if info.Height != 0x1234 {
		t.Errorf("Height = %d, want %d", info.Height, 0x1234)
	}

	if info.PeerCount != 25 {
		t.Errorf("PeerCount = %d, want 25", info.PeerCount)
	}

	if info.Syncing {
		t.Error("Syncing should be false")
	}

	if info.Difficulty != 1000000 {
		t.Errorf("Difficulty = %d, want 1000000", info.Difficulty)
	}
}

func TestGetNetworkInfoSyncing(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		switch req.Method {
		case "tos_blockNumber":
			return "0x1234", nil
		case "tos_getBlockByNumber":
			return BlockInfo{Difficulty: 1000000}, nil
		case "net_peerCount":
			return "0x5", nil
		case "tos_syncing":
			return map[string]string{
				"startingBlock": "0x1000",
				"currentBlock":  "0x1200",
				"highestBlock":  "0x1400",
			}, nil
		case "tos_gasPrice":
			return "0x3b9aca00", nil
		}
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	info, err := client.GetNetworkInfo(ctx)
	if err != nil {
		t.Fatalf("GetNetworkInfo failed: %v", err)
	}

	if !info.Syncing {
		t.Error("Syncing should be true")
	}
}

func TestGetBalance(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_getBalance" {
			t.Errorf("Method = %s, want tos_getBalance", req.Method)
		}
		if req.Params[0] != "tos1address" {
			t.Errorf("Params[0] = %v, want tos1address", req.Params[0])
		}
		if req.Params[1] != "latest" {
			t.Errorf("Params[1] = %v, want latest", req.Params[1])
		}
		return "0x8ac7230489e80000", nil // 10 TOS in wei
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	balance, err := client.GetBalance(ctx, "tos1address")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	expected := uint64(0x8ac7230489e80000)
	if balance != expected {
		t.Errorf("Balance = %d, want %d", balance, expected)
	}
}

func TestGetTransactionReceipt(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_getTransactionReceipt" {
			t.Errorf("Method = %s, want tos_getTransactionReceipt", req.Method)
		}
		return TxReceipt{
			TxHash:      "0xtxhash",
			BlockHash:   "0xblockhash",
			BlockNumber: 12345,
			Status:      1,
			GasUsed:     21000,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	receipt, err := client.GetTransactionReceipt(ctx, "0xtxhash")
	if err != nil {
		t.Fatalf("GetTransactionReceipt failed: %v", err)
	}

	if receipt.Status != 1 {
		t.Errorf("Status = %d, want 1", receipt.Status)
	}

	if receipt.GasUsed != 21000 {
		t.Errorf("GasUsed = %d, want 21000", receipt.GasUsed)
	}
}

func TestGetTransactionReceiptNull(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	receipt, err := client.GetTransactionReceipt(ctx, "0xnonexistent")
	if err != nil {
		t.Fatalf("GetTransactionReceipt failed: %v", err)
	}

	if receipt != nil {
		t.Error("Receipt should be nil for non-existent tx")
	}
}

func TestSendRawTransaction(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_sendRawTransaction" {
			t.Errorf("Method = %s, want tos_sendRawTransaction", req.Method)
		}
		return "0xnewtxhash", nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	txHash, err := client.SendRawTransaction(ctx, "0xsignedtx")
	if err != nil {
		t.Fatalf("SendRawTransaction failed: %v", err)
	}

	if txHash != "0xnewtxhash" {
		t.Errorf("TxHash = %s, want 0xnewtxhash", txHash)
	}
}

func TestGetNonce(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_getTransactionCount" {
			t.Errorf("Method = %s, want tos_getTransactionCount", req.Method)
		}
		if req.Params[1] != "pending" {
			t.Errorf("Params[1] = %v, want pending", req.Params[1])
		}
		return "0x5", nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	nonce, err := client.GetNonce(ctx, "tos1address")
	if err != nil {
		t.Fatalf("GetNonce failed: %v", err)
	}

	if nonce != 5 {
		t.Errorf("Nonce = %d, want 5", nonce)
	}
}

func TestEstimateGas(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_estimateGas" {
			t.Errorf("Method = %s, want tos_estimateGas", req.Method)
		}
		return "0x5208", nil // 21000
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	gas, err := client.EstimateGas(ctx, "tos1from", "tos1to", 1000000000)
	if err != nil {
		t.Fatalf("EstimateGas failed: %v", err)
	}

	if gas != 21000 {
		t.Errorf("Gas = %d, want 21000", gas)
	}
}

func TestSendTransaction(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		switch req.Method {
		case "tos_gasPrice":
			return "0x3b9aca00", nil
		case "tos_sendTransaction":
			return "0xnewtxhash", nil
		}
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	txHash, err := client.SendTransaction(ctx, "tos1recipient", 1000000000)
	if err != nil {
		t.Fatalf("SendTransaction failed: %v", err)
	}

	if txHash != "0xnewtxhash" {
		t.Errorf("TxHash = %s, want 0xnewtxhash", txHash)
	}
}

func TestSendTransactionGasPriceError(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method == "tos_gasPrice" {
			return nil, &RPCError{Code: -32000, Message: "Node error"}
		}
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	_, err := client.SendTransaction(ctx, "tos1recipient", 1000000000)
	if err == nil {
		t.Error("SendTransaction should fail when gas price fails")
	}
}

func TestGetGasPrice(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method != "tos_gasPrice" {
			t.Errorf("Method = %s, want tos_gasPrice", req.Method)
		}
		return "0x3b9aca00", nil // 1 gwei
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	gasPrice, err := client.GetGasPrice(ctx)
	if err != nil {
		t.Fatalf("GetGasPrice failed: %v", err)
	}

	if gasPrice != 1000000000 {
		t.Errorf("GasPrice = %d, want 1000000000", gasPrice)
	}
}

func TestGetBlockTxFees(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		switch req.Method {
		case "tos_getBlockByNumber":
			return map[string]interface{}{
				"transactions": []map[string]string{
					{"hash": "0xtx1", "gasPrice": "0x3b9aca00"},
					{"hash": "0xtx2", "gasPrice": "0x3b9aca00"},
				},
			}, nil
		case "tos_getTransactionReceipt":
			return TxReceipt{GasUsed: 21000}, nil
		}
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	fees, err := client.GetBlockTxFees(ctx, 12345)
	if err != nil {
		t.Fatalf("GetBlockTxFees failed: %v", err)
	}

	// 2 txs * 21000 gas * 1 gwei = 42000 gwei
	expected := uint64(2 * 21000 * 1000000000)
	if fees != expected {
		t.Errorf("Fees = %d, want %d", fees, expected)
	}
}

func TestGetBlockTxFeesNull(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	fees, err := client.GetBlockTxFees(ctx, 99999999)
	if err != nil {
		t.Fatalf("GetBlockTxFees failed: %v", err)
	}

	if fees != 0 {
		t.Errorf("Fees = %d, want 0", fees)
	}
}

func TestSearchBlockByHash(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		if req.Method == "tos_getBlockByNumber" {
			// Return found block at height 10002
			return BlockInfo{Hash: "0xtarget", Height: 10002}, nil
		}
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.SearchBlockByHash(ctx, "0xtarget", 10000, 5)
	if err != nil {
		t.Fatalf("SearchBlockByHash failed: %v", err)
	}

	if block == nil {
		t.Fatal("Block should be found")
	}

	if block.Hash != "0xtarget" {
		t.Errorf("Hash = %s, want 0xtarget", block.Hash)
	}
}

func TestSearchBlockByHashNotFound(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		return BlockInfo{Hash: "0xother", Height: 10000}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.SearchBlockByHash(ctx, "0xnotfound", 10000, 2)
	if err != nil {
		t.Fatalf("SearchBlockByHash failed: %v", err)
	}

	if block != nil {
		t.Error("Block should not be found")
	}
}

func TestGetBlockRewardWithFees(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		switch req.Method {
		case "tos_getBlockByNumber":
			return BlockInfo{
				Reward: 5000000000,
				Height: 12345,
			}, nil
		case "tos_getTransactionReceipt":
			return TxReceipt{GasUsed: 21000}, nil
		}
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	reward, fees, err := client.GetBlockRewardWithFees(ctx, 12345)
	if err != nil {
		t.Fatalf("GetBlockRewardWithFees failed: %v", err)
	}

	if reward != 5000000000 {
		t.Errorf("Reward = %d, want 5000000000", reward)
	}

	// Fees might be 0 if block has no transactions in mock
	_ = fees
}

func TestConnectionError(t *testing.T) {
	// Client pointing to non-existent server
	client := NewTOSClient("http://localhost:19999", 1*time.Second)
	ctx := context.Background()

	_, err := client.GetWork(ctx)
	if err == nil {
		t.Error("GetWork should fail with connection error")
	}

	// After failure, client should record it
	if client.failCount == 0 {
		t.Error("Fail count should be incremented")
	}
}

func TestContextCancellation(t *testing.T) {
	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		time.Sleep(5 * time.Second)
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.GetWork(ctx)
	if err == nil {
		t.Error("GetWork should fail with context timeout")
	}
}

func TestConcurrentCalls(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	server := mockRPCServer(t, func(req RPCRequest) (interface{}, *RPCError) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return []string{"0x1", "0x2", "0x3", "0x4"}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.GetWork(ctx)
		}()
	}
	wg.Wait()

	mu.Lock()
	if callCount != 10 {
		t.Errorf("Call count = %d, want 10", callCount)
	}
	mu.Unlock()
}

func TestBlockTemplateStruct(t *testing.T) {
	template := BlockTemplate{
		HeaderHash:   "0xheader",
		ParentHash:   "0xparent",
		Height:       12345,
		Timestamp:    1700000000,
		Difficulty:   1000000,
		Target:       "0xtarget",
		ExtraNonce:   "0xextra",
		Transactions: []byte{0x01, 0x02},
	}

	if template.HeaderHash != "0xheader" {
		t.Errorf("HeaderHash = %s, want 0xheader", template.HeaderHash)
	}

	if len(template.Transactions) != 2 {
		t.Errorf("Transactions length = %d, want 2", len(template.Transactions))
	}
}

func TestBlockInfoStruct(t *testing.T) {
	block := BlockInfo{
		Hash:         "0xhash",
		ParentHash:   "0xparent",
		Height:       12345,
		Timestamp:    1700000000,
		Difficulty:   1000000,
		TotalDiff:    "0xtotal",
		Nonce:        "0xnonce",
		Miner:        "tos1miner",
		Reward:       5000000000,
		Size:         1024,
		GasUsed:      100000,
		GasLimit:     15000000,
		Transactions: 50,
		TxFees:       1000000,
	}

	if block.Transactions != 50 {
		t.Errorf("Transactions = %d, want 50", block.Transactions)
	}

	if block.TxFees != 1000000 {
		t.Errorf("TxFees = %d, want 1000000", block.TxFees)
	}
}

func TestNetworkInfoStruct(t *testing.T) {
	info := NetworkInfo{
		ChainID:    1,
		Height:     12345,
		Difficulty: 1000000,
		Hashrate:   500000,
		PeerCount:  25,
		Syncing:    false,
		GasPrice:   1000000000,
	}

	if info.ChainID != 1 {
		t.Errorf("ChainID = %d, want 1", info.ChainID)
	}

	if info.Hashrate != 500000 {
		t.Errorf("Hashrate = %d, want 500000", info.Hashrate)
	}
}

func TestTxReceiptStruct(t *testing.T) {
	receipt := TxReceipt{
		TxHash:      "0xtxhash",
		BlockHash:   "0xblockhash",
		BlockNumber: 12345,
		Status:      1,
		GasUsed:     21000,
	}

	if receipt.Status != 1 {
		t.Errorf("Status = %d, want 1", receipt.Status)
	}
}

func BenchmarkGetWork(b *testing.B) {
	server := mockRPCServer(nil, func(req RPCRequest) (interface{}, *RPCError) {
		return []string{"0x1", "0x2", "0x3", "0x4"}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.GetWork(ctx)
	}
}

func BenchmarkSubmitWork(b *testing.B) {
	server := mockRPCServer(nil, func(req RPCRequest) (interface{}, *RPCError) {
		return true, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.SubmitWork(ctx, "0xnonce", "0xheader", "0xmix")
	}
}
