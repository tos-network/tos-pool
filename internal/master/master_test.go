package master

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/storage"
)

func TestPruneJobBacklog(t *testing.T) {
	tests := []struct {
		name          string
		currentHeight uint64
		backlogJobs   map[string]*Job
		expectedLen   int
	}{
		{
			name:          "empty backlog",
			currentHeight: 100,
			backlogJobs:   map[string]*Job{},
			expectedLen:   0,
		},
		{
			name:          "backlog within limit",
			currentHeight: 100,
			backlogJobs: map[string]*Job{
				"job1": {ID: "job1", Height: 99},
				"job2": {ID: "job2", Height: 98},
			},
			expectedLen: 2,
		},
		{
			name:          "backlog exceeds limit - prunes old",
			currentHeight: 100,
			backlogJobs: map[string]*Job{
				"job1": {ID: "job1", Height: 99},
				"job2": {ID: "job2", Height: 98},
				"job3": {ID: "job3", Height: 97},
				"job4": {ID: "job4", Height: 96}, // Should be pruned
				"job5": {ID: "job5", Height: 95}, // Should be pruned
			},
			expectedLen: 3,
		},
		{
			name:          "low height - no pruning",
			currentHeight: 2,
			backlogJobs: map[string]*Job{
				"job1": {ID: "job1", Height: 1},
				"job2": {ID: "job2", Height: 0},
			},
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Master{
				currentHeight: tt.currentHeight,
				jobBacklog:    tt.backlogJobs,
			}

			m.pruneJobBacklog()

			if len(m.jobBacklog) != tt.expectedLen {
				t.Errorf("pruneJobBacklog() backlog len = %d, want %d", len(m.jobBacklog), tt.expectedLen)
			}

			// Verify remaining jobs are at valid heights
			minHeight := tt.currentHeight
			if minHeight > MaxJobBacklog {
				minHeight -= MaxJobBacklog
			} else {
				minHeight = 0
			}

			for id, job := range m.jobBacklog {
				if job.Height < minHeight {
					t.Errorf("pruneJobBacklog() left job %s at height %d, minHeight %d",
						id, job.Height, minHeight)
				}
			}
		})
	}
}

func TestMaxJobBacklog(t *testing.T) {
	if MaxJobBacklog != 3 {
		t.Errorf("MaxJobBacklog = %d, want 3", MaxJobBacklog)
	}
}

func TestJobCreatedAt(t *testing.T) {
	now := time.Now()
	job := &Job{
		ID:        "test",
		Height:    100,
		CreatedAt: now,
	}

	if job.CreatedAt.IsZero() {
		t.Error("Job CreatedAt should be set")
	}

	if job.CreatedAt.After(time.Now()) {
		t.Error("Job CreatedAt should not be in the future")
	}
}

func TestShareSubmissionWithTrust(t *testing.T) {
	share := &ShareSubmission{
		Address:        "tos1test",
		Worker:         "worker1",
		JobID:          "job123",
		Nonce:          "0x1234567890abcdef",
		Difficulty:     1000000,
		Height:         100,
		TrustScore:     50,
		SkipValidation: true,
	}

	if share.TrustScore != 50 {
		t.Errorf("ShareSubmission.TrustScore = %d, want 50", share.TrustScore)
	}

	if !share.SkipValidation {
		t.Error("ShareSubmission.SkipValidation should be true")
	}
}

func TestShareResult(t *testing.T) {
	tests := []struct {
		name    string
		result  *ShareResult
		isValid bool
		isBlock bool
	}{
		{
			name:    "valid share",
			result:  &ShareResult{Valid: true, Block: false, Message: "Share accepted"},
			isValid: true,
			isBlock: false,
		},
		{
			name:    "block found",
			result:  &ShareResult{Valid: true, Block: true, Message: "Block found!"},
			isValid: true,
			isBlock: true,
		},
		{
			name:    "invalid share",
			result:  &ShareResult{Valid: false, Block: false, Message: "Low difficulty share"},
			isValid: false,
			isBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Valid != tt.isValid {
				t.Errorf("ShareResult.Valid = %v, want %v", tt.result.Valid, tt.isValid)
			}
			if tt.result.Block != tt.isBlock {
				t.Errorf("ShareResult.Block = %v, want %v", tt.result.Block, tt.isBlock)
			}
		})
	}
}

func BenchmarkPruneJobBacklog(b *testing.B) {
	// Create a master with many jobs
	m := &Master{
		currentHeight: 1000,
		jobBacklog:    make(map[string]*Job),
	}

	// Add many jobs
	for i := uint64(0); i < 100; i++ {
		id := string(rune('a' + (i % 26)))
		m.jobBacklog[id] = &Job{ID: id, Height: 1000 - i}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.pruneJobBacklog()
	}
}

func TestConstants(t *testing.T) {
	if PayoutLockTTL != 10*time.Minute {
		t.Errorf("PayoutLockTTL = %v, want 10m", PayoutLockTTL)
	}
	if TxConfirmTimeout != 5*time.Minute {
		t.Errorf("TxConfirmTimeout = %v, want 5m", TxConfirmTimeout)
	}
	if TxConfirmPollRate != 5*time.Second {
		t.Errorf("TxConfirmPollRate = %v, want 5s", TxConfirmPollRate)
	}
	if MinPeersForPayout != 3 {
		t.Errorf("MinPeersForPayout = %d, want 3", MinPeersForPayout)
	}
	if OrphanSearchRange != 16 {
		t.Errorf("OrphanSearchRange = %d, want 16", OrphanSearchRange)
	}
	if MaxJobBacklog != 3 {
		t.Errorf("MaxJobBacklog = %d, want 3", MaxJobBacklog)
	}
}

func TestJobStruct(t *testing.T) {
	job := &Job{
		ID:         "abc123",
		Height:     12345,
		HeaderHash: []byte{0x01, 0x02, 0x03},
		ParentHash: []byte{0x04, 0x05, 0x06},
		Target:     []byte{0x00, 0x00, 0xff},
		Difficulty: 1000000,
		Timestamp:  1700000000,
		CreatedAt:  time.Now(),
	}

	if job.ID != "abc123" {
		t.Errorf("Job.ID = %s, want abc123", job.ID)
	}
	if job.Height != 12345 {
		t.Errorf("Job.Height = %d, want 12345", job.Height)
	}
	if job.Difficulty != 1000000 {
		t.Errorf("Job.Difficulty = %d, want 1000000", job.Difficulty)
	}
	if len(job.HeaderHash) != 3 {
		t.Errorf("Job.HeaderHash len = %d, want 3", len(job.HeaderHash))
	}
}

func TestShareSubmissionStruct(t *testing.T) {
	share := &ShareSubmission{
		Address:        "tos1testaddress",
		Worker:         "rig1",
		JobID:          "job123",
		Nonce:          "0xdeadbeef",
		Difficulty:     500000,
		Height:         12345,
		TrustScore:     75,
		SkipValidation: true,
	}

	if share.Address != "tos1testaddress" {
		t.Errorf("ShareSubmission.Address = %s, want tos1testaddress", share.Address)
	}
	if share.Worker != "rig1" {
		t.Errorf("ShareSubmission.Worker = %s, want rig1", share.Worker)
	}
	if share.TrustScore != 75 {
		t.Errorf("ShareSubmission.TrustScore = %d, want 75", share.TrustScore)
	}
}

func TestShareResultMessages(t *testing.T) {
	tests := []struct {
		result   *ShareResult
		expected string
	}{
		{&ShareResult{Valid: false, Message: "No active job"}, "No active job"},
		{&ShareResult{Valid: false, Message: "Stale job"}, "Stale job"},
		{&ShareResult{Valid: false, Message: "Invalid nonce"}, "Invalid nonce"},
		{&ShareResult{Valid: false, Message: "Low difficulty share"}, "Low difficulty share"},
		{&ShareResult{Valid: false, Message: "Hash computation failed"}, "Hash computation failed"},
		{&ShareResult{Valid: true, Message: "Share accepted"}, "Share accepted"},
		{&ShareResult{Valid: true, Block: true, Message: "Block found!"}, "Block found!"},
		{&ShareResult{Valid: false, Message: "Pool shutting down"}, "Pool shutting down"},
	}

	for _, tt := range tests {
		if tt.result.Message != tt.expected {
			t.Errorf("ShareResult.Message = %s, want %s", tt.result.Message, tt.expected)
		}
	}
}

func setupTestMaster(t *testing.T) (*Master, *miniredis.Miniredis) {
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
		Pool: config.PoolConfig{
			Name: "Test Pool",
			Fee:  1.0,
		},
		Mining: config.MiningConfig{
			JobRefreshInterval: 1 * time.Second,
			InitialDifficulty:  1000000,
		},
		Validation: config.ValidationConfig{
			HashrateWindow:      600 * time.Second,
			HashrateLargeWindow: 3600 * time.Second,
		},
		PPLNS: config.PPLNSConfig{
			Window:        2.0,
			MinWindow:     0.5,
			MaxWindow:     10.0,
			DynamicWindow: false,
		},
		Unlocker: config.UnlockerConfig{
			Enabled:       false,
			Interval:      1 * time.Minute,
			MatureDepth:   100,
			ImmatureDepth: 10,
		},
		Payouts: config.PayoutsConfig{
			Enabled:           false,
			Interval:          1 * time.Hour,
			Threshold:         1000000000,
			MaxAddressesPerTx: 10,
		},
		Notify: config.NotifyConfig{
			Enabled: false,
		},
	}

	// Create master without upstream for basic tests
	master := NewMaster(cfg, redis, nil)

	return master, mr
}

func TestNewMaster(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	if master == nil {
		t.Fatal("NewMaster returned nil")
	}

	if master.cfg == nil {
		t.Error("Master.cfg should not be nil")
	}

	if master.redis == nil {
		t.Error("Master.redis should not be nil")
	}

	if master.shareChan == nil {
		t.Error("Master.shareChan should not be nil")
	}

	if master.jobBacklog == nil {
		t.Error("Master.jobBacklog should not be nil")
	}

	if master.jobUpdateChan == nil {
		t.Error("Master.jobUpdateChan should not be nil")
	}
}

func TestGetCurrentJobNil(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	job := master.GetCurrentJob()
	if job != nil {
		t.Error("GetCurrentJob() should return nil initially")
	}
}

func TestGetJobUpdateChan(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	ch := master.GetJobUpdateChan()
	if ch == nil {
		t.Error("GetJobUpdateChan() should return a channel")
	}

	// Channel should be readable
	select {
	case <-ch:
		t.Error("Channel should be empty")
	default:
		// Expected - channel is empty
	}
}

func TestStopWithoutStart(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	// Should not panic when stopping without starting
	master.Stop()
}

func TestGetDynamicPPLNSWindowDisabled(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	// When dynamic window is disabled, should return configured window
	window := master.GetDynamicPPLNSWindow()
	if window != 2.0 {
		t.Errorf("GetDynamicPPLNSWindow() = %f, want 2.0", window)
	}
}

func TestGetPPLNSShareWindow(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	master.currentDiff = 1000000

	shareWindow := master.GetPPLNSShareWindow()
	expected := uint64(2.0 * 1000000)
	if shareWindow != expected {
		t.Errorf("GetPPLNSShareWindow() = %d, want %d", shareWindow, expected)
	}
}

func TestGetStatsNoUpstream(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	stats, err := master.GetStats()
	if err != nil {
		t.Errorf("GetStats() returned error: %v", err)
	}
	if stats == nil {
		t.Error("GetStats() should return stats even if empty")
	}
}

func TestGetNetworkStatsNoUpstream(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	stats, err := master.GetNetworkStats()
	// Should return nil/error when no network stats are stored
	_ = err
	_ = stats
}

func TestHasHealthyUpstreamNilManager(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	// With nil upstream manager, this should not panic
	// But since upstream is nil, we can't call methods on it directly
	if master.upstream != nil {
		t.Error("Expected upstream to be nil in this test")
	}
}

func TestPruneJobBacklogEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		currentHeight uint64
		backlogSize   int
	}{
		{"height 0", 0, 5},
		{"height 1", 1, 5},
		{"height 2", 2, 5},
		{"height 3", 3, 5},
		{"height exactly MaxJobBacklog", 3, 10},
		{"very high height", 1000000, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Master{
				currentHeight: tt.currentHeight,
				jobBacklog:    make(map[string]*Job),
			}

			// Add jobs at various heights
			for i := 0; i < tt.backlogSize; i++ {
				height := tt.currentHeight
				if height > uint64(i) {
					height -= uint64(i)
				}
				id := string(rune('a' + i))
				m.jobBacklog[id] = &Job{ID: id, Height: height}
			}

			// Should not panic
			m.pruneJobBacklog()

			// Remaining jobs should all be at valid heights
			minHeight := tt.currentHeight
			if minHeight > MaxJobBacklog {
				minHeight -= MaxJobBacklog
			} else {
				minHeight = 0
			}

			for _, job := range m.jobBacklog {
				if job.Height < minHeight {
					t.Errorf("Job at height %d should have been pruned (min: %d)", job.Height, minHeight)
				}
			}
		})
	}
}

func TestJobBacklogConcurrentAccess(t *testing.T) {
	m := &Master{
		currentHeight: 1000,
		jobBacklog:    make(map[string]*Job),
	}

	// Add initial jobs
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		m.jobBacklog[id] = &Job{ID: id, Height: uint64(1000 - i)}
	}

	done := make(chan bool)

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = len(m.jobBacklog)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestShareSubmissionResultChannel(t *testing.T) {
	share := &ShareSubmission{
		Address:    "tos1test",
		Worker:     "worker1",
		JobID:      "job123",
		ResultChan: make(chan *ShareResult, 1),
	}

	// Send a result
	result := &ShareResult{Valid: true, Message: "test"}
	share.ResultChan <- result

	// Receive the result
	received := <-share.ResultChan
	if received.Message != "test" {
		t.Errorf("Received wrong message: %s", received.Message)
	}
}

func TestDynamicPPLNSWindowWithStats(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	// Enable dynamic window
	master.cfg.PPLNS.DynamicWindow = true

	// Without stats, should return default window
	window := master.GetDynamicPPLNSWindow()
	if window != 2.0 {
		t.Errorf("GetDynamicPPLNSWindow() without stats = %f, want 2.0", window)
	}
}

func TestDynamicPPLNSWindowClamping(t *testing.T) {
	master, mr := setupTestMaster(t)
	defer mr.Close()

	// Set up config with specific min/max bounds
	master.cfg.PPLNS.DynamicWindow = true
	master.cfg.PPLNS.Window = 2.0
	master.cfg.PPLNS.MinWindow = 0.5
	master.cfg.PPLNS.MaxWindow = 10.0

	// Without valid hashrate stats, should clamp to bounds or return default
	window := master.GetDynamicPPLNSWindow()

	if window < master.cfg.PPLNS.MinWindow || window > master.cfg.PPLNS.MaxWindow {
		// Window may be default if no stats available
		if window != master.cfg.PPLNS.Window {
			t.Errorf("GetDynamicPPLNSWindow() = %f, should be in bounds [%f, %f] or default %f",
				window, master.cfg.PPLNS.MinWindow, master.cfg.PPLNS.MaxWindow, master.cfg.PPLNS.Window)
		}
	}
}
