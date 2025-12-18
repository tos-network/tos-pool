package policy

import (
	"sync"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Check default values
	if !cfg.BanningEnabled {
		t.Error("BanningEnabled should be true by default")
	}

	if cfg.BanTimeout != 30*time.Minute {
		t.Errorf("BanTimeout = %v, want 30m", cfg.BanTimeout)
	}

	if cfg.InvalidPercent != 50.0 {
		t.Errorf("InvalidPercent = %v, want 50.0", cfg.InvalidPercent)
	}

	if cfg.CheckThreshold != 100 {
		t.Errorf("CheckThreshold = %v, want 100", cfg.CheckThreshold)
	}

	if cfg.MalformedLimit != 5 {
		t.Errorf("MalformedLimit = %v, want 5", cfg.MalformedLimit)
	}

	if !cfg.RateLimitEnabled {
		t.Error("RateLimitEnabled should be true by default")
	}

	if cfg.ConnectionLimit != 10 {
		t.Errorf("ConnectionLimit = %v, want 10", cfg.ConnectionLimit)
	}

	if !cfg.ScoreEnabled {
		t.Error("ScoreEnabled should be true by default")
	}

	if cfg.MaxScore != 100 {
		t.Errorf("MaxScore = %v, want 100", cfg.MaxScore)
	}

	if cfg.CostInvalidShare != 10 {
		t.Errorf("CostInvalidShare = %v, want 10", cfg.CostInvalidShare)
	}

	if cfg.CostMalformed != 25 {
		t.Errorf("CostMalformed = %v, want 25", cfg.CostMalformed)
	}
}

func TestNewPolicyServer(t *testing.T) {
	// Test with nil config
	ps := NewPolicyServer(nil, nil)
	if ps == nil {
		t.Fatal("NewPolicyServer returned nil")
	}
	if ps.config == nil {
		t.Fatal("PolicyServer.config should not be nil")
	}

	// Test with custom config
	cfg := &Config{
		BanningEnabled: false,
		ConnectionLimit: 5,
	}
	ps = NewPolicyServer(cfg, nil)
	if ps.config.ConnectionLimit != 5 {
		t.Errorf("ConnectionLimit = %v, want 5", ps.config.ConnectionLimit)
	}
}

func TestIsBanned(t *testing.T) {
	cfg := DefaultConfig()
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Initially not banned
	if ps.IsBanned(ip) {
		t.Error("IP should not be banned initially")
	}

	// Ban the IP
	ps.BanIP(ip)

	// Should be banned now
	if !ps.IsBanned(ip) {
		t.Error("IP should be banned after BanIP")
	}
}

func TestIsBannedDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BanningEnabled = false
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"
	ps.BanIP(ip)

	// Should not be banned when banning is disabled
	if ps.IsBanned(ip) {
		t.Error("IP should not be banned when banning is disabled")
	}
}

func TestApplyConnectionLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConnectionLimit = 3
	cfg.ConnectionGrace = 0 // Disable grace period for test
	ps := NewPolicyServer(cfg, nil)
	ps.startedAt = 0 // Bypass grace period

	ip := "192.168.1.100"

	// First 3 connections should be allowed
	for i := 0; i < 3; i++ {
		if !ps.ApplyConnectionLimit(ip) {
			t.Errorf("Connection %d should be allowed", i+1)
		}
	}

	// 4th connection should be denied
	if ps.ApplyConnectionLimit(ip) {
		t.Error("4th connection should be denied")
	}
}

func TestApplyConnectionLimitDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RateLimitEnabled = false
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Should always allow when disabled
	for i := 0; i < 100; i++ {
		if !ps.ApplyConnectionLimit(ip) {
			t.Error("Connection should be allowed when rate limiting is disabled")
		}
	}
}

func TestApplyLoginPolicy(t *testing.T) {
	cfg := DefaultConfig()
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"
	addr := "tos1testaddress"

	// Should be allowed initially
	if !ps.ApplyLoginPolicy(addr, ip) {
		t.Error("Login should be allowed for non-blacklisted address")
	}

	// Add to blacklist
	ps.AddToBlacklist(addr)

	// Should be denied now
	if ps.ApplyLoginPolicy(addr, ip) {
		t.Error("Login should be denied for blacklisted address")
	}

	// IP should be banned
	if !ps.IsBanned(ip) {
		t.Error("IP should be banned after blacklisted address login attempt")
	}
}

func TestApplyMalformedPolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MalformedLimit = 3
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// First 2 malformed requests should be allowed
	for i := 0; i < 2; i++ {
		if !ps.ApplyMalformedPolicy(ip) {
			t.Errorf("Malformed request %d should be allowed", i+1)
		}
	}

	// 3rd malformed request should trigger ban
	if ps.ApplyMalformedPolicy(ip) {
		t.Error("3rd malformed request should trigger ban")
	}

	// IP should be banned
	if !ps.IsBanned(ip) {
		t.Error("IP should be banned after malformed limit exceeded")
	}
}

func TestApplyMalformedPolicyDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BanningEnabled = false
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Should always return true when banning is disabled
	for i := 0; i < 100; i++ {
		if !ps.ApplyMalformedPolicy(ip) {
			t.Error("Should always return true when banning is disabled")
		}
	}
}

func TestApplySharePolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CheckThreshold = 10
	cfg.InvalidPercent = 50.0
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Submit 5 valid shares
	for i := 0; i < 5; i++ {
		if !ps.ApplySharePolicy(ip, true) {
			t.Errorf("Valid share %d should be accepted", i+1)
		}
	}

	// Submit 5 invalid shares (50% ratio - should trigger ban at threshold)
	for i := 0; i < 4; i++ {
		if !ps.ApplySharePolicy(ip, false) {
			t.Errorf("Invalid share %d should be accepted before threshold", i+1)
		}
	}

	// 10th share (5th invalid) should trigger evaluation
	// With 5 valid and 5 invalid, ratio is 100% which exceeds 50%
	if ps.ApplySharePolicy(ip, false) {
		t.Error("Should return false when invalid ratio exceeds threshold")
	}
}

func TestApplySharePolicyDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BanningEnabled = false
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Should always return true when banning is disabled
	for i := 0; i < 100; i++ {
		if !ps.ApplySharePolicy(ip, false) {
			t.Error("Should always return true when banning is disabled")
		}
	}
}

func TestAddScore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxScore = 50
	cfg.ScoreResetTime = 1 * time.Hour
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Add score below max
	if !ps.AddScore(ip, 25) {
		t.Error("Score 25 should be allowed (below max 50)")
	}

	if ps.GetScore(ip) != 25 {
		t.Errorf("Score = %d, want 25", ps.GetScore(ip))
	}

	// Add more score to exceed max
	if ps.AddScore(ip, 30) {
		t.Error("Score 55 should exceed max 50")
	}

	// Score should be reset after ban
	if ps.GetScore(ip) != 0 {
		t.Errorf("Score should be reset to 0 after ban, got %d", ps.GetScore(ip))
	}
}

func TestAddScoreDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ScoreEnabled = false
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Should always return true when score is disabled
	for i := 0; i < 100; i++ {
		if !ps.AddScore(ip, 1000) {
			t.Error("Should always return true when score is disabled")
		}
	}
}

func TestApplyConnectionScore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxScore = 10
	cfg.CostConnection = 3
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// 3 connections should be allowed (score: 3, 6, 9)
	for i := 0; i < 3; i++ {
		if !ps.ApplyConnectionScore(ip) {
			t.Errorf("Connection %d should be allowed", i+1)
		}
	}

	// 4th connection should exceed max (score: 12 > 10)
	if ps.ApplyConnectionScore(ip) {
		t.Error("4th connection should exceed max score")
	}
}

func TestApplyAuthScore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxScore = 15
	cfg.CostAuth = 5
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// 2 auths should be allowed (score: 5, 10)
	for i := 0; i < 2; i++ {
		if !ps.ApplyAuthScore(ip) {
			t.Errorf("Auth %d should be allowed", i+1)
		}
	}

	// 3rd auth should exceed max (score: 15 >= 15)
	if ps.ApplyAuthScore(ip) {
		t.Error("3rd auth should exceed max score")
	}
}

func TestApplyInvalidShareScore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxScore = 25
	cfg.CostInvalidShare = 10
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// 2 invalid shares should be allowed (score: 10, 20)
	for i := 0; i < 2; i++ {
		if !ps.ApplyInvalidShareScore(ip) {
			t.Errorf("Invalid share %d should be allowed", i+1)
		}
	}

	// 3rd invalid share should exceed max (score: 30 > 25)
	if ps.ApplyInvalidShareScore(ip) {
		t.Error("3rd invalid share should exceed max score")
	}
}

func TestApplyMalformedScore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxScore = 75
	cfg.CostMalformed = 25
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// 2 malformed should be allowed (score: 25, 50)
	for i := 0; i < 2; i++ {
		if !ps.ApplyMalformedScore(ip) {
			t.Errorf("Malformed %d should be allowed", i+1)
		}
	}

	// 3rd malformed should exceed max (score: 75 >= 75)
	if ps.ApplyMalformedScore(ip) {
		t.Error("3rd malformed should exceed max score")
	}
}

func TestBanIPWhitelisted(t *testing.T) {
	cfg := DefaultConfig()
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Add to whitelist
	ps.AddToWhitelist(ip)

	// Try to ban
	ps.BanIP(ip)

	// Should not be banned (whitelisted)
	if ps.IsBanned(ip) {
		t.Error("Whitelisted IP should not be banned")
	}
}

func TestIsWhitelisted(t *testing.T) {
	cfg := DefaultConfig()
	ps := NewPolicyServer(cfg, nil)

	ip := "192.168.1.100"

	// Initially not whitelisted
	if ps.IsWhitelisted(ip) {
		t.Error("IP should not be whitelisted initially")
	}

	// Add to whitelist
	ps.AddToWhitelist(ip)

	// Should be whitelisted now
	if !ps.IsWhitelisted(ip) {
		t.Error("IP should be whitelisted after AddToWhitelist")
	}
}

func TestIsBlacklisted(t *testing.T) {
	cfg := DefaultConfig()
	ps := NewPolicyServer(cfg, nil)

	addr := "tos1testaddress"

	// Initially not blacklisted
	if ps.IsBlacklisted(addr) {
		t.Error("Address should not be blacklisted initially")
	}

	// Add to blacklist
	ps.AddToBlacklist(addr)

	// Should be blacklisted now
	if !ps.IsBlacklisted(addr) {
		t.Error("Address should be blacklisted after AddToBlacklist")
	}

	// Test case insensitivity
	if !ps.IsBlacklisted("TOS1TESTADDRESS") {
		t.Error("Blacklist should be case-insensitive")
	}
}

func TestGetStats(t *testing.T) {
	cfg := DefaultConfig()
	ps := NewPolicyServer(cfg, nil)

	// Initially no stats
	total, banned := ps.GetStats()
	if total != 0 {
		t.Errorf("Total = %d, want 0", total)
	}
	if banned != 0 {
		t.Errorf("Banned = %d, want 0", banned)
	}

	// Create some stats
	ps.getStats("192.168.1.1")
	ps.getStats("192.168.1.2")
	ps.BanIP("192.168.1.3")

	total, banned = ps.GetStats()
	if total != 3 {
		t.Errorf("Total = %d, want 3", total)
	}
	if banned != 1 {
		t.Errorf("Banned = %d, want 1", banned)
	}
}

func TestIPStatsStruct(t *testing.T) {
	stats := &IPStats{
		LastBeat:      time.Now().UnixMilli(),
		ValidShares:   10,
		InvalidShares: 5,
		Malformed:     2,
		ConnLimit:     100,
		Score:         50,
	}

	if stats.ValidShares != 10 {
		t.Errorf("ValidShares = %d, want 10", stats.ValidShares)
	}

	if stats.InvalidShares != 5 {
		t.Errorf("InvalidShares = %d, want 5", stats.InvalidShares)
	}

	if stats.Score != 50 {
		t.Errorf("Score = %d, want 50", stats.Score)
	}
}

func TestConcurrentAccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConnectionLimit = 1000
	ps := NewPolicyServer(cfg, nil)
	ps.startedAt = 0

	var wg sync.WaitGroup
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	// Concurrent access from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := ips[id%len(ips)]

			for j := 0; j < 100; j++ {
				ps.IsBanned(ip)
				ps.ApplyConnectionLimit(ip)
				ps.ApplySharePolicy(ip, j%2 == 0)
				ps.AddScore(ip, 1)
				ps.GetScore(ip)
			}
		}(i)
	}

	wg.Wait()

	// Should not panic or deadlock
	total, _ := ps.GetStats()
	if total == 0 {
		t.Error("Should have tracked some IPs")
	}
}

func BenchmarkIsBanned(b *testing.B) {
	cfg := DefaultConfig()
	ps := NewPolicyServer(cfg, nil)
	ip := "192.168.1.100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ps.IsBanned(ip)
	}
}

func BenchmarkApplySharePolicy(b *testing.B) {
	cfg := DefaultConfig()
	cfg.CheckThreshold = 1000000 // Prevent banning during benchmark
	ps := NewPolicyServer(cfg, nil)
	ip := "192.168.1.100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ps.ApplySharePolicy(ip, true)
	}
}

func BenchmarkAddScore(b *testing.B) {
	cfg := DefaultConfig()
	cfg.MaxScore = 1000000 // Prevent banning during benchmark
	ps := NewPolicyServer(cfg, nil)
	ip := "192.168.1.100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ps.AddScore(ip, 1)
	}
}
