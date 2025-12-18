package util

import (
	"encoding/binary"
	"math/big"
)

var (
	// MaxTarget is the maximum target value (difficulty 1)
	// For TOS: 2^256 / difficulty
	MaxTarget = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	// Diff1Target is the difficulty 1 target
	Diff1Target = new(big.Int).SetBytes([]byte{
		0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	})
)

// DifficultyToTarget converts difficulty to target
func DifficultyToTarget(difficulty uint64) *big.Int {
	if difficulty == 0 {
		return MaxTarget
	}
	target := new(big.Int).Div(Diff1Target, big.NewInt(int64(difficulty)))
	return target
}

// TargetToDifficulty converts target to difficulty
func TargetToDifficulty(target *big.Int) uint64 {
	if target.Sign() == 0 {
		return 0
	}
	difficulty := new(big.Int).Div(Diff1Target, target)
	return difficulty.Uint64()
}

// HashToDifficulty calculates difficulty from a hash
func HashToDifficulty(hash []byte) uint64 {
	if len(hash) != 32 {
		return 0
	}

	// Convert hash to big.Int (big-endian)
	hashInt := new(big.Int).SetBytes(hash)
	if hashInt.Sign() == 0 {
		return 0
	}

	// difficulty = diff1_target / hash
	difficulty := new(big.Int).Div(Diff1Target, hashInt)
	return difficulty.Uint64()
}

// HashMeetsTarget checks if hash meets the target difficulty
func HashMeetsTarget(hash []byte, target *big.Int) bool {
	if len(hash) != 32 {
		return false
	}
	hashInt := new(big.Int).SetBytes(hash)
	return hashInt.Cmp(target) <= 0
}

// HashMeetsDifficulty checks if hash meets the difficulty requirement
func HashMeetsDifficulty(hash []byte, difficulty uint64) bool {
	target := DifficultyToTarget(difficulty)
	return HashMeetsTarget(hash, target)
}

// CompactToTarget converts compact target representation to big.Int
func CompactToTarget(compact uint32) *big.Int {
	exponent := compact >> 24
	mantissa := compact & 0x007fffff

	var target *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		target = big.NewInt(int64(mantissa))
	} else {
		target = big.NewInt(int64(mantissa))
		target.Lsh(target, 8*(uint(exponent)-3))
	}

	if compact&0x00800000 != 0 {
		target.Neg(target)
	}

	return target
}

// TargetToCompact converts big.Int target to compact representation
func TargetToCompact(target *big.Int) uint32 {
	if target.Sign() == 0 {
		return 0
	}

	negative := target.Sign() < 0
	if negative {
		target = new(big.Int).Neg(target)
	}

	bytes := target.Bytes()
	size := uint32(len(bytes))

	var compact uint32
	if size <= 3 {
		compact = uint32(target.Uint64()) << (8 * (3 - size))
	} else {
		compact = uint32(new(big.Int).Rsh(target, 8*(uint(size)-3)).Uint64())
	}

	if compact&0x00800000 != 0 {
		compact >>= 8
		size++
	}

	compact |= size << 24
	if negative {
		compact |= 0x00800000
	}

	return compact
}

// NetworkHashrate estimates network hashrate from difficulty and block time
func NetworkHashrate(difficulty uint64, blockTimeSeconds float64) float64 {
	if blockTimeSeconds <= 0 {
		return 0
	}
	return float64(difficulty) / blockTimeSeconds
}

// EstimatedTimeToBlock estimates time to find a block given hashrate and difficulty
func EstimatedTimeToBlock(hashrate float64, difficulty uint64) float64 {
	if hashrate <= 0 {
		return 0
	}
	return float64(difficulty) / hashrate
}

// ShareDifficulty calculates the difficulty of a share from its hash
func ShareDifficulty(hash []byte) float64 {
	if len(hash) != 32 {
		return 0
	}

	// Read first 8 bytes as uint64 (big-endian)
	leading := binary.BigEndian.Uint64(hash[:8])
	if leading == 0 {
		return float64(^uint64(0)) // Max difficulty if leading zeros
	}

	// Approximate difficulty calculation
	return float64(Diff1Target.Uint64()) / float64(leading)
}
