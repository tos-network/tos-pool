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

// mockNativeRPCServer creates a test server that responds to TOS native API calls
func mockNativeRPCServer(t *testing.T, handler func(req NativeRPCRequest) (interface{}, *RPCError)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			if t != nil {
				t.Errorf("Expected POST, got %s", r.Method)
			}
		}

		var req NativeRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if t != nil {
				t.Errorf("Failed to decode request: %v", err)
			}
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
	client := NewTOSClient("http://localhost:8080", 30*time.Second)

	if client == nil {
		t.Fatal("NewTOSClient returned nil")
	}

	if client.url != "http://localhost:8080" {
		t.Errorf("url = %s, want http://localhost:8080", client.url)
	}

	if client.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", client.timeout)
	}

	if !client.healthy {
		t.Error("Client should be healthy initially")
	}
}

func TestSetMinerAddress(t *testing.T) {
	client := NewTOSClient("http://localhost:8080", 30*time.Second)
	client.SetMinerAddress("tos1testaddress")

	if client.minerAddress != "tos1testaddress" {
		t.Errorf("minerAddress = %s, want tos1testaddress", client.minerAddress)
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
	client := NewTOSClient("http://localhost:8080", 30*time.Second)

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

func TestParseDifficulty(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"1000000", 1000000},
		{"0", 0},
		{"12345678901234567890", 12345678901234567890},
		{"invalid", 0},
	}

	for _, tt := range tests {
		result := parseDifficulty(tt.input)
		if result != tt.expected {
			t.Errorf("parseDifficulty(%s) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestDifficultyToTarget(t *testing.T) {
	tests := []struct {
		difficulty string
		wantLen    int
	}{
		{"1000000", 64},
		{"1", 64},
		{"0", 64},
	}

	for _, tt := range tests {
		result := difficultyToTarget(tt.difficulty)
		if len(result) != tt.wantLen {
			t.Errorf("difficultyToTarget(%s) length = %d, want %d", tt.difficulty, len(result), tt.wantLen)
		}
	}

	// Test that higher difficulty produces lower target
	target1 := difficultyToTarget("1000")
	target2 := difficultyToTarget("2000")
	if target1 <= target2 {
		t.Error("higher difficulty should produce lower target")
	}
}

func TestGetWork(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		if req.Method != "get_block_template" {
			t.Errorf("Method = %s, want get_block_template", req.Method)
		}

		// Verify params is an object with address
		params, ok := req.Params.(map[string]interface{})
		if !ok {
			t.Error("Params should be an object")
		}
		if _, exists := params["address"]; !exists {
			t.Error("Params should contain address")
		}

		return GetBlockTemplateResult{
			Template:   "deadbeef1234567890",
			Algorithm:  "tos/v3",
			Height:     12345,
			TopoHeight: 12345,
			Difficulty: "1000000",
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	client.SetMinerAddress("tos1testminer")
	ctx := context.Background()

	work, err := client.GetWork(ctx)
	if err != nil {
		t.Fatalf("GetWork failed: %v", err)
	}

	if work.HeaderHash != "deadbeef1234567890" {
		t.Errorf("HeaderHash = %s, want deadbeef1234567890", work.HeaderHash)
	}

	if work.Height != 12345 {
		t.Errorf("Height = %d, want 12345", work.Height)
	}

	if work.Difficulty != 1000000 {
		t.Errorf("Difficulty = %d, want 1000000", work.Difficulty)
	}

	// Target should be 64 hex chars
	if len(work.Target) != 64 {
		t.Errorf("Target length = %d, want 64", len(work.Target))
	}
}

func TestGetWorkRPCError(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		return nil, &RPCError{Code: -32000, Message: "No work available"}
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	client.SetMinerAddress("tos1test")
	ctx := context.Background()

	_, err := client.GetWork(ctx)
	if err == nil {
		t.Error("GetWork should fail with RPC error")
	}
}

func TestSubmitWork(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		if req.Method != "submit_block" {
			t.Errorf("Method = %s, want submit_block", req.Method)
		}
		return true, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	success, err := client.SubmitWork(ctx, "nonce123", "template_data", "")
	if err != nil {
		t.Fatalf("SubmitWork failed: %v", err)
	}

	if !success {
		t.Error("SubmitWork should return true")
	}
}

func TestSubmitBlock(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		if req.Method != "submit_block" {
			t.Errorf("Method = %s, want submit_block", req.Method)
		}

		params, ok := req.Params.(map[string]interface{})
		if !ok {
			t.Error("Params should be an object")
		}
		if _, exists := params["block_template"]; !exists {
			t.Error("Params should contain block_template")
		}

		return true, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	success, err := client.SubmitBlock(ctx, "blocktemplatedata", "minerworkdata")
	if err != nil {
		t.Fatalf("SubmitBlock failed: %v", err)
	}

	if !success {
		t.Error("SubmitBlock should return true on success")
	}
}

func TestGetBlockByNumber(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		if req.Method != "get_block_at_topoheight" {
			t.Errorf("Method = %s, want get_block_at_topoheight", req.Method)
		}

		params, ok := req.Params.(map[string]interface{})
		if !ok {
			t.Error("Params should be an object")
		}
		if _, exists := params["topoheight"]; !exists {
			t.Error("Params should contain topoheight")
		}

		return RPCBlockResponse{
			Hash:        "blockhash",
			TopoHeight:  12345,
			Height:      12345,
			Tips:        []string{"parent1"},
			Timestamp:   1734567890000,
			Difficulty:  "1000000",
			MinerReward: 90000000,
			TotalFees:   5000,
			Miner:       "tos1miner",
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.GetBlockByNumber(ctx, 12345)
	if err != nil {
		t.Fatalf("GetBlockByNumber failed: %v", err)
	}

	if block.Hash != "blockhash" {
		t.Errorf("Hash = %s, want blockhash", block.Hash)
	}

	if block.Height != 12345 {
		t.Errorf("Height = %d, want 12345", block.Height)
	}

	if block.Miner != "tos1miner" {
		t.Errorf("Miner = %s, want tos1miner", block.Miner)
	}
}

func TestGetBlockByNumberNull(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
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
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		if req.Method != "get_block_by_hash" {
			t.Errorf("Method = %s, want get_block_by_hash", req.Method)
		}

		return RPCBlockResponse{
			Hash:   "blockhash",
			Height: 12345,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.GetBlockByHash(ctx, "blockhash")
	if err != nil {
		t.Fatalf("GetBlockByHash failed: %v", err)
	}

	if block.Hash != "blockhash" {
		t.Errorf("Hash = %s, want blockhash", block.Hash)
	}
}

func TestGetLatestBlock(t *testing.T) {
	callCount := 0
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		callCount++
		switch req.Method {
		case "get_info":
			return GetInfoResult{
				TopoHeight: 99999,
			}, nil
		case "get_block_at_topoheight":
			return RPCBlockResponse{
				Hash:   "latesthash",
				Height: 99999,
			}, nil
		}
		return nil, &RPCError{Code: -32601, Message: "Method not found"}
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
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		switch req.Method {
		case "get_info":
			return GetInfoResult{
				Height:           12345,
				TopoHeight:       12345,
				StableHeight:     12337,
				StableTopoHeight: 12337,
				TopBlockHash:     "tophash",
				Difficulty:       "1000000",
				BlockTimeTarget:  3000,
				AverageBlockTime: 3000,
				BlockReward:      100000000,
				DevReward:        10000000,
				MinerReward:      90000000,
				MempoolSize:      5,
				Version:          "1.0.0",
				Network:          "mainnet",
			}, nil
		case "p2p_status":
			return P2pStatusResult{
				PeerCount:        10,
				MaxPeers:         32,
				OurTopoHeight:    12345,
				BestTopoHeight:   12345,
				MedianTopoHeight: 12345,
			}, nil
		default:
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

	if info.Height != 12345 {
		t.Errorf("Height = %d, want 12345", info.Height)
	}

	if info.PeerCount != 10 {
		t.Errorf("PeerCount = %d, want 10", info.PeerCount)
	}

	if info.Syncing {
		t.Error("Syncing should be false when our_topoheight == best_topoheight")
	}

	if info.GasPrice != 0 {
		t.Errorf("GasPrice = %d, want 0 (TOS has no gas)", info.GasPrice)
	}
}

func TestGetNetworkInfoSyncing(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		switch req.Method {
		case "get_info":
			return GetInfoResult{
				TopoHeight: 12345,
				Difficulty: "1000000",
			}, nil
		case "p2p_status":
			return P2pStatusResult{
				PeerCount:      5,
				OurTopoHeight:  12345,
				BestTopoHeight: 12500, // Behind
			}, nil
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
		t.Error("Syncing should be true when our_topoheight < best_topoheight")
	}
}

func TestGetBalance(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		if req.Method != "get_balance" {
			t.Errorf("Method = %s, want get_balance", req.Method)
		}

		params, ok := req.Params.(map[string]interface{})
		if !ok {
			t.Error("Params should be an object")
		}
		if _, exists := params["asset"]; !exists {
			t.Error("Params should contain asset (native TOS hash)")
		}

		return GetBalanceResult{
			Balance:    100000000000,
			TopoHeight: 12345,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	balance, err := client.GetBalance(ctx, "tos1testaddress")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance != 100000000000 {
		t.Errorf("Balance = %d, want 100000000000", balance)
	}
}

func TestGetBlockTxFees(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		return RPCBlockResponse{
			Hash:      "blockhash",
			Height:   12345,
			TotalFees: 5000,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	fees, err := client.GetBlockTxFees(ctx, 12345)
	if err != nil {
		t.Fatalf("GetBlockTxFees failed: %v", err)
	}

	if fees != 5000 {
		t.Errorf("Fees = %d, want 5000", fees)
	}
}

func TestGetBlockRewardWithFees(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		return RPCBlockResponse{
			Height:      12345,
			MinerReward: 90000000,
			TotalFees:   5000,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	reward, fees, err := client.GetBlockRewardWithFees(ctx, 12345)
	if err != nil {
		t.Fatalf("GetBlockRewardWithFees failed: %v", err)
	}

	if reward != 90000000 {
		t.Errorf("Reward = %d, want 90000000", reward)
	}

	if fees != 5000 {
		t.Errorf("Fees = %d, want 5000", fees)
	}
}

func TestSearchBlockByHash(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		if req.Method == "get_block_by_hash" {
			return RPCBlockResponse{Hash: "target", Height: 10002}, nil
		}
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	block, err := client.SearchBlockByHash(ctx, "target", 10000, 5)
	if err != nil {
		t.Fatalf("SearchBlockByHash failed: %v", err)
	}

	if block == nil {
		t.Fatal("Block should be found")
	}

	if block.Hash != "target" {
		t.Errorf("Hash = %s, want target", block.Hash)
	}
}

func TestConvertBlockResponse(t *testing.T) {
	native := &RPCBlockResponse{
		Hash:                 "blockhash123",
		TopoHeight:           12345,
		BlockType:            "Normal",
		Difficulty:           "1000000",
		Supply:               100000000000000,
		Reward:               100000000,
		MinerReward:          90000000,
		DevReward:            10000000,
		CumulativeDifficulty: "12345000000",
		TotalFees:            5000,
		TotalSizeInBytes:     1024,
		Version:              1,
		Tips:                 []string{"parent1", "parent2"},
		Timestamp:            1734567890000, // milliseconds
		Height:               12345,
		Nonce:                123456789,
		ExtraNonce:           "0000000000000000",
		Miner:                "tos1miner",
		TxsHashes:            []string{"tx1", "tx2", "tx3"},
	}

	result := convertBlockResponse(native)

	if result.Hash != "blockhash123" {
		t.Errorf("Hash = %s, want blockhash123", result.Hash)
	}

	if result.ParentHash != "parent1" {
		t.Errorf("ParentHash = %s, want parent1", result.ParentHash)
	}

	if result.Height != 12345 {
		t.Errorf("Height = %d, want 12345", result.Height)
	}

	if result.Timestamp != 1734567890 {
		t.Errorf("Timestamp = %d, want 1734567890 (converted from ms)", result.Timestamp)
	}

	if result.Miner != "tos1miner" {
		t.Errorf("Miner = %s, want tos1miner", result.Miner)
	}

	if result.Reward != 90000000 {
		t.Errorf("Reward = %d, want 90000000 (miner reward)", result.Reward)
	}

	if result.TxFees != 5000 {
		t.Errorf("TxFees = %d, want 5000", result.TxFees)
	}

	if result.Transactions != 3 {
		t.Errorf("Transactions = %d, want 3", result.Transactions)
	}

	if result.GasUsed != 0 {
		t.Errorf("GasUsed = %d, want 0 (TOS has no gas)", result.GasUsed)
	}
}

func TestConvertBlockResponseEmptyTips(t *testing.T) {
	native := &RPCBlockResponse{
		Hash: "blockhash",
		Tips: []string{},
	}

	result := convertBlockResponse(native)

	if result.ParentHash != "" {
		t.Errorf("ParentHash = %s, want empty string for no tips", result.ParentHash)
	}
}

func TestNativeAssetHash(t *testing.T) {
	if len(NativeAssetHash) != 64 {
		t.Errorf("NativeAssetHash length = %d, want 64", len(NativeAssetHash))
	}

	// Should be all zeros
	for _, c := range NativeAssetHash {
		if c != '0' {
			t.Errorf("NativeAssetHash should be all zeros, got %s", NativeAssetHash)
			break
		}
	}
}

func TestEstimateGas(t *testing.T) {
	client := NewTOSClient("http://localhost:8080", 30*time.Second)
	ctx := context.Background()

	gas, err := client.EstimateGas(ctx, "from", "to", 1000)
	if err != nil {
		t.Fatalf("EstimateGas failed: %v", err)
	}

	if gas != 0 {
		t.Errorf("Gas = %d, want 0 (TOS has no gas)", gas)
	}
}

func TestGetGasPrice(t *testing.T) {
	client := NewTOSClient("http://localhost:8080", 30*time.Second)
	ctx := context.Background()

	price, err := client.GetGasPrice(ctx)
	if err != nil {
		t.Fatalf("GetGasPrice failed: %v", err)
	}

	if price != 0 {
		t.Errorf("GasPrice = %d, want 0 (TOS has no gas)", price)
	}
}

func TestSendTransaction(t *testing.T) {
	client := NewTOSClient("http://localhost:8080", 30*time.Second)
	ctx := context.Background()

	_, err := client.SendTransaction(ctx, "tos1recipient", 1000)
	if err == nil {
		t.Error("SendTransaction should return error (requires wallet integration)")
	}
}

func TestConnectionError(t *testing.T) {
	client := NewTOSClient("http://localhost:19999", 1*time.Second)
	client.SetMinerAddress("tos1test")
	ctx := context.Background()

	_, err := client.GetWork(ctx)
	if err == nil {
		t.Error("GetWork should fail with connection error")
	}

	if client.failCount == 0 {
		t.Error("Fail count should be incremented")
	}
}

func TestContextCancellation(t *testing.T) {
	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		time.Sleep(5 * time.Second)
		return nil, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	client.SetMinerAddress("tos1test")
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

	server := mockNativeRPCServer(t, func(req NativeRPCRequest) (interface{}, *RPCError) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return GetBlockTemplateResult{
			Template:   "test",
			Difficulty: "1000",
			Height:     1,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	client.SetMinerAddress("tos1test")
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
		HeaderHash:   "header",
		ParentHash:   "parent",
		Height:       12345,
		Timestamp:    1700000000,
		Difficulty:   1000000,
		Target:       "target",
		ExtraNonce:   "extra",
		Transactions: []byte{0x01, 0x02},
	}

	if template.HeaderHash != "header" {
		t.Errorf("HeaderHash = %s, want header", template.HeaderHash)
	}

	if len(template.Transactions) != 2 {
		t.Errorf("Transactions length = %d, want 2", len(template.Transactions))
	}
}

func TestBlockInfoStruct(t *testing.T) {
	block := BlockInfo{
		Hash:         "hash",
		ParentHash:   "parent",
		Height:       12345,
		Timestamp:    1700000000,
		Difficulty:   1000000,
		TotalDiff:    "total",
		Nonce:        "nonce",
		Miner:        "tos1miner",
		Reward:       5000000000,
		Size:         1024,
		GasUsed:      0,
		GasLimit:     0,
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
		GasPrice:   0,
	}

	if info.ChainID != 1 {
		t.Errorf("ChainID = %d, want 1", info.ChainID)
	}

	if info.GasPrice != 0 {
		t.Errorf("GasPrice = %d, want 0", info.GasPrice)
	}
}

func TestTxReceiptStruct(t *testing.T) {
	receipt := TxReceipt{
		TxHash:      "txhash",
		BlockHash:   "blockhash",
		BlockNumber: 12345,
		Status:      1,
		GasUsed:     0,
	}

	if receipt.Status != 1 {
		t.Errorf("Status = %d, want 1", receipt.Status)
	}
}

func TestP2pStatusResult(t *testing.T) {
	tag := "testnode"
	result := P2pStatusResult{
		PeerCount:        10,
		MaxPeers:         32,
		Tag:              &tag,
		OurTopoHeight:    12345,
		BestTopoHeight:   12346,
		MedianTopoHeight: 12345,
		PeerID:           1234567890,
	}

	if *result.Tag != "testnode" {
		t.Errorf("Tag = %s, want testnode", *result.Tag)
	}

	// Test syncing detection
	syncing := result.OurTopoHeight < result.BestTopoHeight
	if !syncing {
		t.Error("Should be syncing when our_topoheight < best_topoheight")
	}
}

func TestGetBlockTemplateResultStruct(t *testing.T) {
	result := GetBlockTemplateResult{
		Template:   "deadbeef",
		Algorithm:  "tos/v3",
		Height:     12345,
		TopoHeight: 12345,
		Difficulty: "1000000",
	}

	if result.Algorithm != "tos/v3" {
		t.Errorf("Algorithm = %s, want tos/v3", result.Algorithm)
	}
}

func BenchmarkGetWork(b *testing.B) {
	server := mockNativeRPCServer(nil, func(req NativeRPCRequest) (interface{}, *RPCError) {
		return GetBlockTemplateResult{
			Template:   "test",
			Difficulty: "1000",
			Height:     1,
		}, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	client.SetMinerAddress("tos1test")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.GetWork(ctx)
	}
}

func BenchmarkSubmitWork(b *testing.B) {
	server := mockNativeRPCServer(nil, func(req NativeRPCRequest) (interface{}, *RPCError) {
		return true, nil
	})
	defer server.Close()

	client := NewTOSClient(server.URL, 30*time.Second)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.SubmitWork(ctx, "nonce", "header", "mix")
	}
}
