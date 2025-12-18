package util

import (
	"bytes"
	"testing"
)

func TestHexToBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected []byte
		hasError bool
	}{
		{"0x1234", []byte{0x12, 0x34}, false},
		{"1234", []byte{0x12, 0x34}, false},
		{"0xabcd", []byte{0xab, 0xcd}, false},
		{"ABCD", []byte{0xab, 0xcd}, false},
		{"", []byte{}, false},
		{"0x", []byte{}, false},
		{"xyz", nil, true},
		{"0x123", nil, true}, // Odd length
	}

	for _, tt := range tests {
		result, err := HexToBytes(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("HexToBytes(%q) should return error", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("HexToBytes(%q) returned error: %v", tt.input, err)
			}
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("HexToBytes(%q) = %x, want %x", tt.input, result, tt.expected)
			}
		}
	}
}

func TestBytesToHex(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte{0x12, 0x34}, "0x1234"},
		{[]byte{0xab, 0xcd}, "0xabcd"},
		{[]byte{}, "0x"},
		{[]byte{0x00}, "0x00"},
	}

	for _, tt := range tests {
		result := BytesToHex(tt.input)
		if result != tt.expected {
			t.Errorf("BytesToHex(%x) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestBytesToHexNoPre(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte{0x12, 0x34}, "1234"},
		{[]byte{0xab, 0xcd}, "abcd"},
		{[]byte{}, ""},
	}

	for _, tt := range tests {
		result := BytesToHexNoPre(tt.input)
		if result != tt.expected {
			t.Errorf("BytesToHexNoPre(%x) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestReverseBytes(t *testing.T) {
	input := []byte{1, 2, 3, 4, 5}
	expected := []byte{5, 4, 3, 2, 1}

	ReverseBytes(input)
	if !bytes.Equal(input, expected) {
		t.Errorf("ReverseBytes: got %v, want %v", input, expected)
	}

	// Test empty slice
	empty := []byte{}
	ReverseBytes(empty)
	if len(empty) != 0 {
		t.Error("ReverseBytes should handle empty slice")
	}
}

func TestReverseBytesCopy(t *testing.T) {
	input := []byte{1, 2, 3, 4, 5}
	original := make([]byte, len(input))
	copy(original, input)
	expected := []byte{5, 4, 3, 2, 1}

	result := ReverseBytesCopy(input)

	if !bytes.Equal(result, expected) {
		t.Errorf("ReverseBytesCopy: got %v, want %v", result, expected)
	}

	// Original should be unchanged
	if !bytes.Equal(input, original) {
		t.Error("ReverseBytesCopy should not modify original")
	}
}

func TestPadBytes(t *testing.T) {
	tests := []struct {
		input    []byte
		length   int
		expected []byte
	}{
		{[]byte{0x01, 0x02}, 4, []byte{0x00, 0x00, 0x01, 0x02}},
		{[]byte{0x01, 0x02}, 2, []byte{0x01, 0x02}},
		{[]byte{0x01, 0x02}, 1, []byte{0x01, 0x02}}, // No truncation
	}

	for _, tt := range tests {
		result := PadBytes(tt.input, tt.length)
		if !bytes.Equal(result, tt.expected) {
			t.Errorf("PadBytes(%x, %d) = %x, want %x", tt.input, tt.length, result, tt.expected)
		}
	}
}

func TestIsValidHex(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"0x1234", true},
		{"1234", true},
		{"abcdef", true},
		{"ABCDEF", true},
		{"0xABCDEF", true},
		{"xyz", false},
		{"0x123g", false},
		{"", true}, // Empty is valid
	}

	for _, tt := range tests {
		result := IsValidHex(tt.input)
		if result != tt.expected {
			t.Errorf("IsValidHex(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestValidateNonce(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"0x1234567890abcdef", true},
		{"1234567890abcdef", true},
		{"0x123456789ABCDEF0", true},
		{"0x1234", false},           // Too short
		{"0x1234567890abcdef12", false}, // Too long
		{"0x123456789abcdxyz", false},   // Invalid chars
	}

	for _, tt := range tests {
		result := ValidateNonce(tt.input)
		if result != tt.expected {
			t.Errorf("ValidateNonce(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestValidateHash(t *testing.T) {
	validHash := "0x" + "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	invalidHash := "0x1234"

	if !ValidateHash(validHash) {
		t.Error("ValidateHash should accept valid 64-char hash")
	}

	if ValidateHash(invalidHash) {
		t.Error("ValidateHash should reject short hash")
	}
}

func TestValidateAddress(t *testing.T) {
	// Generate a valid 62-char address using only valid bech32 characters
	// Valid bech32 chars: 023456789acdefghjklmnpqrstuvwxyz
	validChars := "023456789acdefghjklmnpqrstuvwxyz"
	validAddr := "tos1"
	for i := 0; i < 58; i++ {
		validAddr += string(validChars[i%len(validChars)])
	}

	tests := []struct {
		input    string
		expected bool
	}{
		{validAddr, true},
		// Invalid addresses
		{"tos0abcdefghijk", false}, // Wrong prefix
		{"btc1abcdefghijk", false}, // Wrong prefix
		{"tos1abc", false},         // Too short
		{"tos1" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa11", false}, // Contains invalid '1' and 'a' after tos1
	}

	for _, tt := range tests {
		result := ValidateAddress(tt.input)
		if result != tt.expected {
			t.Errorf("ValidateAddress(%q) = %v, want %v (len=%d)", tt.input, result, tt.expected, len(tt.input))
		}
	}
}

func BenchmarkHexToBytes(b *testing.B) {
	input := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	for i := 0; i < b.N; i++ {
		HexToBytes(input)
	}
}

func BenchmarkBytesToHex(b *testing.B) {
	input := make([]byte, 32)
	for i := range input {
		input[i] = byte(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BytesToHex(input)
	}
}
