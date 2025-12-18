package slave

import (
	"testing"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
)

func TestShouldSkipValidation(t *testing.T) {
	tests := []struct {
		name              string
		trustThreshold    int
		trustCheckPercent int
		sessionTrustScore int
		iterations        int
		expectSkipRate    float64 // Expected skip rate (0.0-1.0)
		tolerance         float64 // Tolerance for skip rate
	}{
		{
			name:              "no trust threshold - always validate",
			trustThreshold:    0,
			trustCheckPercent: 75,
			sessionTrustScore: 100,
			iterations:        100,
			expectSkipRate:    0.0,
			tolerance:         0.0,
		},
		{
			name:              "below threshold - always validate",
			trustThreshold:    50,
			trustCheckPercent: 75,
			sessionTrustScore: 25,
			iterations:        100,
			expectSkipRate:    0.0,
			tolerance:         0.0,
		},
		{
			name:              "at threshold - validate 75%, skip 25%",
			trustThreshold:    50,
			trustCheckPercent: 75,
			sessionTrustScore: 50,
			iterations:        1000,
			expectSkipRate:    0.25,
			tolerance:         0.05,
		},
		{
			name:              "trusted - validate 50%, skip 50%",
			trustThreshold:    50,
			trustCheckPercent: 50,
			sessionTrustScore: 100,
			iterations:        1000,
			expectSkipRate:    0.50,
			tolerance:         0.05,
		},
		{
			name:              "100% check - never skip",
			trustThreshold:    50,
			trustCheckPercent: 100,
			sessionTrustScore: 100,
			iterations:        100,
			expectSkipRate:    0.0,
			tolerance:         0.0,
		},
		{
			name:              "0% check - always skip",
			trustThreshold:    50,
			trustCheckPercent: 0,
			sessionTrustScore: 100,
			iterations:        100,
			expectSkipRate:    1.0,
			tolerance:         0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Validation: config.ValidationConfig{
					TrustThreshold:    tt.trustThreshold,
					TrustCheckPercent: tt.trustCheckPercent,
				},
			}

			s := &StratumServer{cfg: cfg}
			session := &Session{TrustScore: tt.sessionTrustScore}

			skipped := 0
			for i := 0; i < tt.iterations; i++ {
				if s.shouldSkipValidation(session) {
					skipped++
				}
			}

			skipRate := float64(skipped) / float64(tt.iterations)

			if tt.tolerance == 0 {
				// Exact match expected
				if skipRate != tt.expectSkipRate {
					t.Errorf("skipRate = %v, want exactly %v", skipRate, tt.expectSkipRate)
				}
			} else {
				// Within tolerance
				diff := skipRate - tt.expectSkipRate
				if diff < 0 {
					diff = -diff
				}
				if diff > tt.tolerance {
					t.Errorf("skipRate = %v, want %v±%v", skipRate, tt.expectSkipRate, tt.tolerance)
				}
			}
		})
	}
}

func TestShareStruct(t *testing.T) {
	share := &Share{
		SessionID:      123,
		Address:        "tos1testaddress",
		Worker:         "worker1",
		JobID:          "job456",
		Nonce:          "abcdef1234567890", // No 0x prefix for Stratum
		Hash:           "1234567890abcdef", // No 0x prefix for Stratum
		Difficulty:     1000000,
		Height:         12345,
		Timestamp:      time.Now().Unix(),
		IsBlock:        false,
		TrustScore:     75,
		SkipValidation: true,
	}

	// Test all fields
	if share.SessionID != 123 {
		t.Errorf("Share.SessionID = %d, want 123", share.SessionID)
	}
	if share.TrustScore != 75 {
		t.Errorf("Share.TrustScore = %d, want 75", share.TrustScore)
	}
	if !share.SkipValidation {
		t.Error("Share.SkipValidation should be true")
	}
}

func TestVardiffStats(t *testing.T) {
	stats := &VardiffStats{
		LastRetarget:   time.Now(),
		SharesSince:    10,
		MinerRequested: true,
	}

	if stats.SharesSince != 10 {
		t.Errorf("VardiffStats.SharesSince = %d, want 10", stats.SharesSince)
	}

	if !stats.MinerRequested {
		t.Error("VardiffStats.MinerRequested should be true")
	}
}

func TestJobStruct(t *testing.T) {
	now := time.Now()
	job := &Job{
		ID:         "job123",
		Height:     12345,
		HeaderHash: "abcdef1234567890",        // No 0x prefix for Stratum
		ParentHash: "123456abcdef7890",        // No 0x prefix for Stratum
		Target:     "00001234567890abcdef",    // No 0x prefix for Stratum
		Difficulty: 1000000,
		Timestamp:  12345678,
		CleanJobs:  true,
		CreatedAt:  now,
	}

	if job.ID != "job123" {
		t.Errorf("Job.ID = %s, want job123", job.ID)
	}

	if job.Height != 12345 {
		t.Errorf("Job.Height = %d, want 12345", job.Height)
	}

	if !job.CleanJobs {
		t.Error("Job.CleanJobs should be true")
	}
}

func TestSessionStruct(t *testing.T) {
	session := &Session{
		ID:              1,
		Address:         "tos1test",
		Worker:          "rig1",
		Authorized:      true,
		Difficulty:      1000000,
		ExtraNonce1:     "12345678",
		ExtraNonce2Size: 4,
		ValidShares:     100,
		InvalidShares:   5,
		StaleShares:     2,
		TrustScore:      75,
		RemoteAddr:      "192.168.1.1:12345",
		ConnectedAt:     time.Now(),
	}

	if session.TrustScore != 75 {
		t.Errorf("Session.TrustScore = %d, want 75", session.TrustScore)
	}

	if session.ValidShares != 100 {
		t.Errorf("Session.ValidShares = %d, want 100", session.ValidShares)
	}

	if session.InvalidShares != 5 {
		t.Errorf("Session.InvalidShares = %d, want 5", session.InvalidShares)
	}
}

func TestParseWorkerID(t *testing.T) {
	tests := []struct {
		input          string
		expectAddress  string
		expectWorker   string
	}{
		{"tos1abc.worker1", "tos1abc", "worker1"},
		{"tos1abc.rig.secondary", "tos1abc", "rig.secondary"},
		{"tos1abc", "tos1abc", "default"},
		{".worker", "", "worker"},
		{"", "", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			addr, worker := parseWorkerID(tt.input)
			if addr != tt.expectAddress {
				t.Errorf("parseWorkerID(%q) address = %q, want %q", tt.input, addr, tt.expectAddress)
			}
			if worker != tt.expectWorker {
				t.Errorf("parseWorkerID(%q) worker = %q, want %q", tt.input, worker, tt.expectWorker)
			}
		})
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1:12345", "192.168.1.1"},
		{"10.0.0.1:80", "10.0.0.1"},
		{"[::1]:12345", "::1"},
		{"[2001:db8::1]:8080", "2001:db8::1"},
		{"127.0.0.1", "127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractIP(tt.input)
			if result != tt.expected {
				t.Errorf("extractIP(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func BenchmarkShouldSkipValidation(b *testing.B) {
	cfg := &config.Config{
		Validation: config.ValidationConfig{
			TrustThreshold:    50,
			TrustCheckPercent: 75,
		},
	}

	s := &StratumServer{cfg: cfg}
	session := &Session{TrustScore: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.shouldSkipValidation(session)
	}
}

func BenchmarkParseWorkerID(b *testing.B) {
	input := "tos1abcdefghijklmnop.worker1"
	for i := 0; i < b.N; i++ {
		parseWorkerID(input)
	}
}

func BenchmarkExtractIP(b *testing.B) {
	input := "192.168.1.100:12345"
	for i := 0; i < b.N; i++ {
		extractIP(input)
	}
}

func TestMaxRequestConstants(t *testing.T) {
	if MaxRequestSize != 1024 {
		t.Errorf("MaxRequestSize = %d, want 1024", MaxRequestSize)
	}
	if MaxRequestBuffer != MaxRequestSize+64 {
		t.Errorf("MaxRequestBuffer = %d, want %d", MaxRequestBuffer, MaxRequestSize+64)
	}
}

func TestNewStratumServer(t *testing.T) {
	cfg := &config.Config{
		Slave: config.SlaveConfig{
			StratumBind: ":3333",
		},
	}

	server := NewStratumServer(cfg, nil)

	if server == nil {
		t.Fatal("NewStratumServer returned nil")
	}

	if server.cfg != cfg {
		t.Error("Server.cfg not set correctly")
	}

	if server.quit == nil {
		t.Error("Server.quit channel should be initialized")
	}
}

func TestSetShareCallback(t *testing.T) {
	cfg := &config.Config{}
	server := NewStratumServer(cfg, nil)

	server.SetShareCallback(func(s *Share) {
		// Callback set
	})

	if server.onShare == nil {
		t.Error("onShare callback should be set")
	}
}

func TestSetBlockCallback(t *testing.T) {
	cfg := &config.Config{}
	server := NewStratumServer(cfg, nil)

	server.SetBlockCallback(func(s *Share) {
		// Callback set
	})

	if server.onBlock == nil {
		t.Error("onBlock callback should be set")
	}
}

func TestStratumRequestStruct(t *testing.T) {
	req := StratumRequest{
		ID:     1,
		Method: "mining.subscribe",
		Params: []interface{}{"TOS-miner/1.0"},
	}

	if req.Method != "mining.subscribe" {
		t.Errorf("StratumRequest.Method = %s, want mining.subscribe", req.Method)
	}

	if len(req.Params) != 1 {
		t.Errorf("StratumRequest.Params len = %d, want 1", len(req.Params))
	}
}

func TestStratumResponseStruct(t *testing.T) {
	resp := StratumResponse{
		ID:     1,
		Result: true,
		Error:  nil,
	}

	if resp.ID != 1 {
		t.Errorf("StratumResponse.ID = %v, want 1", resp.ID)
	}

	if resp.Error != nil {
		t.Error("StratumResponse.Error should be nil")
	}
}

func TestStratumResponseWithError(t *testing.T) {
	resp := StratumResponse{
		ID:     1,
		Result: nil,
		Error:  []interface{}{24, "Unauthorized worker", nil},
	}

	if resp.Result != nil {
		t.Error("StratumResponse.Result should be nil")
	}

	errSlice, ok := resp.Error.([]interface{})
	if !ok {
		t.Error("StratumResponse.Error should be a slice")
	}

	if errSlice[0].(int) != 24 {
		t.Errorf("Error code = %v, want 24", errSlice[0])
	}
}

func TestStratumNotifyStruct(t *testing.T) {
	notify := StratumNotify{
		ID:     nil,
		Method: "mining.notify",
		Params: []interface{}{"job1", "header", "target"},
	}

	if notify.Method != "mining.notify" {
		t.Errorf("StratumNotify.Method = %s, want mining.notify", notify.Method)
	}

	if notify.ID != nil {
		t.Error("StratumNotify.ID should be nil for notifications")
	}
}

func TestSessionVardiffStatsInit(t *testing.T) {
	session := &Session{
		ID:         1,
		Difficulty: 1000000,
		VardiffStats: &VardiffStats{
			LastRetarget:   time.Now(),
			SharesSince:    0,
			MinerRequested: false,
		},
	}

	if session.VardiffStats == nil {
		t.Error("Session.VardiffStats should not be nil")
	}

	if session.VardiffStats.SharesSince != 0 {
		t.Errorf("VardiffStats.SharesSince = %d, want 0", session.VardiffStats.SharesSince)
	}
}

func TestShareIsBlock(t *testing.T) {
	normalShare := &Share{
		Address:    "tos1test",
		Difficulty: 1000000,
		IsBlock:    false,
	}

	blockShare := &Share{
		Address:    "tos1test",
		Difficulty: 1000000,
		IsBlock:    true,
	}

	if normalShare.IsBlock {
		t.Error("Normal share should not be a block")
	}

	if !blockShare.IsBlock {
		t.Error("Block share should be marked as block")
	}
}

func TestParseWorkerIDEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectAddress  string
		expectWorker   string
	}{
		{"dots in worker name", "tos1addr.worker.name.with.dots", "tos1addr", "worker.name.with.dots"},
		{"numeric worker", "tos1addr.12345", "tos1addr", "12345"},
		{"underscores", "tos1addr.worker_1_gpu", "tos1addr", "worker_1_gpu"},
		{"mixed case", "tos1addr.WorkerOne", "tos1addr", "WorkerOne"},
		{"empty worker part", "tos1addr.", "tos1addr", ""},
		{"just dot", ".", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, worker := parseWorkerID(tt.input)
			if addr != tt.expectAddress {
				t.Errorf("parseWorkerID(%q) address = %q, want %q", tt.input, addr, tt.expectAddress)
			}
			if worker != tt.expectWorker {
				t.Errorf("parseWorkerID(%q) worker = %q, want %q", tt.input, worker, tt.expectWorker)
			}
		})
	}
}

func TestExtractIPEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"localhost ipv6", "[::1]:8080", "::1"},
		{"full ipv6", "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:443", "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
		{"ipv4 no port", "1.2.3.4", "1.2.3.4"},
		{"high port", "192.168.1.1:65535", "192.168.1.1"},
		{"port 1", "10.0.0.1:1", "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractIP(tt.input)
			if result != tt.expected {
				t.Errorf("extractIP(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSessionAuthorization(t *testing.T) {
	session := &Session{
		ID:         1,
		Authorized: false,
	}

	if session.Authorized {
		t.Error("Session should not be authorized initially")
	}

	session.Authorized = true
	if !session.Authorized {
		t.Error("Session should be authorized after setting")
	}
}

func TestSessionShareCounts(t *testing.T) {
	session := &Session{
		ID:            1,
		ValidShares:   0,
		InvalidShares: 0,
		StaleShares:   0,
	}

	// Simulate share processing
	session.ValidShares++
	session.ValidShares++
	session.InvalidShares++
	session.StaleShares++

	if session.ValidShares != 2 {
		t.Errorf("ValidShares = %d, want 2", session.ValidShares)
	}
	if session.InvalidShares != 1 {
		t.Errorf("InvalidShares = %d, want 1", session.InvalidShares)
	}
	if session.StaleShares != 1 {
		t.Errorf("StaleShares = %d, want 1", session.StaleShares)
	}
}

func TestJobCreatedAtTracking(t *testing.T) {
	before := time.Now()
	job := &Job{
		ID:        "test",
		CreatedAt: time.Now(),
	}
	after := time.Now()

	if job.CreatedAt.Before(before) {
		t.Error("Job.CreatedAt should not be before creation time")
	}
	if job.CreatedAt.After(after) {
		t.Error("Job.CreatedAt should not be after test completion")
	}
}

func TestTrustScoreBoundaries(t *testing.T) {
	cfg := &config.Config{
		Validation: config.ValidationConfig{
			TrustThreshold:    50,
			TrustCheckPercent: 50,
		},
	}

	server := &StratumServer{cfg: cfg}

	// Test boundary at exactly threshold
	sessionAtThreshold := &Session{TrustScore: 50}
	// At threshold, some should skip (probability test)

	// Test just below threshold - should never skip
	sessionBelowThreshold := &Session{TrustScore: 49}
	skipped := false
	for i := 0; i < 100; i++ {
		if server.shouldSkipValidation(sessionBelowThreshold) {
			skipped = true
			break
		}
	}
	if skipped {
		t.Error("Session below threshold should never skip validation")
	}

	// Test zero trust score
	sessionZeroTrust := &Session{TrustScore: 0}
	for i := 0; i < 100; i++ {
		if server.shouldSkipValidation(sessionZeroTrust) {
			t.Error("Session with zero trust should never skip validation")
			break
		}
	}

	_ = sessionAtThreshold // Used for threshold boundary awareness
}

func TestShouldSkipValidationDisabled(t *testing.T) {
	// Zero threshold means disabled
	cfg := &config.Config{
		Validation: config.ValidationConfig{
			TrustThreshold:    0,
			TrustCheckPercent: 50,
		},
	}

	server := &StratumServer{cfg: cfg}
	session := &Session{TrustScore: 1000} // Very high trust score

	// With threshold 0, should never skip
	for i := 0; i < 100; i++ {
		if server.shouldSkipValidation(session) {
			t.Error("Should never skip when threshold is 0")
			break
		}
	}
}
