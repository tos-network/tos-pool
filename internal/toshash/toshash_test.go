package toshash

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestHash(t *testing.T) {
	// Test with valid input
	input := make([]byte, InputSize)
	for i := range input {
		input[i] = byte(i)
	}

	hash := Hash(input)
	if hash == nil {
		t.Fatal("Hash returned nil for valid input")
	}

	if len(hash) != OutputSize {
		t.Errorf("Hash output size: got %d, want %d", len(hash), OutputSize)
	}

	// Test determinism
	hash2 := Hash(input)
	if !bytes.Equal(hash, hash2) {
		t.Error("Hash is not deterministic")
	}
}

func TestHashInvalidInput(t *testing.T) {
	// Test with invalid input size
	shortInput := make([]byte, 10)
	hash := Hash(shortInput)
	if hash != nil {
		t.Error("Hash should return nil for invalid input size")
	}

	longInput := make([]byte, 200)
	hash = Hash(longInput)
	if hash != nil {
		t.Error("Hash should return nil for invalid input size")
	}
}

func TestHashToDifficulty(t *testing.T) {
	// Test with all zeros (max difficulty)
	zeroHash := make([]byte, 32)
	diff := HashToDifficulty(zeroHash)
	if diff != ^uint64(0) {
		t.Errorf("Zero hash should give max difficulty")
	}

	// Test with high value hash (low difficulty)
	highHash := make([]byte, 32)
	highHash[0] = 0xFF
	diff = HashToDifficulty(highHash)
	if diff == 0 {
		t.Error("High hash should give non-zero difficulty")
	}
}

func TestBuildHeader(t *testing.T) {
	prevHash := make([]byte, 32)
	merkleRoot := make([]byte, 32)
	timestamp := uint64(1702900000)
	nonce := uint64(12345678)

	header := BuildHeader(prevHash, merkleRoot, timestamp, nonce)

	if len(header) != InputSize {
		t.Errorf("Header size: got %d, want %d", len(header), InputSize)
	}

	// Verify timestamp is stored correctly
	storedTimestamp := binary.LittleEndian.Uint64(header[64:72])
	if storedTimestamp != timestamp {
		t.Errorf("Timestamp: got %d, want %d", storedTimestamp, timestamp)
	}

	// Verify nonce is stored correctly
	storedNonce := binary.LittleEndian.Uint64(header[72:80])
	if storedNonce != nonce {
		t.Errorf("Nonce: got %d, want %d", storedNonce, nonce)
	}
}

func TestValidateShare(t *testing.T) {
	// Create a test header
	header := make([]byte, InputSize)
	for i := range header {
		header[i] = byte(i)
	}

	// Test with very low share difficulty (should pass)
	valid, isBlock := ValidateShare(header, 0, 1, 1000000000000)
	if !valid {
		t.Error("Share should be valid with difficulty 1")
	}
	if isBlock {
		t.Error("Share should not be a block with high network difficulty")
	}
}

func TestRotateRight(t *testing.T) {
	tests := []struct {
		x      uint64
		k      uint
		expect uint64
	}{
		{0x8000000000000000, 1, 0x4000000000000000},
		{0x0000000000000001, 1, 0x8000000000000000},
		{0xFFFFFFFFFFFFFFFF, 64, 0xFFFFFFFFFFFFFFFF},
	}

	for _, tt := range tests {
		result := rotateRight(tt.x, tt.k)
		if result != tt.expect {
			t.Errorf("rotateRight(%x, %d) = %x, want %x", tt.x, tt.k, result, tt.expect)
		}
	}
}

func TestMixFunction(t *testing.T) {
	// Test that mix function produces different output for different inputs
	result1 := mixFunction(100, 200)
	result2 := mixFunction(200, 100)
	result3 := mixFunction(100, 200)

	if result1 == result2 {
		t.Error("Mix function should be order-dependent")
	}

	if result1 != result3 {
		t.Error("Mix function should be deterministic")
	}
}

func BenchmarkHash(b *testing.B) {
	input := make([]byte, InputSize)
	for i := range input {
		input[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Hash(input)
	}
}

func BenchmarkMixFunction(b *testing.B) {
	for i := 0; i < b.N; i++ {
		mixFunction(uint64(i), uint64(i+1))
	}
}
