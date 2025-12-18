// Package policy implements security policies for the mining pool.
// This includes IP banning, rate limiting, and invalid share tracking.
package policy

import (
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tos-network/tos-pool/internal/storage"
	"github.com/tos-network/tos-pool/internal/util"
)

// Config holds policy configuration
type Config struct {
	// Banning configuration
	BanningEnabled    bool
	BanTimeout        time.Duration // How long to ban an IP
	InvalidPercent    float32       // Ratio of invalid shares to trigger ban
	CheckThreshold    int32         // Minimum shares before checking ratio
	MalformedLimit    int32         // Max malformed requests before ban
	IPSetName         string        // Linux ipset name for kernel-level banning

	// Rate limiting configuration
	RateLimitEnabled  bool
	ConnectionLimit   int32         // Max new connections per IP per interval
	ConnectionGrace   time.Duration // Grace period after startup
	LimitJump         int32         // How much to increase limit on valid share

	// Score-based rate limiting
	ScoreEnabled      bool
	MaxScore          int32         // Maximum score before temporary ban
	ScoreResetTime    time.Duration // How often to reset scores
	ScoreTempBanTime  time.Duration // How long to temp ban when max score reached

	// Action costs (added to score)
	CostInvalidShare  int32 // Cost for invalid share
	CostMalformed     int32 // Cost for malformed request
	CostConnection    int32 // Cost for new connection
	CostAuth          int32 // Cost for authorization attempt

	// Reset intervals
	ResetInterval     time.Duration // How often to reset stats
	RefreshInterval   time.Duration // How often to refresh blacklist/whitelist
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() *Config {
	return &Config{
		BanningEnabled:    true,
		BanTimeout:        30 * time.Minute,
		InvalidPercent:    50.0,
		CheckThreshold:    100,
		MalformedLimit:    5,
		IPSetName:         "",

		RateLimitEnabled:  true,
		ConnectionLimit:   10,
		ConnectionGrace:   5 * time.Minute,
		LimitJump:         5,

		// Score-based rate limiting defaults
		ScoreEnabled:      true,
		MaxScore:          100,
		ScoreResetTime:    1 * time.Minute,
		ScoreTempBanTime:  5 * time.Minute,
		CostInvalidShare:  10,
		CostMalformed:     25,
		CostConnection:    1,
		CostAuth:          2,

		ResetInterval:     1 * time.Hour,
		RefreshInterval:   5 * time.Minute,
	}
}

// IPStats tracks per-IP statistics
type IPStats struct {
	mu            sync.Mutex
	LastBeat      int64  // Timestamp of last activity
	BannedAt      int64  // Timestamp when banned (0 = not banned)
	ValidShares   int32  // Count of valid shares
	InvalidShares int32  // Count of invalid shares
	Malformed     int32  // Count of malformed requests
	ConnLimit     int32  // Remaining connection allowance
	Banned        int32  // 1 = banned, 0 = not banned
	Score         int32  // Score-based rate limiting score
	LastScoreReset int64 // When score was last reset
}

// PolicyServer manages security policies
type PolicyServer struct {
	config    *Config
	redis     *storage.RedisClient

	// Per-IP stats
	statsMu   sync.RWMutex
	stats     map[string]*IPStats

	// Blacklist/Whitelist
	listMu    sync.RWMutex
	blacklist map[string]struct{}
	whitelist map[string]struct{}

	// Ban channel for async banning
	banChan   chan string

	// Timing
	startedAt int64

	// Control
	quit      chan struct{}
	wg        sync.WaitGroup
}

// NewPolicyServer creates a new policy server
func NewPolicyServer(cfg *Config, redis *storage.RedisClient) *PolicyServer {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &PolicyServer{
		config:    cfg,
		redis:     redis,
		stats:     make(map[string]*IPStats),
		blacklist: make(map[string]struct{}),
		whitelist: make(map[string]struct{}),
		banChan:   make(chan string, 64),
		startedAt: time.Now().UnixMilli(),
		quit:      make(chan struct{}),
	}
}

// Start begins the policy server background tasks
func (p *PolicyServer) Start() {
	util.Info("Starting policy server...")

	// Initial refresh
	p.refreshLists()

	// Start reset timer
	p.wg.Add(1)
	go p.resetLoop()

	// Start refresh timer
	p.wg.Add(1)
	go p.refreshLoop()

	// Start ban workers
	for i := 0; i < 2; i++ {
		p.wg.Add(1)
		go p.banWorker()
	}

	util.Info("Policy server started")
}

// Stop shuts down the policy server
func (p *PolicyServer) Stop() {
	close(p.quit)
	p.wg.Wait()
	util.Info("Policy server stopped")
}

// resetLoop periodically resets stale stats
func (p *PolicyServer) resetLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.ResetInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.quit:
			return
		case <-ticker.C:
			p.resetStats()
		}
	}
}

// refreshLoop periodically refreshes blacklist/whitelist
func (p *PolicyServer) refreshLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.quit:
			return
		case <-ticker.C:
			p.refreshLists()
		}
	}
}

// banWorker processes ban requests
func (p *PolicyServer) banWorker() {
	defer p.wg.Done()

	for {
		select {
		case <-p.quit:
			return
		case ip := <-p.banChan:
			p.executeBan(ip)
		}
	}
}

// resetStats clears old statistics
func (p *PolicyServer) resetStats() {
	now := time.Now().UnixMilli()
	banTimeout := p.config.BanTimeout.Milliseconds()
	staleTimeout := p.config.ResetInterval.Milliseconds()

	p.statsMu.Lock()
	defer p.statsMu.Unlock()

	removed := 0
	unbanned := 0

	for ip, stats := range p.stats {
		stats.mu.Lock()

		// Check if ban expired
		if stats.BannedAt > 0 && now-stats.BannedAt >= banTimeout {
			stats.BannedAt = 0
			if atomic.CompareAndSwapInt32(&stats.Banned, 1, 0) {
				unbanned++
				util.Infof("Ban expired for %s", ip)
			}
		}

		// Remove stale entries
		if now-stats.LastBeat >= staleTimeout && stats.Banned == 0 {
			stats.mu.Unlock()
			delete(p.stats, ip)
			removed++
			continue
		}

		stats.mu.Unlock()
	}

	if removed > 0 || unbanned > 0 {
		util.Debugf("Policy stats reset: removed %d stale, unbanned %d IPs", removed, unbanned)
	}
}

// refreshLists reloads blacklist/whitelist from storage
func (p *PolicyServer) refreshLists() {
	if p.redis == nil {
		return
	}

	// Load blacklist
	blacklist, err := p.redis.GetBlacklist()
	if err != nil {
		util.Warnf("Failed to load blacklist: %v", err)
	} else {
		p.listMu.Lock()
		p.blacklist = make(map[string]struct{})
		for _, addr := range blacklist {
			p.blacklist[strings.ToLower(addr)] = struct{}{}
		}
		p.listMu.Unlock()
	}

	// Load whitelist
	whitelist, err := p.redis.GetWhitelist()
	if err != nil {
		util.Warnf("Failed to load whitelist: %v", err)
	} else {
		p.listMu.Lock()
		p.whitelist = make(map[string]struct{})
		for _, ip := range whitelist {
			p.whitelist[ip] = struct{}{}
		}
		p.listMu.Unlock()
	}
}

// getStats gets or creates stats for an IP
func (p *PolicyServer) getStats(ip string) *IPStats {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()

	stats, ok := p.stats[ip]
	if !ok {
		stats = &IPStats{
			LastBeat:  time.Now().UnixMilli(),
			ConnLimit: p.config.ConnectionLimit,
		}
		p.stats[ip] = stats
	} else {
		stats.LastBeat = time.Now().UnixMilli()
	}

	return stats
}

// IsBanned checks if an IP is currently banned
func (p *PolicyServer) IsBanned(ip string) bool {
	if !p.config.BanningEnabled {
		return false
	}

	stats := p.getStats(ip)
	return atomic.LoadInt32(&stats.Banned) > 0
}

// ApplyConnectionLimit checks and decrements connection limit
func (p *PolicyServer) ApplyConnectionLimit(ip string) bool {
	if !p.config.RateLimitEnabled {
		return true
	}

	// Grace period after startup
	if time.Now().UnixMilli()-p.startedAt < p.config.ConnectionGrace.Milliseconds() {
		return true
	}

	stats := p.getStats(ip)
	stats.mu.Lock()
	defer stats.mu.Unlock()

	stats.ConnLimit--
	return stats.ConnLimit >= 0
}

// ApplyLoginPolicy checks if a wallet address is blacklisted
func (p *PolicyServer) ApplyLoginPolicy(address, ip string) bool {
	p.listMu.RLock()
	_, blacklisted := p.blacklist[strings.ToLower(address)]
	p.listMu.RUnlock()

	if blacklisted {
		util.Warnf("Blacklisted address %s from IP %s", address, ip)
		p.BanIP(ip)
		return false
	}

	return true
}

// ApplyMalformedPolicy tracks malformed requests
func (p *PolicyServer) ApplyMalformedPolicy(ip string) bool {
	if !p.config.BanningEnabled {
		return true
	}

	stats := p.getStats(ip)
	stats.mu.Lock()
	defer stats.mu.Unlock()

	stats.Malformed++
	if stats.Malformed >= p.config.MalformedLimit {
		stats.mu.Unlock()
		p.BanIP(ip)
		stats.mu.Lock()
		return false
	}

	return true
}

// ApplySharePolicy tracks valid/invalid shares and may ban
func (p *PolicyServer) ApplySharePolicy(ip string, valid bool) bool {
	if !p.config.BanningEnabled {
		return true
	}

	stats := p.getStats(ip)
	stats.mu.Lock()
	defer stats.mu.Unlock()

	if valid {
		stats.ValidShares++
		// Reward valid shares with connection allowance
		if p.config.RateLimitEnabled {
			stats.ConnLimit += p.config.LimitJump
		}
	} else {
		stats.InvalidShares++
	}

	// Check if we have enough samples
	totalShares := stats.ValidShares + stats.InvalidShares
	if totalShares < p.config.CheckThreshold {
		return true
	}

	// Calculate invalid ratio
	invalidRatio := float32(stats.InvalidShares) / float32(stats.ValidShares+1) * 100

	// Reset counters
	stats.ValidShares = 0
	stats.InvalidShares = 0

	// Ban if ratio too high
	if invalidRatio >= p.config.InvalidPercent {
		util.Warnf("Banning %s: invalid share ratio %.1f%% >= %.1f%%", ip, invalidRatio, p.config.InvalidPercent)
		stats.mu.Unlock()
		p.BanIP(ip)
		stats.mu.Lock()
		return false
	}

	return true
}

// AddScore adds to an IP's score and returns false if banned
func (p *PolicyServer) AddScore(ip string, cost int32) bool {
	if !p.config.ScoreEnabled {
		return true
	}

	stats := p.getStats(ip)
	stats.mu.Lock()
	defer stats.mu.Unlock()

	now := time.Now().Unix()

	// Reset score if enough time passed
	if now-stats.LastScoreReset >= int64(p.config.ScoreResetTime.Seconds()) {
		stats.Score = 0
		stats.LastScoreReset = now
	}

	// Add cost to score
	stats.Score += cost

	// Check if max score exceeded
	if stats.Score >= p.config.MaxScore {
		util.Warnf("Score limit exceeded for %s: %d >= %d", ip, stats.Score, p.config.MaxScore)
		stats.Score = 0 // Reset score

		// Temporary ban
		if p.config.ScoreTempBanTime > 0 {
			stats.BannedAt = time.Now().UnixMilli()
			atomic.StoreInt32(&stats.Banned, 1)
		}
		return false
	}

	return true
}

// GetScore returns current score for an IP
func (p *PolicyServer) GetScore(ip string) int32 {
	stats := p.getStats(ip)
	stats.mu.Lock()
	defer stats.mu.Unlock()
	return stats.Score
}

// ApplyConnectionScore applies connection cost
func (p *PolicyServer) ApplyConnectionScore(ip string) bool {
	return p.AddScore(ip, p.config.CostConnection)
}

// ApplyAuthScore applies authorization cost
func (p *PolicyServer) ApplyAuthScore(ip string) bool {
	return p.AddScore(ip, p.config.CostAuth)
}

// ApplyInvalidShareScore applies invalid share cost
func (p *PolicyServer) ApplyInvalidShareScore(ip string) bool {
	return p.AddScore(ip, p.config.CostInvalidShare)
}

// ApplyMalformedScore applies malformed request cost
func (p *PolicyServer) ApplyMalformedScore(ip string) bool {
	return p.AddScore(ip, p.config.CostMalformed)
}

// BanIP bans an IP address
func (p *PolicyServer) BanIP(ip string) {
	if !p.config.BanningEnabled {
		return
	}

	// Check whitelist
	p.listMu.RLock()
	_, whitelisted := p.whitelist[ip]
	p.listMu.RUnlock()

	if whitelisted {
		util.Debugf("IP %s is whitelisted, not banning", ip)
		return
	}

	stats := p.getStats(ip)
	stats.mu.Lock()
	stats.BannedAt = time.Now().UnixMilli()
	stats.mu.Unlock()

	if atomic.CompareAndSwapInt32(&stats.Banned, 0, 1) {
		util.Infof("Banned IP: %s", ip)

		// Queue for ipset if configured
		if p.config.IPSetName != "" {
			select {
			case p.banChan <- ip:
			default:
				util.Warn("Ban channel full, skipping ipset for", ip)
			}
		}
	}
}

// executeBan adds IP to kernel ipset
func (p *PolicyServer) executeBan(ip string) {
	if p.config.IPSetName == "" {
		return
	}

	timeout := int(p.config.BanTimeout.Seconds())
	cmd := exec.Command("sudo", "ipset", "add", p.config.IPSetName, ip, "timeout", string(rune(timeout)), "-!")

	if err := cmd.Run(); err != nil {
		util.Warnf("Failed to add %s to ipset: %v", ip, err)
	} else {
		util.Debugf("Added %s to ipset %s with timeout %ds", ip, p.config.IPSetName, timeout)
	}
}

// IsWhitelisted checks if an IP is whitelisted
func (p *PolicyServer) IsWhitelisted(ip string) bool {
	p.listMu.RLock()
	defer p.listMu.RUnlock()
	_, ok := p.whitelist[ip]
	return ok
}

// IsBlacklisted checks if an address is blacklisted
func (p *PolicyServer) IsBlacklisted(address string) bool {
	p.listMu.RLock()
	defer p.listMu.RUnlock()
	_, ok := p.blacklist[strings.ToLower(address)]
	return ok
}

// GetStats returns stats for monitoring
func (p *PolicyServer) GetStats() (total, banned int) {
	p.statsMu.RLock()
	defer p.statsMu.RUnlock()

	total = len(p.stats)
	for _, stats := range p.stats {
		if atomic.LoadInt32(&stats.Banned) > 0 {
			banned++
		}
	}
	return
}

// AddToBlacklist adds an address to the blacklist
func (p *PolicyServer) AddToBlacklist(address string) error {
	if p.redis != nil {
		if err := p.redis.AddToBlacklist(address); err != nil {
			return err
		}
	}

	p.listMu.Lock()
	p.blacklist[strings.ToLower(address)] = struct{}{}
	p.listMu.Unlock()

	return nil
}

// AddToWhitelist adds an IP to the whitelist
func (p *PolicyServer) AddToWhitelist(ip string) error {
	if p.redis != nil {
		if err := p.redis.AddToWhitelist(ip); err != nil {
			return err
		}
	}

	p.listMu.Lock()
	p.whitelist[ip] = struct{}{}
	p.listMu.Unlock()

	return nil
}
