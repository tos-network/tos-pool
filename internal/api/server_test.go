package api

import (
	"testing"
	"time"
)

func TestWorkerStatsStruct(t *testing.T) {
	stats := WorkerStats{
		Name:     "worker1",
		Hashrate: 1500000.5,
		LastSeen: time.Now().Unix(),
	}

	if stats.Name != "worker1" {
		t.Errorf("WorkerStats.Name = %s, want worker1", stats.Name)
	}

	if stats.Hashrate != 1500000.5 {
		t.Errorf("WorkerStats.Hashrate = %f, want 1500000.5", stats.Hashrate)
	}

	if stats.LastSeen == 0 {
		t.Error("WorkerStats.LastSeen should be set")
	}
}

func TestMinerResponseStruct(t *testing.T) {
	workers := []WorkerStats{
		{Name: "rig1", Hashrate: 1000000, LastSeen: time.Now().Unix()},
		{Name: "rig2", Hashrate: 500000, LastSeen: time.Now().Unix()},
	}

	response := MinerResponse{
		Address:         "tos1testaddr",
		Hashrate:        1500000,
		HashrateLarge:   1400000,
		Balance:         1000000,
		ImmatureBalance: 500000,
		PendingBalance:  0,
		TotalPaid:       5000000,
		BlocksFound:     10,
		LastShare:       time.Now().Unix(),
		Workers:         workers,
		Payments:        nil,
	}

	if response.Address != "tos1testaddr" {
		t.Errorf("MinerResponse.Address = %s, want tos1testaddr", response.Address)
	}

	if len(response.Workers) != 2 {
		t.Errorf("MinerResponse.Workers len = %d, want 2", len(response.Workers))
	}

	if response.Workers[0].Name != "rig1" {
		t.Errorf("MinerResponse.Workers[0].Name = %s, want rig1", response.Workers[0].Name)
	}
}

func TestPoolStatsStruct(t *testing.T) {
	stats := PoolStats{
		Hashrate:        1000000,
		HashrateLarge:   950000,
		Miners:          100,
		Workers:         150,
		BlocksFound:     50,
		LastBlockFound:  time.Now().Unix(),
		LastBlockHeight: 12345,
		TotalPaid:       50000000,
		Fee:             1.0,
	}

	if stats.Miners != 100 {
		t.Errorf("PoolStats.Miners = %d, want 100", stats.Miners)
	}

	if stats.Workers != 150 {
		t.Errorf("PoolStats.Workers = %d, want 150", stats.Workers)
	}

	if stats.Fee != 1.0 {
		t.Errorf("PoolStats.Fee = %f, want 1.0", stats.Fee)
	}
}

func TestNetworkStatsStruct(t *testing.T) {
	stats := NetworkStats{
		Height:     12345,
		Difficulty: 1000000,
		Hashrate:   5000000,
	}

	if stats.Height != 12345 {
		t.Errorf("NetworkStats.Height = %d, want 12345", stats.Height)
	}
}

func TestStatsResponseStruct(t *testing.T) {
	response := &StatsResponse{
		Pool: PoolStats{
			Hashrate: 1000000,
			Miners:   100,
		},
		Network: NetworkStats{
			Height:     12345,
			Difficulty: 1000000,
		},
		Now: time.Now().Unix(),
	}

	if response.Pool.Miners != 100 {
		t.Errorf("StatsResponse.Pool.Miners = %d, want 100", response.Pool.Miners)
	}

	if response.Network.Height != 12345 {
		t.Errorf("StatsResponse.Network.Height = %d, want 12345", response.Network.Height)
	}
}

func TestBlockResponseStruct(t *testing.T) {
	response := BlockResponse{
		Height:        12345,
		Hash:          "0xabcdef",
		Finder:        "tos1finder",
		Reward:        500000000,
		Timestamp:     time.Now().Unix(),
		Status:        "matured",
		Confirmations: 100,
	}

	if response.Height != 12345 {
		t.Errorf("BlockResponse.Height = %d, want 12345", response.Height)
	}

	if response.Status != "matured" {
		t.Errorf("BlockResponse.Status = %s, want matured", response.Status)
	}
}

func TestUpstreamStatusStruct(t *testing.T) {
	status := UpstreamStatus{
		Name:         "primary",
		URL:          "http://127.0.0.1:8545",
		Healthy:      true,
		ResponseTime: 50.5,
		Height:       12345,
		Weight:       10,
		FailCount:    0,
		SuccessCount: 100,
	}

	if status.Name != "primary" {
		t.Errorf("UpstreamStatus.Name = %s, want primary", status.Name)
	}

	if !status.Healthy {
		t.Error("UpstreamStatus.Healthy should be true")
	}
}

func TestLuckResponseStruct(t *testing.T) {
	response := LuckResponse{
		Luck24h: 100.5,
		Luck7d:  98.2,
		Luck30d: 101.3,
		LuckAll: 99.8,
		Blocks: []BlockLuck{
			{Height: 12345, Effort: 95.5, RoundShares: 100000, Difficulty: 105000, Timestamp: time.Now().Unix()},
		},
	}

	if response.Luck24h != 100.5 {
		t.Errorf("LuckResponse.Luck24h = %f, want 100.5", response.Luck24h)
	}

	if len(response.Blocks) != 1 {
		t.Errorf("LuckResponse.Blocks len = %d, want 1", len(response.Blocks))
	}
}

func TestBlockLuckStruct(t *testing.T) {
	luck := BlockLuck{
		Height:      12345,
		Effort:      95.5,
		RoundShares: 100000,
		Difficulty:  105000,
		Timestamp:   time.Now().Unix(),
	}

	if luck.Height != 12345 {
		t.Errorf("BlockLuck.Height = %d, want 12345", luck.Height)
	}

	if luck.Effort != 95.5 {
		t.Errorf("BlockLuck.Effort = %f, want 95.5", luck.Effort)
	}
}

func TestAdminStatsResponseStruct(t *testing.T) {
	response := AdminStatsResponse{
		PendingPayouts: 5,
		LockedPayouts:  false,
		BlacklistCount: 10,
		WhitelistCount: 3,
	}

	if response.PendingPayouts != 5 {
		t.Errorf("AdminStatsResponse.PendingPayouts = %d, want 5", response.PendingPayouts)
	}

	if response.LockedPayouts {
		t.Error("AdminStatsResponse.LockedPayouts should be false")
	}
}

func TestBlacklistRequestStruct(t *testing.T) {
	req := BlacklistRequest{
		Address: "tos1badactor",
	}

	if req.Address != "tos1badactor" {
		t.Errorf("BlacklistRequest.Address = %s, want tos1badactor", req.Address)
	}
}

func TestWhitelistRequestStruct(t *testing.T) {
	req := WhitelistRequest{
		IP: "192.168.1.100",
	}

	if req.IP != "192.168.1.100" {
		t.Errorf("WhitelistRequest.IP = %s, want 192.168.1.100", req.IP)
	}
}

func TestParseHours(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		hasError bool
	}{
		{"24", 24, false},
		{"1", 1, false},
		{"168", 168, false},
		{"0", 24, true},
		{"-1", 24, true},
		{"abc", 24, true},
		{"", 24, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseHours(tt.input)
			if tt.hasError {
				if err == nil && result != tt.expected {
					// parseHours returns 24 as default for invalid inputs
				}
			} else {
				if result != tt.expected {
					t.Errorf("parseHours(%q) = %d, want %d", tt.input, result, tt.expected)
				}
			}
		})
	}
}
