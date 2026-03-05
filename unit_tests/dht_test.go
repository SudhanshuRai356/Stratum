package unittests

import (
	"encoding/hex"
	"testing"

	"github.com/Kushal-39/Stratum/internal/dht"
)

// Test Case 1: Valid node with correct ID format
func TestNodeValidation_ValidNode(t *testing.T) {
	node := dht.Node{
		ID:   "0123456789abcdef0123456789abcdef01234567", // 40 hex chars
		IP:   "192.168.1.1",
		Port: 6881,
	}

	err := node.Validate()
	if err != nil {
		t.Errorf("Valid node should pass validation: %v", err)
	}
}

// Test Case 2: Invalid node ID length (too short)
func TestNodeValidation_ShortID(t *testing.T) {
	node := dht.Node{
		ID:   "0123456789", // Too short
		IP:   "192.168.1.1",
		Port: 6881,
	}

	err := node.Validate()
	if err == nil {
		t.Error("Should reject node ID shorter than 40 characters")
	}
}

// Test Case 3: Invalid node ID length (too long)
func TestNodeValidation_LongID(t *testing.T) {
	node := dht.Node{
		ID:   "0123456789abcdef0123456789abcdef0123456789", // Too long
		IP:   "192.168.1.1",
		Port: 6881,
	}

	err := node.Validate()
	if err == nil {
		t.Error("Should reject node ID longer than 40 characters")
	}
}

// Test Case 4: Invalid node ID (non-hex characters)
func TestNodeValidation_NonHexID(t *testing.T) {
	node := dht.Node{
		ID:   "ghijklmnopqrstuvwxyz0123456789abcdef012", // Contains non-hex
		IP:   "192.168.1.1",
		Port: 6881,
	}

	err := node.Validate()
	if err == nil {
		t.Error("Should reject node ID with non-hexadecimal characters")
	}
}

// Test Case 5: Invalid IP address
func TestNodeValidation_InvalidIP(t *testing.T) {
	node := dht.Node{
		ID:   "0123456789abcdef0123456789abcdef01234567",
		IP:   "999.999.999.999", // Invalid IP
		Port: 6881,
	}

	err := node.Validate()
	if err == nil {
		t.Error("Should reject invalid IP address")
	}
}

// Test Case 6: Valid hostname instead of IP
func TestNodeValidation_ValidHostname(t *testing.T) {
	node := dht.Node{
		ID:   "0123456789abcdef0123456789abcdef01234567",
		IP:   "localhost",
		Port: 6881,
	}

	err := node.Validate()
	if err != nil {
		t.Errorf("Should accept valid hostname: %v", err)
	}
}

// Test Case 7: Invalid port (too low)
func TestNodeValidation_PortTooLow(t *testing.T) {
	node := dht.Node{
		ID:   "0123456789abcdef0123456789abcdef01234567",
		IP:   "192.168.1.1",
		Port: 0, // Invalid port
	}

	err := node.Validate()
	if err == nil {
		t.Error("Should reject port 0")
	}
}

// Test Case 8: Invalid port (too high)
func TestNodeValidation_PortTooHigh(t *testing.T) {
	node := dht.Node{
		ID:   "0123456789abcdef0123456789abcdef01234567",
		IP:   "192.168.1.1",
		Port: 65536, // Port out of range
	}

	err := node.Validate()
	if err == nil {
		t.Error("Should reject port greater than 65535")
	}
}

// Test Case 9: Valid IPv6 address
func TestNodeValidation_IPv6(t *testing.T) {
	node := dht.Node{
		ID:   "0123456789abcdef0123456789abcdef01234567",
		IP:   "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		Port: 6881,
	}

	err := node.Validate()
	if err != nil {
		t.Errorf("Should accept valid IPv6 address: %v", err)
	}
}

// Test Case 10: Empty node ID
func TestNodeValidation_EmptyID(t *testing.T) {
	node := dht.Node{
		ID:   "",
		IP:   "192.168.1.1",
		Port: 6881,
	}

	err := node.Validate()
	if err == nil {
		t.Error("Should reject empty node ID")
	}
}

// Benchmark node validation
func BenchmarkNodeValidation(b *testing.B) {
	node := dht.Node{
		ID:   "0123456789abcdef0123456789abcdef01234567",
		IP:   "192.168.1.1",
		Port: 6881,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.Validate()
	}
}

// Benchmark hex decoding in validation
func BenchmarkHexDecoding(b *testing.B) {
	hexStr := "0123456789abcdef0123456789abcdef01234567"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hex.DecodeString(hexStr)
	}
}
