// Package api provides the REST API server.
package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/storage"
	"github.com/tos-network/tos-pool/internal/util"
)

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
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

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
		Workers:         []WorkerStats{}, // TODO: populate from worker data
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
