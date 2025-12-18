// Package api provides the REST API server.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/storage"
	"github.com/tos-network/tos-pool/internal/util"
)

// UpstreamStateFunc is a callback to get upstream states
type UpstreamStateFunc func() []UpstreamStatus

// UpstreamStatus represents the status of an upstream node
type UpstreamStatus struct {
	Name         string  `json:"name"`
	URL          string  `json:"url"`
	Healthy      bool    `json:"healthy"`
	ResponseTime float64 `json:"response_time_ms"`
	Height       uint64  `json:"height"`
	Weight       int     `json:"weight"`
	FailCount    int32   `json:"fail_count"`
	SuccessCount int32   `json:"success_count"`
}

// Server is the API server
type Server struct {
	cfg    *config.Config
	redis  *storage.RedisClient
	router *gin.Engine
	server *http.Server

	// Cache
	statsCacheMu   sync.RWMutex
	statsCache     *StatsResponse
	statsCacheTime time.Time

	// Upstream state callback
	upstreamStateFunc UpstreamStateFunc
}

// StatsResponse is the /api/stats response
type StatsResponse struct {
	Pool    PoolStats    `json:"pool"`
	Network NetworkStats `json:"network"`
	Now     int64        `json:"now"`
}

// PoolStats contains pool statistics
type PoolStats struct {
	Hashrate        float64 `json:"hashrate"`
	HashrateLarge   float64 `json:"hashrate_large"`
	Miners          int64   `json:"miners"`
	Workers         int64   `json:"workers"`
	BlocksFound     uint64  `json:"blocks_found"`
	LastBlockFound  int64   `json:"last_block_found"`
	LastBlockHeight uint64  `json:"last_block_height"`
	TotalPaid       uint64  `json:"total_paid"`
	Fee             float64 `json:"fee"`
}

// NetworkStats contains network statistics
type NetworkStats struct {
	Height     uint64  `json:"height"`
	Difficulty uint64  `json:"difficulty"`
	Hashrate   float64 `json:"hashrate"`
}

// MinerResponse is the /api/miners/:address response
type MinerResponse struct {
	Address         string            `json:"address"`
	Hashrate        float64           `json:"hashrate"`
	HashrateLarge   float64           `json:"hashrate_large"`
	Balance         uint64            `json:"balance"`
	ImmatureBalance uint64            `json:"immature"`
	PendingBalance  uint64            `json:"pending"`
	TotalPaid       uint64            `json:"paid"`
	BlocksFound     uint64            `json:"blocks_found"`
	LastShare       int64             `json:"last_share"`
	Workers         []WorkerStats     `json:"workers"`
	Payments        []*storage.Payment `json:"payments"`
}

// WorkerStats contains worker statistics
type WorkerStats struct {
	Name     string  `json:"name"`
	Hashrate float64 `json:"hashrate"`
	LastSeen int64   `json:"last_seen"`
}

// BlockResponse is a block in the blocks list
type BlockResponse struct {
	Height        uint64  `json:"height"`
	Hash          string  `json:"hash"`
	Finder        string  `json:"finder"`
	Reward        uint64  `json:"reward"`
	Timestamp     int64   `json:"timestamp"`
	Status        string  `json:"status"`
	Confirmations uint64  `json:"confirmations"`
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, redis *storage.RedisClient) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	s := &Server{
		cfg:    cfg,
		redis:  redis,
		router: router,
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures API endpoints
func (s *Server) setupRoutes() {
	// CORS middleware
	s.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	api := s.router.Group("/api")
	{
		api.GET("/stats", s.handleStats)
		api.GET("/blocks", s.handleBlocks)
		api.GET("/payments", s.handlePayments)
		api.GET("/miners/:address", s.handleMiner)
		api.GET("/miners/:address/payments", s.handleMinerPayments)
		api.GET("/miners/:address/chart", s.handleMinerChart)
		api.GET("/luck", s.handleLuck)
		api.GET("/chart/hashrate", s.handlePoolHashrateChart)
	}

	// Admin API (password protected)
	if s.cfg.API.AdminEnabled && s.cfg.API.AdminPassword != "" {
		admin := s.router.Group("/admin")
		admin.Use(s.adminAuthMiddleware())
		{
			admin.GET("/stats", s.handleAdminStats)
			admin.GET("/blacklist", s.handleGetBlacklist)
			admin.POST("/blacklist", s.handleAddBlacklist)
			admin.DELETE("/blacklist/:address", s.handleRemoveBlacklist)
			admin.GET("/whitelist", s.handleGetWhitelist)
			admin.POST("/whitelist", s.handleAddWhitelist)
			admin.DELETE("/whitelist/:ip", s.handleRemoveWhitelist)
			admin.GET("/pending-payments", s.handlePendingPayments)
			admin.GET("/backup", s.handleBackup)
			admin.GET("/upstreams", s.handleUpstreams)
		}
	}

	// Health check
	s.router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
}

// Start begins the API server
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:    s.cfg.API.Bind,
		Handler: s.router,
	}

	util.Infof("API server listening on %s", s.cfg.API.Bind)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			util.Errorf("API server error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the API server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// SetUpstreamStateFunc sets the callback for getting upstream states
func (s *Server) SetUpstreamStateFunc(fn UpstreamStateFunc) {
	s.upstreamStateFunc = fn
}

// handleStats returns pool and network statistics
func (s *Server) handleStats(c *gin.Context) {
	// Check cache
	s.statsCacheMu.RLock()
	if s.statsCache != nil && time.Since(s.statsCacheTime) < s.cfg.API.StatsCache {
		cache := s.statsCache
		s.statsCacheMu.RUnlock()
		c.JSON(200, cache)
		return
	}
	s.statsCacheMu.RUnlock()

	// Get fresh stats
	poolStats, err := s.redis.GetPoolStats(
		s.cfg.Validation.HashrateWindow,
		s.cfg.Validation.HashrateLargeWindow,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get pool stats"})
		return
	}

	netStats, err := s.redis.GetNetworkStats()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get network stats"})
		return
	}

	response := &StatsResponse{
		Pool: PoolStats{
			Hashrate:        poolStats.Hashrate,
			HashrateLarge:   poolStats.HashrateLarge,
			Miners:          poolStats.Miners,
			Workers:         poolStats.Workers,
			BlocksFound:     poolStats.BlocksFound,
			LastBlockFound:  poolStats.LastBlockFound,
			LastBlockHeight: poolStats.LastBlockHeight,
			TotalPaid:       poolStats.TotalPaid,
			Fee:             s.cfg.Pool.Fee,
		},
		Network: NetworkStats{
			Height:     netStats.Height,
			Difficulty: netStats.Difficulty,
			Hashrate:   netStats.Hashrate,
		},
		Now: time.Now().Unix(),
	}

	// Update cache
	s.statsCacheMu.Lock()
	s.statsCache = response
	s.statsCacheTime = time.Now()
	s.statsCacheMu.Unlock()

	c.JSON(200, response)
}

// handleBlocks returns recent blocks
func (s *Server) handleBlocks(c *gin.Context) {
	blocks, err := s.redis.GetRecentBlocks(50)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get blocks"})
		return
	}

	// Get current height for confirmations
	netStats, _ := s.redis.GetNetworkStats()
	currentHeight := uint64(0)
	if netStats != nil {
		currentHeight = netStats.Height
	}

	response := make([]BlockResponse, 0, len(blocks))
	for _, block := range blocks {
		confirmations := uint64(0)
		if currentHeight > block.Height {
			confirmations = currentHeight - block.Height
		}

		response = append(response, BlockResponse{
			Height:        block.Height,
			Hash:          block.Hash,
			Finder:        block.Finder,
			Reward:        block.Reward,
			Timestamp:     block.Timestamp,
			Status:        string(block.Status),
			Confirmations: confirmations,
		})
	}

	c.JSON(200, gin.H{"blocks": response})
}

// handlePayments returns recent payments
func (s *Server) handlePayments(c *gin.Context) {
	payments, err := s.redis.GetRecentPayments(100)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get payments"})
		return
	}

	c.JSON(200, gin.H{"payments": payments})
}

// handleMiner returns miner statistics
func (s *Server) handleMiner(c *gin.Context) {
	address := c.Param("address")

	// Validate address
	if !util.ValidateAddress(address) {
		c.JSON(400, gin.H{"error": "Invalid address"})
		return
	}

	// Get miner data
	miner, err := s.redis.GetMiner(address)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get miner"})
		return
	}

	if miner == nil {
		c.JSON(404, gin.H{"error": "Miner not found"})
		return
	}

	// Get hashrate
	hashrate, _ := s.redis.GetMinerHashrate(address, s.cfg.Validation.HashrateWindow)
	hashrateLarge, _ := s.redis.GetMinerHashrate(address, s.cfg.Validation.HashrateLargeWindow)

	// Get recent payments
	payments, _ := s.redis.GetMinerPayments(address, 20)

	// Get worker statistics
	workerStats, _ := s.redis.GetMinerWorkers(address, s.cfg.Validation.HashrateWindow)
	workers := make([]WorkerStats, len(workerStats))
	for i, w := range workerStats {
		workers[i] = WorkerStats{
			Name:     w.Name,
			Hashrate: w.Hashrate,
			LastSeen: w.LastSeen,
		}
	}

	response := MinerResponse{
		Address:         address,
		Hashrate:        hashrate,
		HashrateLarge:   hashrateLarge,
		Balance:         miner.Balance,
		ImmatureBalance: miner.ImmatureBalance,
		PendingBalance:  miner.PendingBalance,
		TotalPaid:       miner.TotalPaid,
		BlocksFound:     miner.BlocksFound,
		LastShare:       miner.LastShare,
		Workers:         workers,
		Payments:        payments,
	}

	c.JSON(200, response)
}

// handleMinerPayments returns payment history for a miner
func (s *Server) handleMinerPayments(c *gin.Context) {
	address := c.Param("address")

	if !util.ValidateAddress(address) {
		c.JSON(400, gin.H{"error": "Invalid address"})
		return
	}

	payments, err := s.redis.GetMinerPayments(address, 100)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get payments"})
		return
	}

	c.JSON(200, gin.H{"payments": payments})
}

// LuckResponse represents luck statistics
type LuckResponse struct {
	Luck24h   float64      `json:"luck_24h"`
	Luck7d    float64      `json:"luck_7d"`
	Luck30d   float64      `json:"luck_30d"`
	LuckAll   float64      `json:"luck_all"`
	Blocks    []BlockLuck  `json:"recent_blocks"`
}

// BlockLuck represents luck info for a single block
type BlockLuck struct {
	Height      uint64  `json:"height"`
	Effort      float64 `json:"effort"`
	RoundShares uint64  `json:"round_shares"`
	Difficulty  uint64  `json:"difficulty"`
	Timestamp   int64   `json:"timestamp"`
}

// handleLuck returns mining luck statistics
func (s *Server) handleLuck(c *gin.Context) {
	luck, err := s.redis.GetLuckStats()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get luck stats"})
		return
	}

	c.JSON(200, luck)
}

// adminAuthMiddleware validates admin password
func (s *Server) adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check Authorization header
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(401, gin.H{"error": "Authorization required"})
			c.Abort()
			return
		}

		// Support both "Bearer <password>" and plain password
		password := strings.TrimPrefix(auth, "Bearer ")
		if password != s.cfg.API.AdminPassword {
			c.JSON(403, gin.H{"error": "Invalid password"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// AdminStatsResponse contains detailed admin statistics
type AdminStatsResponse struct {
	Pool           *storage.PoolStats    `json:"pool"`
	Network        *storage.NetworkStats `json:"network"`
	PendingPayouts int                   `json:"pending_payouts"`
	LockedPayouts  bool                  `json:"locked_payouts"`
	BlacklistCount int                   `json:"blacklist_count"`
	WhitelistCount int                   `json:"whitelist_count"`
	RedisInfo      map[string]string     `json:"redis_info"`
}

// handleAdminStats returns detailed admin statistics
func (s *Server) handleAdminStats(c *gin.Context) {
	poolStats, _ := s.redis.GetPoolStats(
		s.cfg.Validation.HashrateWindow,
		s.cfg.Validation.HashrateLargeWindow,
	)
	netStats, _ := s.redis.GetNetworkStats()
	pendingPayments, _ := s.redis.GetPendingPayments()
	locked, _ := s.redis.IsPayoutsLocked()
	blacklist, _ := s.redis.GetBlacklist()
	whitelist, _ := s.redis.GetWhitelist()

	response := AdminStatsResponse{
		Pool:           poolStats,
		Network:        netStats,
		PendingPayouts: len(pendingPayments),
		LockedPayouts:  locked,
		BlacklistCount: len(blacklist),
		WhitelistCount: len(whitelist),
	}

	c.JSON(200, response)
}

// handleGetBlacklist returns all blacklisted addresses
func (s *Server) handleGetBlacklist(c *gin.Context) {
	blacklist, err := s.redis.GetBlacklist()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get blacklist"})
		return
	}

	c.JSON(200, gin.H{"blacklist": blacklist})
}

// BlacklistRequest represents a blacklist add request
type BlacklistRequest struct {
	Address string `json:"address"`
}

// handleAddBlacklist adds an address to the blacklist
func (s *Server) handleAddBlacklist(c *gin.Context) {
	var req BlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	if req.Address == "" {
		c.JSON(400, gin.H{"error": "Address required"})
		return
	}

	if err := s.redis.AddToBlacklist(req.Address); err != nil {
		c.JSON(500, gin.H{"error": "Failed to add to blacklist"})
		return
	}

	util.Infof("Admin: Added %s to blacklist", req.Address)
	c.JSON(200, gin.H{"status": "ok", "address": req.Address})
}

// handleRemoveBlacklist removes an address from the blacklist
func (s *Server) handleRemoveBlacklist(c *gin.Context) {
	address := c.Param("address")
	if address == "" {
		c.JSON(400, gin.H{"error": "Address required"})
		return
	}

	if err := s.redis.RemoveFromBlacklist(address); err != nil {
		c.JSON(500, gin.H{"error": "Failed to remove from blacklist"})
		return
	}

	util.Infof("Admin: Removed %s from blacklist", address)
	c.JSON(200, gin.H{"status": "ok", "address": address})
}

// handleGetWhitelist returns all whitelisted IPs
func (s *Server) handleGetWhitelist(c *gin.Context) {
	whitelist, err := s.redis.GetWhitelist()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get whitelist"})
		return
	}

	c.JSON(200, gin.H{"whitelist": whitelist})
}

// WhitelistRequest represents a whitelist add request
type WhitelistRequest struct {
	IP string `json:"ip"`
}

// handleAddWhitelist adds an IP to the whitelist
func (s *Server) handleAddWhitelist(c *gin.Context) {
	var req WhitelistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	if req.IP == "" {
		c.JSON(400, gin.H{"error": "IP required"})
		return
	}

	if err := s.redis.AddToWhitelist(req.IP); err != nil {
		c.JSON(500, gin.H{"error": "Failed to add to whitelist"})
		return
	}

	util.Infof("Admin: Added %s to whitelist", req.IP)
	c.JSON(200, gin.H{"status": "ok", "ip": req.IP})
}

// handleRemoveWhitelist removes an IP from the whitelist
func (s *Server) handleRemoveWhitelist(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		c.JSON(400, gin.H{"error": "IP required"})
		return
	}

	if err := s.redis.RemoveFromWhitelist(ip); err != nil {
		c.JSON(500, gin.H{"error": "Failed to remove from whitelist"})
		return
	}

	util.Infof("Admin: Removed %s from whitelist", ip)
	c.JSON(200, gin.H{"status": "ok", "ip": ip})
}

// handlePendingPayments returns pending payments
func (s *Server) handlePendingPayments(c *gin.Context) {
	payments, err := s.redis.GetPendingPayments()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get pending payments"})
		return
	}

	c.JSON(200, gin.H{"pending_payments": payments})
}

// handleBackup returns a JSON backup of pool data
func (s *Server) handleBackup(c *gin.Context) {
	backup, err := s.redis.CreateBackup()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create backup"})
		return
	}

	// Set headers for file download
	c.Header("Content-Disposition", "attachment; filename=tos-pool-backup.json")
	c.Header("Content-Type", "application/json")

	data, _ := json.MarshalIndent(backup, "", "  ")
	c.Data(200, "application/json", data)
}

// handlePoolHashrateChart returns pool hashrate history
func (s *Server) handlePoolHashrateChart(c *gin.Context) {
	hoursStr := c.DefaultQuery("hours", "24")
	hours := 24
	if h, err := parseHours(hoursStr); err == nil {
		hours = h
	}

	// Limit to 168 hours (7 days)
	if hours > 168 {
		hours = 168
	}

	history, err := s.redis.GetPoolHashrateHistory(hours)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get hashrate history"})
		return
	}

	c.JSON(200, gin.H{
		"hours":  hours,
		"points": history,
	})
}

// handleMinerChart returns miner hashrate history
func (s *Server) handleMinerChart(c *gin.Context) {
	address := c.Param("address")

	if !util.ValidateAddress(address) {
		c.JSON(400, gin.H{"error": "Invalid address"})
		return
	}

	hoursStr := c.DefaultQuery("hours", "24")
	hours := 24
	if h, err := parseHours(hoursStr); err == nil {
		hours = h
	}

	// Limit to 24 hours for miner charts
	if hours > 24 {
		hours = 24
	}

	history, err := s.redis.GetMinerHashrateHistory(address, hours)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get hashrate history"})
		return
	}

	c.JSON(200, gin.H{
		"address": address,
		"hours":   hours,
		"points":  history,
	})
}

// parseHours parses hours string to int
func parseHours(s string) (int, error) {
	var hours int
	_, err := fmt.Sscanf(s, "%d", &hours)
	if err != nil || hours < 1 {
		return 24, err
	}
	return hours, nil
}

// handleUpstreams returns upstream node status
func (s *Server) handleUpstreams(c *gin.Context) {
	if s.upstreamStateFunc == nil {
		c.JSON(200, gin.H{
			"upstreams":     []UpstreamStatus{},
			"total":         0,
			"healthy":       0,
			"active":        "",
		})
		return
	}

	upstreams := s.upstreamStateFunc()

	healthyCount := 0
	var activeUpstream string
	for _, u := range upstreams {
		if u.Healthy {
			healthyCount++
			if activeUpstream == "" {
				activeUpstream = u.Name
			}
		}
	}

	c.JSON(200, gin.H{
		"upstreams": upstreams,
		"total":     len(upstreams),
		"healthy":   healthyCount,
		"active":    activeUpstream,
	})
}
