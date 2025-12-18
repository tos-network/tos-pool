// Package toshash provides TOS Hash V3 verification for mining shares.
package toshash

import (
	"encoding/binary"

	"github.com/zeebo/blake3"
)

const (
	// MemorySize is the scratchpad size in 64-bit words (64KB / 8 = 8192)
	MemorySize = 8192

	// MemorySizeBytes is the scratchpad size in bytes
	MemorySizeBytes = MemorySize * 8

	// MixingRounds is the number of strided mixing rounds
	MixingRounds = 8

	// MemoryPasses is the number of sequential memory passes
	MemoryPasses = 4

	// MixConstant is the mixing constant
	MixConstant = 0x517cc1b727220a95

	// InputSize is the block header size
	InputSize = 112

	// OutputSize is the hash output size
	OutputSize = 32
)

// Hash computes TOS Hash V3 for the given input
// Input: 112 bytes (104-byte header + 8-byte nonce)
// Output: 32-byte hash
func Hash(input []byte) []byte {
	if len(input) != InputSize {
		return nil
	}

	// Stage 1: Initialize scratchpad from Blake3(input)
	scratchpad := initializeScratchpad(input)

	// Stage 2: Sequential memory mixing (4 passes)
	sequentialMixing(scratchpad)

	// Stage 3: Strided memory mixing (8 rounds)
	stridedMixing(scratchpad)

	// Stage 4: XOR-fold and final Blake3
	return finalize(scratchpad, input)
}

// initializeScratchpad creates the 64KB scratchpad from the input
func initializeScratchpad(input []byte) []uint64 {
	scratchpad := make([]uint64, MemorySize)

	// Hash input to get initial state
	hasher := blake3.New()
	hasher.Write(input)
	seed := hasher.Sum(nil)

	// Expand seed to fill scratchpad using Blake3 in counter mode
	for i := 0; i < MemorySize; i += 4 {
		h := blake3.New()

		// Counter-based expansion
		var counter [8]byte
		binary.LittleEndian.PutUint64(counter[:], uint64(i/4))

		h.Write(seed)
		h.Write(counter[:])
		block := h.Sum(nil)

		// Each Blake3 output gives us 4 uint64 values
		for j := 0; j < 4 && i+j < MemorySize; j++ {
			scratchpad[i+j] = binary.LittleEndian.Uint64(block[j*8 : (j+1)*8])
		}
	}

	return scratchpad
}

// sequentialMixing performs forward and backward passes over the scratchpad
func sequentialMixing(scratchpad []uint64) {
	for pass := 0; pass < MemoryPasses; pass++ {
		// Forward pass
		for i := 1; i < MemorySize; i++ {
			scratchpad[i] ^= mixFunction(scratchpad[i-1], scratchpad[i])
		}

		// Backward pass
		for i := MemorySize - 2; i >= 0; i-- {
			scratchpad[i] ^= mixFunction(scratchpad[i+1], scratchpad[i])
		}
	}
}

// stridedMixing performs power-of-2 stride mixing
func stridedMixing(scratchpad []uint64) {
	for round := 0; round < MixingRounds; round++ {
		stride := 1 << round // 1, 2, 4, 8, 16, 32, 64, 128

		for i := 0; i < MemorySize; i++ {
			j := (i + stride) % MemorySize
			scratchpad[i] ^= mixFunction(scratchpad[j], scratchpad[i])
		}
	}
}

// mixFunction is the core mixing operation
func mixFunction(a, b uint64) uint64 {
	// Rotate and XOR
	rotated := rotateRight(a, 17) ^ b
	// Multiply by constant
	mixed := rotated * MixConstant
	// Additional rotation
	return rotateRight(mixed, 23)
}

// rotateRight performs a 64-bit right rotation
func rotateRight(x uint64, k uint) uint64 {
	return (x >> k) | (x << (64 - k))
}

// finalize compresses the scratchpad into the final hash
func finalize(scratchpad []uint64, input []byte) []byte {
	// XOR-fold scratchpad into 256 bits (4 uint64)
	var folded [4]uint64
	for i := 0; i < MemorySize; i++ {
		folded[i%4] ^= scratchpad[i]
	}

	// Final Blake3 hash
	hasher := blake3.New()

	// Include original input
	hasher.Write(input)

	// Include folded scratchpad
	for i := 0; i < 4; i++ {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], folded[i])
		hasher.Write(buf[:])
	}

	return hasher.Sum(nil)
}

// Verify checks if a hash meets the target difficulty
func Verify(input []byte, target []byte) bool {
	hash := Hash(input)
	if hash == nil {
		return false
	}

	// Compare hash with target (hash must be <= target)
	// Both are big-endian for comparison
	for i := 0; i < 32; i++ {
		if hash[i] < target[i] {
			return true
		}
		if hash[i] > target[i] {
			return false
		}
	}
	return true // Equal
}

// VerifyDifficulty checks if a hash meets the difficulty requirement
func VerifyDifficulty(input []byte, difficulty uint64) bool {
	hash := Hash(input)
	if hash == nil {
		return false
	}

	// Calculate actual difficulty from hash
	actualDiff := HashToDifficulty(hash)
	return actualDiff >= difficulty
}

// HashToDifficulty calculates difficulty from a hash
func HashToDifficulty(hash []byte) uint64 {
	if len(hash) < 8 {
		return 0
	}

	// Use first 8 bytes as big-endian uint64
	leading := binary.BigEndian.Uint64(hash[:8])
	if leading == 0 {
		return ^uint64(0) // Max difficulty
	}

	// Difficulty = 2^64 / leading_value (approximation)
	return ^uint64(0) / leading
}

// BuildHeader constructs a mining header from components
func BuildHeader(prevHash, merkleRoot []byte, timestamp, nonce uint64) []byte {
	header := make([]byte, InputSize)

	// Layout:
	// [0:32]   Previous block hash
	// [32:64]  Merkle root
	// [64:72]  Timestamp (8 bytes, little-endian)
	// [72:80]  Nonce (8 bytes, little-endian)
	// [80:112] Extra data / reserved

	copy(header[0:32], prevHash)
	copy(header[32:64], merkleRoot)
	binary.LittleEndian.PutUint64(header[64:72], timestamp)
	binary.LittleEndian.PutUint64(header[72:80], nonce)

	return header
}

// ValidateShare validates a mining share
func ValidateShare(header []byte, nonce uint64, shareDifficulty, networkDifficulty uint64) (bool, bool) {
	// Replace nonce in header
	workHeader := make([]byte, len(header))
	copy(workHeader, header)
	binary.LittleEndian.PutUint64(workHeader[72:80], nonce)

	// Compute hash
	hash := Hash(workHeader)
	if hash == nil {
		return false, false
	}

	// Check difficulty
	actualDiff := HashToDifficulty(hash)

	// Check share difficulty
	if actualDiff < shareDifficulty {
		return false, false
	}

	// Check if block found
	if actualDiff >= networkDifficulty {
		return true, true
	}

	return true, false
}
