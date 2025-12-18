package master

import (
	"testing"
	"time"
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
