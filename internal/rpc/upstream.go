// Package rpc provides TOS node communication with multi-upstream failover.
package rpc

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/util"
)

// UpstreamState represents the health state of an upstream node
type UpstreamState struct {
	Name          string
	URL           string
	Healthy       bool
	LastCheck     time.Time
	SuccessCount  int32
	FailCount     int32
	ResponseTime  time.Duration
	Height        uint64
	Weight        int
}

// Upstream wraps a TOSClient with health tracking
type Upstream struct {
	client   *TOSClient
	name     string
	weight   int

	mu           sync.RWMutex
	healthy      bool
	failCount    int32
	successCount int32
	lastCheck    time.Time
	responseTime time.Duration
	height       uint64
}

// UpstreamManager manages multiple upstream nodes with automatic failover
type UpstreamManager struct {
	upstreams []*Upstream
	cfg       *config.NodeConfig

	// Current active upstream index
	activeIdx int32

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewUpstreamManager creates a new upstream manager with failover support
func NewUpstreamManager(ctx context.Context, cfg *config.NodeConfig) *UpstreamManager {
	mgrCtx, cancel := context.WithCancel(ctx)

	mgr := &UpstreamManager{
		cfg:    cfg,
		ctx:    mgrCtx,
		cancel: cancel,
	}

	// Build upstream list from config
	if len(cfg.Upstreams) > 0 {
		// Use configured upstreams
		for _, ucfg := range cfg.Upstreams {
			timeout := ucfg.Timeout
			if timeout == 0 {
				timeout = cfg.Timeout
			}
			weight := ucfg.Weight
			if weight == 0 {
				weight = 1
			}
			name := ucfg.Name
			if name == "" {
				name = ucfg.URL
			}

			upstream := &Upstream{
				client:  NewTOSClient(ucfg.URL, timeout),
				name:    name,
				weight:  weight,
				healthy: true, // Assume healthy initially
			}
			mgr.upstreams = append(mgr.upstreams, upstream)
		}
	} else if cfg.URL != "" {
		// Fall back to single URL for backward compatibility
		upstream := &Upstream{
			client:  NewTOSClient(cfg.URL, cfg.Timeout),
			name:    "primary",
			weight:  1,
			healthy: true,
		}
		mgr.upstreams = append(mgr.upstreams, upstream)
	}

	// Sort by weight (higher weight first)
	sort.Slice(mgr.upstreams, func(i, j int) bool {
		return mgr.upstreams[i].weight > mgr.upstreams[j].weight
	})

	return mgr
}

// Start begins the health check loop
func (m *UpstreamManager) Start() {
	if len(m.upstreams) == 0 {
		util.Warn("No upstreams configured")
		return
	}

	util.Infof("Starting upstream manager with %d nodes", len(m.upstreams))
	for i, u := range m.upstreams {
		util.Infof("  [%d] %s (weight=%d)", i, u.name, u.weight)
	}

	// Initial health check
	m.checkAllUpstreams()

	// Start health check loop
	m.wg.Add(1)
	go m.healthCheckLoop()
}

// Stop shuts down the upstream manager
func (m *UpstreamManager) Stop() {
	m.cancel()
	m.wg.Wait()
	util.Info("Upstream manager stopped")
}

// healthCheckLoop periodically checks all upstream nodes
func (m *UpstreamManager) healthCheckLoop() {
	defer m.wg.Done()

	interval := m.cfg.HealthCheckInterval
	if interval == 0 {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAllUpstreams()
		}
	}
}

// checkAllUpstreams checks health of all upstream nodes
func (m *UpstreamManager) checkAllUpstreams() {
	var wg sync.WaitGroup

	for _, upstream := range m.upstreams {
		wg.Add(1)
		go func(u *Upstream) {
			defer wg.Done()
			m.checkUpstream(u)
		}(upstream)
	}

	wg.Wait()

	// Select best healthy upstream
	m.selectBestUpstream()
}

// checkUpstream performs a health check on a single upstream
func (m *UpstreamManager) checkUpstream(u *Upstream) {
	timeout := m.cfg.HealthCheckTimeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	defer cancel()

	start := time.Now()

	// Try to get latest block as health check
	block, err := u.client.GetLatestBlock(ctx)

	responseTime := time.Since(start)

	u.mu.Lock()
	defer u.mu.Unlock()

	u.lastCheck = time.Now()
	u.responseTime = responseTime

	if err != nil {
		// Health check failed
		u.failCount++
		u.successCount = 0

		maxFailures := m.cfg.MaxFailures
		if maxFailures == 0 {
			maxFailures = 3
		}

		if u.failCount >= int32(maxFailures) && u.healthy {
			u.healthy = false
			util.Warnf("Upstream %s marked UNHEALTHY after %d failures: %v", u.name, u.failCount, err)
		}
	} else {
		// Health check succeeded
		u.successCount++
		u.height = block.Height

		recoveryThreshold := m.cfg.RecoveryThreshold
		if recoveryThreshold == 0 {
			recoveryThreshold = 2
		}

		if !u.healthy && u.successCount >= int32(recoveryThreshold) {
			u.healthy = true
			u.failCount = 0
			util.Infof("Upstream %s recovered and marked HEALTHY (height=%d, response=%v)", u.name, u.height, responseTime)
		} else if u.healthy {
			u.failCount = 0
		}
	}
}

// selectBestUpstream selects the best healthy upstream based on weight and height
func (m *UpstreamManager) selectBestUpstream() {
	var bestIdx int = -1
	var bestWeight int = -1
	var bestHeight uint64 = 0

	for i, u := range m.upstreams {
		u.mu.RLock()
		healthy := u.healthy
		weight := u.weight
		height := u.height
		u.mu.RUnlock()

		if !healthy {
			continue
		}

		// Prefer higher weight, then higher height
		if weight > bestWeight || (weight == bestWeight && height > bestHeight) {
			bestIdx = i
			bestWeight = weight
			bestHeight = height
		}
	}

	if bestIdx >= 0 {
		oldIdx := atomic.LoadInt32(&m.activeIdx)
		if int32(bestIdx) != oldIdx {
			atomic.StoreInt32(&m.activeIdx, int32(bestIdx))
			util.Infof("Switched to upstream %s (idx=%d, weight=%d, height=%d)",
				m.upstreams[bestIdx].name, bestIdx, bestWeight, bestHeight)
		}
	} else {
		util.Warn("No healthy upstreams available!")
	}
}

// GetClient returns the current active client
// If the active client fails, it will try to failover to another healthy upstream
func (m *UpstreamManager) GetClient() *TOSClient {
	if len(m.upstreams) == 0 {
		return nil
	}

	idx := atomic.LoadInt32(&m.activeIdx)
	if idx >= 0 && idx < int32(len(m.upstreams)) {
		return m.upstreams[idx].client
	}

	// Fallback to first upstream
	return m.upstreams[0].client
}

// GetActiveUpstream returns the name of the active upstream
func (m *UpstreamManager) GetActiveUpstream() string {
	if len(m.upstreams) == 0 {
		return ""
	}

	idx := atomic.LoadInt32(&m.activeIdx)
	if idx >= 0 && idx < int32(len(m.upstreams)) {
		return m.upstreams[idx].name
	}

	return m.upstreams[0].name
}

// GetUpstreamStates returns the state of all upstreams for monitoring
func (m *UpstreamManager) GetUpstreamStates() []UpstreamState {
	states := make([]UpstreamState, len(m.upstreams))

	for i, u := range m.upstreams {
		u.mu.RLock()
		states[i] = UpstreamState{
			Name:         u.name,
			URL:          u.client.url,
			Healthy:      u.healthy,
			LastCheck:    u.lastCheck,
			SuccessCount: u.successCount,
			FailCount:    u.failCount,
			ResponseTime: u.responseTime,
			Height:       u.height,
			Weight:       u.weight,
		}
		u.mu.RUnlock()
	}

	return states
}

// HasHealthyUpstream returns true if at least one upstream is healthy
func (m *UpstreamManager) HasHealthyUpstream() bool {
	for _, u := range m.upstreams {
		u.mu.RLock()
		healthy := u.healthy
		u.mu.RUnlock()
		if healthy {
			return true
		}
	}
	return false
}

// RecordSuccess records a successful call on the active upstream
func (m *UpstreamManager) RecordSuccess() {
	idx := atomic.LoadInt32(&m.activeIdx)
	if idx >= 0 && idx < int32(len(m.upstreams)) {
		u := m.upstreams[idx]
		u.mu.Lock()
		u.successCount++
		u.failCount = 0
		u.healthy = true
		u.mu.Unlock()
	}
}

// RecordFailure records a failed call and triggers failover if needed
func (m *UpstreamManager) RecordFailure() {
	idx := atomic.LoadInt32(&m.activeIdx)
	if idx < 0 || idx >= int32(len(m.upstreams)) {
		return
	}

	u := m.upstreams[idx]
	u.mu.Lock()
	u.failCount++
	u.successCount = 0

	maxFailures := m.cfg.MaxFailures
	if maxFailures == 0 {
		maxFailures = 3
	}

	shouldFailover := u.failCount >= int32(maxFailures) && u.healthy
	if shouldFailover {
		u.healthy = false
		util.Warnf("Upstream %s marked unhealthy due to call failures", u.name)
	}
	u.mu.Unlock()

	// Trigger failover if current upstream became unhealthy
	if shouldFailover {
		m.selectBestUpstream()
	}
}

// CallWithFailover executes a function with automatic failover to healthy upstreams
func (m *UpstreamManager) CallWithFailover(fn func(*TOSClient) error) error {
	// Try active upstream first
	client := m.GetClient()
	if client == nil {
		return nil
	}

	err := fn(client)
	if err == nil {
		m.RecordSuccess()
		return nil
	}

	// Record failure and try failover
	m.RecordFailure()

	// Try other healthy upstreams
	activeIdx := atomic.LoadInt32(&m.activeIdx)
	for i, u := range m.upstreams {
		if int32(i) == activeIdx {
			continue // Skip the one we already tried
		}

		u.mu.RLock()
		healthy := u.healthy
		u.mu.RUnlock()

		if !healthy {
			continue
		}

		util.Infof("Failover: trying upstream %s", u.name)

		if err := fn(u.client); err == nil {
			// Success - update active upstream
			atomic.StoreInt32(&m.activeIdx, int32(i))
			util.Infof("Failover successful: now using %s", u.name)
			return nil
		}

		// This upstream also failed
		u.mu.Lock()
		u.failCount++
		u.mu.Unlock()
	}

	// All upstreams failed
	return err
}

// UpstreamCount returns the number of configured upstreams
func (m *UpstreamManager) UpstreamCount() int {
	return len(m.upstreams)
}

// HealthyCount returns the number of healthy upstreams
func (m *UpstreamManager) HealthyCount() int {
	count := 0
	for _, u := range m.upstreams {
		u.mu.RLock()
		if u.healthy {
			count++
		}
		u.mu.RUnlock()
	}
	return count
}
