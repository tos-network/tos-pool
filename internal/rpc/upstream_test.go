package rpc

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
)

func TestNewUpstreamManager_SingleURL(t *testing.T) {
	cfg := &config.NodeConfig{
		URL:     "http://localhost:8545",
		Timeout: 10 * time.Second,
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	if mgr.UpstreamCount() != 1 {
		t.Errorf("Expected 1 upstream, got %d", mgr.UpstreamCount())
	}

	if mgr.GetActiveUpstream() != "primary" {
		t.Errorf("Expected active upstream 'primary', got '%s'", mgr.GetActiveUpstream())
	}

	client := mgr.GetClient()
	if client == nil {
		t.Error("Expected non-nil client")
	}

	if client.url != "http://localhost:8545" {
		t.Errorf("Expected URL 'http://localhost:8545', got '%s'", client.url)
	}
}

func TestNewUpstreamManager_MultipleUpstreams(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "primary", URL: "http://node1:8545", Weight: 10},
			{Name: "backup1", URL: "http://node2:8545", Weight: 5},
			{Name: "backup2", URL: "http://node3:8545", Weight: 1},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	if mgr.UpstreamCount() != 3 {
		t.Errorf("Expected 3 upstreams, got %d", mgr.UpstreamCount())
	}

	// Should be sorted by weight (highest first)
	if mgr.upstreams[0].name != "primary" {
		t.Errorf("Expected first upstream 'primary', got '%s'", mgr.upstreams[0].name)
	}
	if mgr.upstreams[0].weight != 10 {
		t.Errorf("Expected first upstream weight 10, got %d", mgr.upstreams[0].weight)
	}
	if mgr.upstreams[1].name != "backup1" {
		t.Errorf("Expected second upstream 'backup1', got '%s'", mgr.upstreams[1].name)
	}
	if mgr.upstreams[2].name != "backup2" {
		t.Errorf("Expected third upstream 'backup2', got '%s'", mgr.upstreams[2].name)
	}
}

func TestNewUpstreamManager_DefaultWeight(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"}, // No weight specified
			{Name: "node2", URL: "http://node2:8545", Weight: 0}, // Zero weight
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Both should have default weight of 1
	for i, u := range mgr.upstreams {
		if u.weight != 1 {
			t.Errorf("Upstream %d: expected weight 1, got %d", i, u.weight)
		}
	}
}

func TestNewUpstreamManager_DefaultName(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{URL: "http://node1:8545"}, // No name specified
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Name should default to URL
	if mgr.upstreams[0].name != "http://node1:8545" {
		t.Errorf("Expected name to default to URL, got '%s'", mgr.upstreams[0].name)
	}
}

func TestNewUpstreamManager_NoUpstreams(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		// No URL or Upstreams configured
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	if mgr.UpstreamCount() != 0 {
		t.Errorf("Expected 0 upstreams, got %d", mgr.UpstreamCount())
	}

	if mgr.GetClient() != nil {
		t.Error("Expected nil client when no upstreams configured")
	}

	if mgr.GetActiveUpstream() != "" {
		t.Errorf("Expected empty active upstream, got '%s'", mgr.GetActiveUpstream())
	}
}

func TestUpstreamManager_InitialHealthy(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
			{Name: "node2", URL: "http://node2:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// All upstreams should be healthy initially
	if !mgr.HasHealthyUpstream() {
		t.Error("Expected at least one healthy upstream")
	}

	if mgr.HealthyCount() != 2 {
		t.Errorf("Expected 2 healthy upstreams, got %d", mgr.HealthyCount())
	}
}

func TestUpstreamManager_RecordSuccess(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Record some successes
	mgr.RecordSuccess()
	mgr.RecordSuccess()
	mgr.RecordSuccess()

	states := mgr.GetUpstreamStates()
	if len(states) != 1 {
		t.Fatalf("Expected 1 state, got %d", len(states))
	}

	if states[0].SuccessCount != 3 {
		t.Errorf("Expected success count 3, got %d", states[0].SuccessCount)
	}

	if states[0].FailCount != 0 {
		t.Errorf("Expected fail count 0, got %d", states[0].FailCount)
	}

	if !states[0].Healthy {
		t.Error("Expected upstream to be healthy")
	}
}

func TestUpstreamManager_RecordFailure(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout:     10 * time.Second,
		MaxFailures: 3,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Record failures but not enough to trigger unhealthy
	mgr.RecordFailure()
	mgr.RecordFailure()

	states := mgr.GetUpstreamStates()
	if states[0].FailCount != 2 {
		t.Errorf("Expected fail count 2, got %d", states[0].FailCount)
	}
	if !states[0].Healthy {
		t.Error("Expected upstream to still be healthy after 2 failures")
	}

	// Third failure should mark unhealthy
	mgr.RecordFailure()

	states = mgr.GetUpstreamStates()
	if states[0].FailCount != 3 {
		t.Errorf("Expected fail count 3, got %d", states[0].FailCount)
	}
	if states[0].Healthy {
		t.Error("Expected upstream to be unhealthy after 3 failures")
	}
}

func TestUpstreamManager_FailureResetsSuccessCount(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Record some successes
	mgr.RecordSuccess()
	mgr.RecordSuccess()

	states := mgr.GetUpstreamStates()
	if states[0].SuccessCount != 2 {
		t.Errorf("Expected success count 2, got %d", states[0].SuccessCount)
	}

	// Failure should reset success count
	mgr.RecordFailure()

	states = mgr.GetUpstreamStates()
	if states[0].SuccessCount != 0 {
		t.Errorf("Expected success count 0 after failure, got %d", states[0].SuccessCount)
	}
}

func TestUpstreamManager_SuccessResetsFailCount(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Record some failures
	mgr.RecordFailure()
	mgr.RecordFailure()

	states := mgr.GetUpstreamStates()
	if states[0].FailCount != 2 {
		t.Errorf("Expected fail count 2, got %d", states[0].FailCount)
	}

	// Success should reset fail count
	mgr.RecordSuccess()

	states = mgr.GetUpstreamStates()
	if states[0].FailCount != 0 {
		t.Errorf("Expected fail count 0 after success, got %d", states[0].FailCount)
	}
}

func TestUpstreamManager_GetUpstreamStates(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "primary", URL: "http://node1:8545", Weight: 10},
			{Name: "backup", URL: "http://node2:8545", Weight: 5},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	states := mgr.GetUpstreamStates()

	if len(states) != 2 {
		t.Fatalf("Expected 2 states, got %d", len(states))
	}

	// States should be in sorted order (by weight, highest first)
	if states[0].Name != "primary" {
		t.Errorf("Expected first state name 'primary', got '%s'", states[0].Name)
	}
	if states[0].Weight != 10 {
		t.Errorf("Expected first state weight 10, got %d", states[0].Weight)
	}
	if states[0].URL != "http://node1:8545" {
		t.Errorf("Expected first state URL 'http://node1:8545', got '%s'", states[0].URL)
	}

	if states[1].Name != "backup" {
		t.Errorf("Expected second state name 'backup', got '%s'", states[1].Name)
	}
	if states[1].Weight != 5 {
		t.Errorf("Expected second state weight 5, got %d", states[1].Weight)
	}
}

func TestUpstreamManager_Failover(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout:     10 * time.Second,
		MaxFailures: 2,
		Upstreams: []config.UpstreamConfig{
			{Name: "primary", URL: "http://node1:8545", Weight: 10},
			{Name: "backup", URL: "http://node2:8545", Weight: 5},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Initially should use primary (highest weight)
	if mgr.GetActiveUpstream() != "primary" {
		t.Errorf("Expected active upstream 'primary', got '%s'", mgr.GetActiveUpstream())
	}

	// Fail primary enough times to trigger failover
	mgr.RecordFailure()
	mgr.RecordFailure()

	// After failover, selectBestUpstream should choose backup
	// Note: RecordFailure calls selectBestUpstream internally when unhealthy
	if mgr.GetActiveUpstream() != "backup" {
		t.Errorf("Expected active upstream 'backup' after failover, got '%s'", mgr.GetActiveUpstream())
	}

	// Verify primary is unhealthy
	states := mgr.GetUpstreamStates()
	for _, s := range states {
		if s.Name == "primary" && s.Healthy {
			t.Error("Expected primary to be unhealthy")
		}
		if s.Name == "backup" && !s.Healthy {
			t.Error("Expected backup to be healthy")
		}
	}
}

func TestUpstreamManager_SelectBestUpstream_ByWeight(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "low", URL: "http://node1:8545", Weight: 1},
			{Name: "high", URL: "http://node2:8545", Weight: 10},
			{Name: "medium", URL: "http://node3:8545", Weight: 5},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Should select highest weight
	mgr.selectBestUpstream()

	if mgr.GetActiveUpstream() != "high" {
		t.Errorf("Expected 'high' to be selected, got '%s'", mgr.GetActiveUpstream())
	}
}

func TestUpstreamManager_SelectBestUpstream_ByHeight(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545", Weight: 5},
			{Name: "node2", URL: "http://node2:8545", Weight: 5},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Set different heights
	mgr.upstreams[0].mu.Lock()
	mgr.upstreams[0].height = 100
	mgr.upstreams[0].mu.Unlock()

	mgr.upstreams[1].mu.Lock()
	mgr.upstreams[1].height = 200
	mgr.upstreams[1].mu.Unlock()

	// Should select higher height when weights are equal
	mgr.selectBestUpstream()

	activeIdx := atomic.LoadInt32(&mgr.activeIdx)
	if mgr.upstreams[activeIdx].height != 200 {
		t.Errorf("Expected node with height 200 to be selected, got height %d", mgr.upstreams[activeIdx].height)
	}
}

func TestUpstreamManager_HasHealthyUpstream(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout:     10 * time.Second,
		MaxFailures: 1,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
			{Name: "node2", URL: "http://node2:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	if !mgr.HasHealthyUpstream() {
		t.Error("Expected HasHealthyUpstream to return true initially")
	}

	// Mark first unhealthy
	mgr.upstreams[0].mu.Lock()
	mgr.upstreams[0].healthy = false
	mgr.upstreams[0].mu.Unlock()

	if !mgr.HasHealthyUpstream() {
		t.Error("Expected HasHealthyUpstream to return true when one is still healthy")
	}

	// Mark second unhealthy
	mgr.upstreams[1].mu.Lock()
	mgr.upstreams[1].healthy = false
	mgr.upstreams[1].mu.Unlock()

	if mgr.HasHealthyUpstream() {
		t.Error("Expected HasHealthyUpstream to return false when all are unhealthy")
	}
}

func TestUpstreamManager_HealthyCount(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
			{Name: "node2", URL: "http://node2:8545"},
			{Name: "node3", URL: "http://node3:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	if mgr.HealthyCount() != 3 {
		t.Errorf("Expected 3 healthy, got %d", mgr.HealthyCount())
	}

	// Mark one unhealthy
	mgr.upstreams[0].mu.Lock()
	mgr.upstreams[0].healthy = false
	mgr.upstreams[0].mu.Unlock()

	if mgr.HealthyCount() != 2 {
		t.Errorf("Expected 2 healthy, got %d", mgr.HealthyCount())
	}

	// Mark another unhealthy
	mgr.upstreams[1].mu.Lock()
	mgr.upstreams[1].healthy = false
	mgr.upstreams[1].mu.Unlock()

	if mgr.HealthyCount() != 1 {
		t.Errorf("Expected 1 healthy, got %d", mgr.HealthyCount())
	}
}

func TestUpstreamManager_GetClient_Fallback(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Set invalid active index
	atomic.StoreInt32(&mgr.activeIdx, 999)

	// Should fallback to first upstream
	client := mgr.GetClient()
	if client == nil {
		t.Error("Expected non-nil client from fallback")
	}
	if client.url != "http://node1:8545" {
		t.Errorf("Expected fallback to first upstream, got URL '%s'", client.url)
	}
}

func TestUpstreamManager_GetActiveUpstream_Fallback(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Set invalid active index
	atomic.StoreInt32(&mgr.activeIdx, 999)

	// Should fallback to first upstream name
	name := mgr.GetActiveUpstream()
	if name != "node1" {
		t.Errorf("Expected fallback name 'node1', got '%s'", name)
	}
}

func TestUpstreamManager_PerUpstreamTimeout(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "fast", URL: "http://fast:8545", Timeout: 5 * time.Second},
			{Name: "slow", URL: "http://slow:8545", Timeout: 30 * time.Second},
			{Name: "default", URL: "http://default:8545"}, // Should use global timeout
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Check timeouts
	for _, u := range mgr.upstreams {
		switch u.name {
		case "fast":
			if u.client.timeout != 5*time.Second {
				t.Errorf("Expected 'fast' timeout 5s, got %v", u.client.timeout)
			}
		case "slow":
			if u.client.timeout != 30*time.Second {
				t.Errorf("Expected 'slow' timeout 30s, got %v", u.client.timeout)
			}
		case "default":
			if u.client.timeout != 10*time.Second {
				t.Errorf("Expected 'default' timeout 10s (global), got %v", u.client.timeout)
			}
		}
	}
}

func TestUpstreamManager_ConcurrentAccess(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
			{Name: "node2", URL: "http://node2:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				mgr.RecordSuccess()
				mgr.RecordFailure()
				mgr.GetClient()
				mgr.GetActiveUpstream()
				mgr.GetUpstreamStates()
				mgr.HasHealthyUpstream()
				mgr.HealthyCount()
				mgr.UpstreamCount()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without deadlock or panic, test passes
}

func TestUpstreamManager_DefaultMaxFailures(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout: 10 * time.Second,
		// MaxFailures not set, should default to 3
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Should take 3 failures to mark unhealthy (default)
	mgr.RecordFailure()
	mgr.RecordFailure()

	if !mgr.HasHealthyUpstream() {
		t.Error("Expected still healthy after 2 failures with default max 3")
	}

	mgr.RecordFailure()

	if mgr.HasHealthyUpstream() {
		t.Error("Expected unhealthy after 3 failures with default max 3")
	}
}

func TestUpstreamManager_StopCancelsContext(t *testing.T) {
	cfg := &config.NodeConfig{
		Timeout:             10 * time.Second,
		HealthCheckInterval: 100 * time.Millisecond,
		Upstreams: []config.UpstreamConfig{
			{Name: "node1", URL: "http://node1:8545"},
		},
	}

	ctx := context.Background()
	mgr := NewUpstreamManager(ctx, cfg)

	// Start doesn't need to actually run health checks for this test
	// Just verify Stop completes without hanging
	done := make(chan bool)
	go func() {
		mgr.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop() timed out - possible deadlock")
	}
}
