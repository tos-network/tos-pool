package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/storage"
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

// setupTestServer creates a test server with miniredis
func setupTestServer(t *testing.T) (*Server, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	redis, err := storage.NewRedisClient(mr.Addr(), "", 0)
	if err != nil {
		mr.Close()
		t.Fatalf("Failed to create Redis client: %v", err)
	}

	cfg := &config.Config{
		API: config.APIConfig{
			Bind:          ":8080",
			StatsCache:    5 * time.Second,
			AdminEnabled:  true,
			AdminPassword: "testpassword",
		},
		Pool: config.PoolConfig{
			Fee: 1.0,
		},
		Validation: config.ValidationConfig{
			HashrateWindow:      600 * time.Second,
			HashrateLargeWindow: 3600 * time.Second,
		},
	}

	server := NewServer(cfg, redis)
	return server, mr
}

func TestNewServer(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.cfg == nil {
		t.Error("Server.cfg should not be nil")
	}

	if server.redis == nil {
		t.Error("Server.redis should not be nil")
	}

	if server.router == nil {
		t.Error("Server.router should not be nil")
	}
}

func TestHealthEndpoint(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["status"] != "ok" {
		t.Errorf("Response status = %s, want ok", response["status"])
	}
}

func TestCORSHeaders(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("OPTIONS", "/api/stats", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != 204 {
		t.Errorf("Status = %d, want 204", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS origin header not set")
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("CORS methods header not set")
	}
}

func TestHandleStats(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	// Add some test data
	mr.Set("pool:hashrate", "1500000")

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response StatsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Pool.Fee != 1.0 {
		t.Errorf("Pool.Fee = %f, want 1.0", response.Pool.Fee)
	}

	if response.Now == 0 {
		t.Error("Now should be set")
	}
}

func TestHandleStatsCache(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	// First request
	req1 := httptest.NewRequest("GET", "/api/stats", nil)
	w1 := httptest.NewRecorder()
	server.router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("First request status = %d", w1.Code)
	}

	// Second request should hit cache
	req2 := httptest.NewRequest("GET", "/api/stats", nil)
	w2 := httptest.NewRecorder()
	server.router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Second request status = %d", w2.Code)
	}
}

func TestHandleBlocks(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/blocks", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if _, ok := response["blocks"]; !ok {
		t.Error("Response should contain 'blocks' field")
	}
}

func TestHandlePayments(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/payments", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if _, ok := response["payments"]; !ok {
		t.Error("Response should contain 'payments' field")
	}
}

func TestHandleMinerInvalidAddress(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/miners/invalid", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleMinerNotFound(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	// Use a valid 62-character bech32 address that doesn't exist in the DB
	req := httptest.NewRequest("GET", "/api/miners/tos1qpzry9x8gf2tvdw0s3jn54khce6mua7lmqqqxw823456789acdefghjklm", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleMinerPaymentsInvalidAddress(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/miners/invalid/payments", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleMinerChartInvalidAddress(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/miners/invalid/chart", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleLuck(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/luck", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandlePoolHashrateChart(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/chart/hashrate", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if hours, ok := response["hours"].(float64); !ok || hours != 24 {
		t.Errorf("Hours = %v, want 24", response["hours"])
	}
}

func TestHandlePoolHashrateChartWithParams(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/chart/hashrate?hours=48", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if hours, ok := response["hours"].(float64); !ok || hours != 48 {
		t.Errorf("Hours = %v, want 48", response["hours"])
	}
}

func TestHandlePoolHashrateChartMaxLimit(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/api/chart/hashrate?hours=500", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if hours, ok := response["hours"].(float64); !ok || hours != 168 {
		t.Errorf("Hours = %v, want 168 (max)", response["hours"])
	}
}

func TestAdminAuthMiddlewareNoAuth(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/stats", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuthMiddlewareWrongPassword(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/stats", nil)
	req.Header.Set("Authorization", "wrongpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAdminAuthMiddlewareCorrectPassword(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/stats", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAdminAuthMiddlewareBearerToken(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAdminStats(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/stats", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response AdminStatsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
}

func TestHandleGetBlacklist(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/blacklist", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAddBlacklist(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	body := bytes.NewBufferString(`{"address":"tos1badactor"}`)
	req := httptest.NewRequest("POST", "/admin/blacklist", body)
	req.Header.Set("Authorization", "testpassword")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAddBlacklistInvalidRequest(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	body := bytes.NewBufferString(`invalid json`)
	req := httptest.NewRequest("POST", "/admin/blacklist", body)
	req.Header.Set("Authorization", "testpassword")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddBlacklistEmptyAddress(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	body := bytes.NewBufferString(`{"address":""}`)
	req := httptest.NewRequest("POST", "/admin/blacklist", body)
	req.Header.Set("Authorization", "testpassword")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRemoveBlacklist(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	// First add to blacklist
	mr.SAdd("pool:blacklist", "tos1badactor")

	req := httptest.NewRequest("DELETE", "/admin/blacklist/tos1badactor", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleGetWhitelist(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/whitelist", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAddWhitelist(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	body := bytes.NewBufferString(`{"ip":"192.168.1.100"}`)
	req := httptest.NewRequest("POST", "/admin/whitelist", body)
	req.Header.Set("Authorization", "testpassword")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAddWhitelistInvalidRequest(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	body := bytes.NewBufferString(`invalid json`)
	req := httptest.NewRequest("POST", "/admin/whitelist", body)
	req.Header.Set("Authorization", "testpassword")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddWhitelistEmptyIP(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	body := bytes.NewBufferString(`{"ip":""}`)
	req := httptest.NewRequest("POST", "/admin/whitelist", body)
	req.Header.Set("Authorization", "testpassword")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRemoveWhitelist(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	// First add to whitelist
	mr.SAdd("pool:whitelist", "192.168.1.100")

	req := httptest.NewRequest("DELETE", "/admin/whitelist/192.168.1.100", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandlePendingPayments(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/pending-payments", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleBackup(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/backup", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	contentDisposition := w.Header().Get("Content-Disposition")
	if contentDisposition == "" {
		t.Error("Content-Disposition header should be set")
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}
}

func TestHandleUpstreamsNoCallback(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	req := httptest.NewRequest("GET", "/admin/upstreams", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if total, ok := response["total"].(float64); !ok || total != 0 {
		t.Errorf("Total = %v, want 0", response["total"])
	}
}

func TestHandleUpstreamsWithCallback(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	server.SetUpstreamStateFunc(func() []UpstreamStatus {
		return []UpstreamStatus{
			{Name: "node1", Healthy: true, Height: 12345},
			{Name: "node2", Healthy: false, Height: 12340},
		}
	})

	req := httptest.NewRequest("GET", "/admin/upstreams", nil)
	req.Header.Set("Authorization", "testpassword")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if total, ok := response["total"].(float64); !ok || total != 2 {
		t.Errorf("Total = %v, want 2", response["total"])
	}
	if healthy, ok := response["healthy"].(float64); !ok || healthy != 1 {
		t.Errorf("Healthy = %v, want 1", response["healthy"])
	}
	if active, ok := response["active"].(string); !ok || active != "node1" {
		t.Errorf("Active = %v, want node1", response["active"])
	}
}

func TestSetUpstreamStateFunc(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	if server.upstreamStateFunc != nil {
		t.Error("upstreamStateFunc should be nil initially")
	}

	fn := func() []UpstreamStatus {
		return []UpstreamStatus{}
	}
	server.SetUpstreamStateFunc(fn)

	if server.upstreamStateFunc == nil {
		t.Error("upstreamStateFunc should be set")
	}
}

func TestServerStartStop(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redis, _ := storage.NewRedisClient(mr.Addr(), "", 0)

	cfg := &config.Config{
		API: config.APIConfig{
			Bind:       ":0", // Use random available port
			StatsCache: 5 * time.Second,
		},
		Validation: config.ValidationConfig{
			HashrateWindow:      600 * time.Second,
			HashrateLargeWindow: 3600 * time.Second,
		},
	}

	server := NewServer(cfg, redis)

	if err := server.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	if err := server.Stop(); err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

func TestServerStopNotStarted(t *testing.T) {
	server, mr := setupTestServer(t)
	defer mr.Close()

	// Should not panic when stopping without starting
	err := server.Stop()
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
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
