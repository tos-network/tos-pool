package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTOSNativeClient(t *testing.T) {
	client := NewTOSNativeClient("http://localhost:8080", 10*time.Second, "tos1testaddress")

	if client == nil {
		t.Fatal("NewTOSNativeClient returned nil")
	}

	if client.url != "http://localhost:8080" {
		t.Errorf("url = %s, want http://localhost:8080", client.url)
	}

	if client.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", client.timeout)
	}

	if client.minerAddress != "tos1testaddress" {
		t.Errorf("minerAddress = %s, want tos1testaddress", client.minerAddress)
	}

	if !client.healthy {
		t.Error("client should be healthy initially")
	}
}

func TestTOSNativeClientSetMinerAddress(t *testing.T) {
	client := NewTOSNativeClient("http://localhost:8080", 10*time.Second, "tos1initial")
	client.SetMinerAddress("tos1newaddress")

	if client.minerAddress != "tos1newaddress" {
		t.Errorf("minerAddress = %s, want tos1newaddress", client.minerAddress)
	}
}

func TestTOSNativeClientIsHealthy(t *testing.T) {
	client := NewTOSNativeClient("http://localhost:8080", 10*time.Second, "tos1test")

	if !client.IsHealthy() {
		t.Error("client should be healthy initially")
	}

	// Simulate failures
	for i := 0; i < 3; i++ {
		client.recordFailure()
	}

	if client.IsHealthy() {
		t.Error("client should be unhealthy after 3 failures")
	}

	// Simulate success
	client.recordSuccess()
	if !client.IsHealthy() {
		t.Error("client should be healthy after success")
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

// Mock server for integration-style tests
func mockNativeRPCServer(t *testing.T, handler func(req NativeRPCRequest) (interface{}, *RPCError)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req NativeRPCRequest
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

func TestGetWorkNative(t *testing.T) {
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

	client := NewTOSNativeClient(server.URL, 10*time.Second, "tos1testminer")
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

func TestGetBlockByNumberNative(t *testing.T) {
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

	client := NewTOSNativeClient(server.URL, 10*time.Second, "tos1test")
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
}

func TestGetBalanceNative(t *testing.T) {
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

	client := NewTOSNativeClient(server.URL, 10*time.Second, "tos1test")
	ctx := context.Background()

	balance, err := client.GetBalance(ctx, "tos1testaddress")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	if balance != 100000000000 {
		t.Errorf("Balance = %d, want 100000000000", balance)
	}
}

func TestGetNetworkInfoNative(t *testing.T) {
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

	client := NewTOSNativeClient(server.URL, 10*time.Second, "tos1test")
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

func TestSubmitBlockNative(t *testing.T) {
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

	client := NewTOSNativeClient(server.URL, 10*time.Second, "tos1test")
	ctx := context.Background()

	success, err := client.SubmitBlock(ctx, "blocktemplatedata", "minerworkdata")
	if err != nil {
		t.Fatalf("SubmitBlock failed: %v", err)
	}

	if !success {
		t.Error("SubmitBlock should return true on success")
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
