package util

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// HexToBytes converts a hex string to bytes
func HexToBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	return hex.DecodeString(s)
}

// BytesToHex converts bytes to hex string with 0x prefix
func BytesToHex(b []byte) string {
	return "0x" + hex.EncodeToString(b)
}

// BytesToHexNoPre converts bytes to hex string without prefix
func BytesToHexNoPre(b []byte) string {
	return hex.EncodeToString(b)
}

// MustHexToBytes converts hex string to bytes, panics on error
func MustHexToBytes(s string) []byte {
	b, err := HexToBytes(s)
	if err != nil {
		panic(fmt.Sprintf("invalid hex string: %s", s))
	}
	return b
}

// ReverseBytes reverses a byte slice in place
func ReverseBytes(b []byte) []byte {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

// ReverseBytesCopy returns a reversed copy of a byte slice
func ReverseBytesCopy(b []byte) []byte {
	result := make([]byte, len(b))
	for i, j := 0, len(b)-1; j >= 0; i, j = i+1, j-1 {
		result[i] = b[j]
	}
	return result
}

// PadBytes pads bytes to specified length (left-pad with zeros)
func PadBytes(b []byte, length int) []byte {
	if len(b) >= length {
		return b
	}
	padded := make([]byte, length)
	copy(padded[length-len(b):], b)
	return padded
}

// IsValidHex checks if string is valid hexadecimal
func IsValidHex(s string) bool {
	s = strings.TrimPrefix(s, "0x")
	_, err := hex.DecodeString(s)
	return err == nil
}

// ValidateNonce validates nonce format (8 bytes / 16 hex chars)
func ValidateNonce(nonce string) bool {
	nonce = strings.TrimPrefix(nonce, "0x")
	if len(nonce) != 16 {
		return false
	}
	return IsValidHex(nonce)
}

// ValidateHash validates hash format (32 bytes / 64 hex chars)
func ValidateHash(hash string) bool {
	hash = strings.TrimPrefix(hash, "0x")
	if len(hash) != 64 {
		return false
	}
	return IsValidHex(hash)
}

// Int64ToHex converts int64 to hex string with 0x prefix
func Int64ToHex(n int64) string {
	return fmt.Sprintf("0x%x", n)
}

// Uint64ToHex converts uint64 to hex string with 0x prefix
func Uint64ToHex(n uint64) string {
	return fmt.Sprintf("0x%x", n)
}

// ValidateAddress validates TOS address format
func ValidateAddress(addr string) bool {
	// TOS addresses start with "tos1" and are 62 characters (bech32)
	if !strings.HasPrefix(addr, "tos1") {
		return false
	}
	if len(addr) != 62 {
		return false
	}
	// Basic bech32 character validation
	for _, c := range addr[4:] {
		if !strings.ContainsRune("023456789acdefghjklmnpqrstuvwxyz", c) {
			return false
		}
	}
	return true
}
