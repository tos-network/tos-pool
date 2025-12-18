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
		Nonce:          "0xabcdef1234567890",
		Hash:           "0x1234567890abcdef",
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
		HeaderHash: "0xabcdef",
		ParentHash: "0x123456",
		Target:     "0x00001234",
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
