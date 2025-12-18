// Package toshash provides TOS Hash V3 verification for mining shares.
package toshash

import (
	"encoding/binary"
	"fmt"

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

	// NonceOffset is the offset of the nonce in the block header
	// TOS header structure: work_hash(32) + timestamp(8) + nonce(8) + extra_nonce(32) + miner(32)
	// Nonce is at bytes 40-47
	NonceOffset = 40
)

// Strides for stage 3 (matches canonical Rust v3)
var strides = [4]int{1, 64, 256, 1024}

// Hash computes TOS Hash V3 for the given input
// Input: 112 bytes (MinerWork format)
// Output: 32-byte hash
// This implementation matches the canonical Rust tos-hash v3 algorithm
func Hash(input []byte) []byte {
	if len(input) != InputSize {
		return nil
	}

	// Stage 1: Initialize scratchpad from Blake3(input)
	scratchpad := stage1Init(input)

	// Stage 2: Sequential memory mixing (4 passes)
	stage2Mix(scratchpad)

	// Stage 3: Strided memory mixing (8 rounds)
	stage3Strided(scratchpad)

	// Stage 4: XOR-fold and final Blake3
	return stage4Finalize(scratchpad)
}

// stage1Init creates the 64KB scratchpad from the input
// Matches canonical Rust v3 stage_1_init
func stage1Init(input []byte) []uint64 {
	scratchpad := make([]uint64, MemorySize)

	// Hash input to get 256-bit seed
	hasher := blake3.New()
	hasher.Write(input)
	hash := hasher.Sum(nil)

	// Convert to u64 seed values (little-endian)
	var state [4]uint64
	for i := 0; i < 4; i++ {
		state[i] = binary.LittleEndian.Uint64(hash[i*8 : (i+1)*8])
	}

	// Fill scratchpad sequentially using mix function
	for i := 0; i < MemorySize; i++ {
		idx := i % 4
		state[idx] = mix(state[idx], state[(idx+1)%4], i)
		scratchpad[i] = state[idx]
	}

	return scratchpad
}

// stage2Mix performs forward and backward passes over the scratchpad
// Matches canonical Rust v3 stage_2_mix
func stage2Mix(scratchpad []uint64) {
	for pass := 0; pass < MemoryPasses; pass++ {
		if pass%2 == 0 {
			// Forward pass
			carry := scratchpad[MemorySize-1]
			for i := 0; i < MemorySize; i++ {
				var prev uint64
				if i > 0 {
					prev = scratchpad[i-1]
				} else {
					prev = scratchpad[MemorySize-1]
				}
				scratchpad[i] = mix(scratchpad[i], prev^carry, pass)
				carry = scratchpad[i]
			}
		} else {
			// Backward pass
			carry := scratchpad[0]
			for i := MemorySize - 1; i >= 0; i-- {
				var next uint64
				if i < MemorySize-1 {
					next = scratchpad[i+1]
				} else {
					next = scratchpad[0]
				}
				scratchpad[i] = mix(scratchpad[i], next^carry, pass)
				carry = scratchpad[i]
			}
		}
	}
}

// stage3Strided performs strided access mixing
// Matches canonical Rust v3 stage_3_strided
func stage3Strided(scratchpad []uint64) {
	for round := 0; round < MixingRounds; round++ {
		stride := strides[round%len(strides)]

		for i := 0; i < MemorySize; i++ {
			j := (i + stride) % MemorySize
			k := (i + stride*2) % MemorySize

			// Three-way mixing without branches
			a := scratchpad[i]
			b := scratchpad[j]
			c := scratchpad[k]

			scratchpad[i] = mix(a, b^c, round)
		}
	}
}

// mix is the core mixing operation
// Matches canonical Rust v3 mix function
func mix(a, b uint64, round int) uint64 {
	// Rotation amount varies by round to add diffusion
	rot := uint((round * 7) % 64)

	// Simple arithmetic mixing - no branches
	x := a + b                     // wrapping add
	y := a ^ rotateLeft(b, rot)    // XOR with rotated b
	z := x * MixConstant           // multiply by golden ratio constant

	return z ^ rotateRight(y, rot/2)
}

// rotateLeft performs a 64-bit left rotation
func rotateLeft(x uint64, k uint) uint64 {
	k &= 63
	return (x << k) | (x >> (64 - k))
}

// rotateRight performs a 64-bit right rotation
func rotateRight(x uint64, k uint) uint64 {
	k &= 63
	return (x >> k) | (x << (64 - k))
}

// stage4Finalize compresses the scratchpad into the final hash
// Matches canonical Rust v3 stage_4_finalize
func stage4Finalize(scratchpad []uint64) []byte {
	// XOR-fold scratchpad into 256 bits (4 uint64)
	var folded [4]uint64
	for i := 0; i < MemorySize; i++ {
		folded[i%4] ^= scratchpad[i]
	}

	// Convert to bytes (little-endian)
	var bytes [32]byte
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint64(bytes[i*8:(i+1)*8], folded[i])
	}

	// Final Blake3 hash for security
	hasher := blake3.New()
	hasher.Write(bytes[:])
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
func BuildHeader(workHash []byte, timestamp, nonce uint64) []byte {
	header := make([]byte, InputSize)

	// TOS header layout:
	// [0:32]   Work hash (Blake3 of block data)
	// [32:40]  Timestamp (8 bytes, big-endian)
	// [40:48]  Nonce (8 bytes, big-endian)
	// [48:80]  Extra nonce (32 bytes)
	// [80:112] Miner address (32 bytes)

	copy(header[0:32], workHash)
	binary.BigEndian.PutUint64(header[32:40], timestamp)
	binary.BigEndian.PutUint64(header[NonceOffset:NonceOffset+8], nonce)

	return header
}

// ValidateShare validates a mining share
func ValidateShare(header []byte, nonce uint64, shareDifficulty, networkDifficulty uint64) (bool, bool) {
	// Replace nonce in header at NonceOffset (bytes 40-47, big-endian)
	workHeader := make([]byte, len(header))
	copy(workHeader, header)
	binary.BigEndian.PutUint64(workHeader[NonceOffset:NonceOffset+8], nonce)

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

// BlockHeader represents a parsed TOS block header
type BlockHeader struct {
	Version    uint8
	Height     uint64
	Timestamp  uint64
	Nonce      uint64
	ExtraNonce [32]byte
	Tips       [][]byte // Each tip is 32 bytes
	TxsHashes  [][]byte // Each tx hash is 32 bytes
	Miner      [32]byte
}

// ParseBlockHeader parses a serialized BlockHeader from the daemon
// BlockHeader format:
//   - version: 1 byte
//   - height: 8 bytes (big-endian)
//   - timestamp: 8 bytes (big-endian)
//   - nonce: 8 bytes (big-endian)
//   - extra_nonce: 32 bytes
//   - tips_count: 1 byte
//   - tips: tips_count × 32 bytes
//   - txs_count: 2 bytes (big-endian)
//   - txs_hashes: txs_count × 32 bytes
//   - miner: 32 bytes
func ParseBlockHeader(data []byte) (*BlockHeader, error) {
	// Minimum size: 1 + 8 + 8 + 8 + 32 + 1 + 2 + 32 = 92 bytes (no tips, no txs)
	if len(data) < 92 {
		return nil, fmt.Errorf("block header too short: %d bytes", len(data))
	}

	pos := 0
	header := &BlockHeader{}

	// version (1 byte)
	header.Version = data[pos]
	pos++

	// height (8 bytes, big-endian)
	header.Height = binary.BigEndian.Uint64(data[pos : pos+8])
	pos += 8

	// timestamp (8 bytes, big-endian)
	header.Timestamp = binary.BigEndian.Uint64(data[pos : pos+8])
	pos += 8

	// nonce (8 bytes, big-endian)
	header.Nonce = binary.BigEndian.Uint64(data[pos : pos+8])
	pos += 8

	// extra_nonce (32 bytes)
	copy(header.ExtraNonce[:], data[pos:pos+32])
	pos += 32

	// tips_count (1 byte)
	tipsCount := int(data[pos])
	pos++

	// tips (tips_count × 32 bytes)
	if pos+tipsCount*32 > len(data) {
		return nil, fmt.Errorf("block header truncated at tips: need %d bytes, have %d", pos+tipsCount*32, len(data))
	}
	header.Tips = make([][]byte, tipsCount)
	for i := 0; i < tipsCount; i++ {
		header.Tips[i] = make([]byte, 32)
		copy(header.Tips[i], data[pos:pos+32])
		pos += 32
	}

	// txs_count (2 bytes, big-endian)
	if pos+2 > len(data) {
		return nil, fmt.Errorf("block header truncated at txs_count")
	}
	txsCount := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2

	// txs_hashes (txs_count × 32 bytes)
	if pos+txsCount*32 > len(data) {
		return nil, fmt.Errorf("block header truncated at txs: need %d bytes, have %d", pos+txsCount*32, len(data))
	}
	header.TxsHashes = make([][]byte, txsCount)
	for i := 0; i < txsCount; i++ {
		header.TxsHashes[i] = make([]byte, 32)
		copy(header.TxsHashes[i], data[pos:pos+32])
		pos += 32
	}

	// miner (32 bytes)
	if pos+32 > len(data) {
		return nil, fmt.Errorf("block header truncated at miner")
	}
	copy(header.Miner[:], data[pos:pos+32])

	return header, nil
}

// ComputeTipsHash computes the Blake3 hash of all concatenated tips
func (h *BlockHeader) ComputeTipsHash() []byte {
	hasher := blake3.New()
	for _, tip := range h.Tips {
		hasher.Write(tip)
	}
	return hasher.Sum(nil)
}

// ComputeTxsHash computes the Blake3 hash of all concatenated tx hashes
func (h *BlockHeader) ComputeTxsHash() []byte {
	hasher := blake3.New()
	for _, tx := range h.TxsHashes {
		hasher.Write(tx)
	}
	return hasher.Sum(nil)
}

// ComputeWorkHash computes the work hash (immutable part of block)
// work_hash = Blake3(version(1) + height(8) + tips_hash(32) + txs_hash(32))
func (h *BlockHeader) ComputeWorkHash() []byte {
	// Build the work data: version + height + tips_hash + txs_hash
	workData := make([]byte, 73) // 1 + 8 + 32 + 32 = 73 bytes

	workData[0] = h.Version
	binary.BigEndian.PutUint64(workData[1:9], h.Height)

	tipsHash := h.ComputeTipsHash()
	copy(workData[9:41], tipsHash)

	txsHash := h.ComputeTxsHash()
	copy(workData[41:73], txsHash)

	// Hash to get work_hash
	hasher := blake3.New()
	hasher.Write(workData)
	return hasher.Sum(nil)
}

// ToMinerWork converts a BlockHeader to MinerWork format (112 bytes)
// MinerWork format:
//   - work_hash: 32 bytes (Blake3 of version + height + tips_hash + txs_hash)
//   - timestamp: 8 bytes (big-endian)
//   - nonce: 8 bytes (big-endian)
//   - extra_nonce: 32 bytes
//   - miner: 32 bytes
func (h *BlockHeader) ToMinerWork() []byte {
	minerWork := make([]byte, InputSize) // 112 bytes

	// work_hash (32 bytes)
	workHash := h.ComputeWorkHash()
	copy(minerWork[0:32], workHash)

	// timestamp (8 bytes, big-endian)
	binary.BigEndian.PutUint64(minerWork[32:40], h.Timestamp)

	// nonce (8 bytes, big-endian)
	binary.BigEndian.PutUint64(minerWork[40:48], h.Nonce)

	// extra_nonce (32 bytes)
	copy(minerWork[48:80], h.ExtraNonce[:])

	// miner (32 bytes)
	copy(minerWork[80:112], h.Miner[:])

	return minerWork
}

// BlockHeaderToMinerWork converts raw BlockHeader bytes to MinerWork format
func BlockHeaderToMinerWork(blockHeader []byte) ([]byte, error) {
	header, err := ParseBlockHeader(blockHeader)
	if err != nil {
		return nil, err
	}
	return header.ToMinerWork(), nil
}
