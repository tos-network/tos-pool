package util

import (
	"math/big"
	"testing"
)

func TestDifficultyToTarget(t *testing.T) {
	tests := []struct {
		difficulty uint64
	}{
		{1},
		{1000},
		{1000000},
		{1000000000},
	}

	for _, tt := range tests {
		target := DifficultyToTarget(tt.difficulty)
		if target == nil {
			t.Errorf("DifficultyToTarget(%d) returned nil", tt.difficulty)
			continue
		}
		if target.Sign() <= 0 {
			t.Errorf("DifficultyToTarget(%d) returned non-positive target", tt.difficulty)
		}
	}

	// Test zero difficulty
	target := DifficultyToTarget(0)
	if target.Cmp(MaxTarget) != 0 {
		t.Error("DifficultyToTarget(0) should return MaxTarget")
	}
}

func TestTargetToDifficulty(t *testing.T) {
	// Test round-trip
	difficulties := []uint64{1, 100, 10000, 1000000}

	for _, diff := range difficulties {
		target := DifficultyToTarget(diff)
		recovered := TargetToDifficulty(target)

		// Allow some rounding error
		if recovered < diff/2 || recovered > diff*2 {
			t.Errorf("Round-trip failed for difficulty %d: got %d", diff, recovered)
		}
	}

	// Test zero target
	zeroTarget := big.NewInt(0)
	if TargetToDifficulty(zeroTarget) != 0 {
		t.Error("TargetToDifficulty(0) should return 0")
	}
}

func TestHashToDifficulty(t *testing.T) {
	// Test with zero hash - should return 0 (can't divide by zero)
	zeroHash := make([]byte, 32)
	diff := HashToDifficulty(zeroHash)
	if diff != 0 {
		t.Errorf("HashToDifficulty(zero) should return 0, got %d", diff)
	}

	// Test with high hash (low difficulty)
	highHash := make([]byte, 32)
	highHash[0] = 0xFF
	highHash[1] = 0xFF
	diff = HashToDifficulty(highHash)
	// High hash value means low difficulty - just verify it's non-negative
	t.Logf("High hash difficulty: %d", diff)

	// Test with invalid hash length
	shortHash := make([]byte, 16)
	diff = HashToDifficulty(shortHash)
	if diff != 0 {
		t.Error("HashToDifficulty with invalid length should return 0")
	}
}

func TestHashMeetsTarget(t *testing.T) {
	// Low hash meets high target
	lowHash := make([]byte, 32)
	lowHash[31] = 0x01

	highTarget := new(big.Int).SetBytes([]byte{0xFF, 0xFF, 0xFF, 0xFF})

	if !HashMeetsTarget(lowHash, highTarget) {
		t.Error("Low hash should meet high target")
	}

	// High hash doesn't meet low target
	highHash := make([]byte, 32)
	highHash[0] = 0xFF

	lowTarget := new(big.Int).SetBytes([]byte{0x00, 0x00, 0x01})

	if HashMeetsTarget(highHash, lowTarget) {
		t.Error("High hash should not meet low target")
	}
}

func TestHashMeetsDifficulty(t *testing.T) {
	// Create a very low hash (high difficulty)
	hash := make([]byte, 32)
	hash[0] = 0x00
	hash[1] = 0x00
	hash[2] = 0x00
	hash[3] = 0x01

	// Calculate what difficulty this hash represents
	actualDiff := HashToDifficulty(hash)
	t.Logf("Hash difficulty: %d", actualDiff)

	// Should meet lower difficulties
	if actualDiff > 1 && !HashMeetsDifficulty(hash, 1) {
		t.Error("Low hash should meet difficulty 1")
	}
}

func TestNetworkHashrate(t *testing.T) {
	// Test basic calculation
	difficulty := uint64(1000000000000)
	blockTime := 15.0 // 15 seconds

	hashrate := NetworkHashrate(difficulty, blockTime)
	expected := float64(difficulty) / blockTime

	if hashrate != expected {
		t.Errorf("NetworkHashrate: got %f, want %f", hashrate, expected)
	}

	// Test zero block time
	hashrate = NetworkHashrate(difficulty, 0)
	if hashrate != 0 {
		t.Error("NetworkHashrate with zero block time should return 0")
	}
}

func TestEstimatedTimeToBlock(t *testing.T) {
	hashrate := 1000000.0 // 1 MH/s
	difficulty := uint64(1000000000)

	time := EstimatedTimeToBlock(hashrate, difficulty)
	expected := float64(difficulty) / hashrate

	if time != expected {
		t.Errorf("EstimatedTimeToBlock: got %f, want %f", time, expected)
	}

	// Test zero hashrate
	time = EstimatedTimeToBlock(0, difficulty)
	if time != 0 {
		t.Error("EstimatedTimeToBlock with zero hashrate should return 0")
	}
}

func TestCompactToTarget(t *testing.T) {
	// Test known compact values
	tests := []struct {
		compact  uint32
		hasValue bool
	}{
		{0x1d00ffff, true}, // Bitcoin genesis difficulty
		{0x00000000, false},
	}

	for _, tt := range tests {
		target := CompactToTarget(tt.compact)
		if tt.hasValue && target.Sign() <= 0 {
			t.Errorf("CompactToTarget(%x) should give positive target", tt.compact)
		}
	}
}

func BenchmarkDifficultyToTarget(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DifficultyToTarget(uint64(i + 1))
	}
}

func BenchmarkHashToDifficulty(b *testing.B) {
	hash := make([]byte, 32)
	hash[0] = 0x00
	hash[1] = 0x00
	hash[2] = 0x01

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashToDifficulty(hash)
	}
}
