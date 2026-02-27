package unittests

import (
	"testing"

	"github.com/Kushal-39/Stratum/internal/peer"
)

// Test Case 1: Pick piece from bitfield with available pieces
func TestPickPieceAvailable(t *testing.T) {
	// Bitfield with first 3 pieces available
	bitfield := []byte{0xE0} // 11100000 in binary
	totalPieces := uint32(8)

	pieceIndex, found := peer.PickPiece(bitfield, totalPieces)
	if !found {
		t.Error("Should find available piece")
	}
	if pieceIndex >= 3 {
		t.Errorf("Expected piece index 0-2, got %d", pieceIndex)
	}
}

// Test Case 2: Pick piece from empty bitfield
func TestPickPieceEmptyBitfield(t *testing.T) {
	bitfield := []byte{0x00}
	totalPieces := uint32(8)

	_, found := peer.PickPiece(bitfield, totalPieces)
	if found {
		t.Error("Should not find piece in empty bitfield")
	}
}

// Test Case 3: Pick piece from full bitfield
func TestPickPieceFullBitfield(t *testing.T) {
	bitfield := []byte{0xFF, 0xFF}
	totalPieces := uint32(16)

	pieceIndex, found := peer.PickPiece(bitfield, totalPieces)
	if !found {
		t.Error("Should find piece in full bitfield")
	}
	if pieceIndex >= totalPieces {
		t.Errorf("Piece index %d exceeds total pieces %d", pieceIndex, totalPieces)
	}
}

// Test Case 4: Pick piece with single bit set
func TestPickPieceSingleBit(t *testing.T) {
	bitfield := []byte{0x01} // Only last bit set
	totalPieces := uint32(8)

	pieceIndex, found := peer.PickPiece(bitfield, totalPieces)
	if !found {
		t.Error("Should find the single available piece")
	}
	if pieceIndex != 7 {
		t.Errorf("Expected piece index 7, got %d", pieceIndex)
	}
}

// Test Case 5: Pick piece bitfield shorter than expected
func TestPickPieceTruncatedBitfield(t *testing.T) {
	bitfield := []byte{0xFF, 0xFF}
	totalPieces := uint32(10) // Only 10 pieces, not 16

	pieceIndex, found := peer.PickPiece(bitfield, totalPieces)
	if !found {
		t.Error("Should find piece")
	}
	if pieceIndex >= totalPieces {
		t.Errorf("Piece index %d should be less than %d", pieceIndex, totalPieces)
	}
}

// Test Case 6: Pick piece from multi-byte bitfield
func TestPickPieceMultiByte(t *testing.T) {
	// First byte empty, second byte has pieces
	bitfield := []byte{0x00, 0x80} // Piece 8 available
	totalPieces := uint32(16)

	pieceIndex, found := peer.PickPiece(bitfield, totalPieces)
	if !found {
		t.Error("Should find piece in second byte")
	}
	if pieceIndex != 8 {
		t.Errorf("Expected piece index 8, got %d", pieceIndex)
	}
}

// Test Case 7: Pick piece with zero total pieces
func TestPickPieceZeroTotal(t *testing.T) {
	bitfield := []byte{0xFF}
	totalPieces := uint32(0)

	_, found := peer.PickPiece(bitfield, totalPieces)
	if found {
		t.Error("Should not find piece when total pieces is 0")
	}
}

// Test Case 8: Pick piece with nil bitfield
func TestPickPieceNilBitfield(t *testing.T) {
	var bitfield []byte
	totalPieces := uint32(8)

	_, found := peer.PickPiece(bitfield, totalPieces)
	if found {
		t.Error("Should not find piece with nil bitfield")
	}
}

// Test Case 9: Pick piece checks bit ordering
func TestPickPieceBitOrdering(t *testing.T) {
	// Test that bits are read in correct order (MSB first)
	bitfield := []byte{0x80} // 10000000 - first bit set
	totalPieces := uint32(8)

	pieceIndex, found := peer.PickPiece(bitfield, totalPieces)
	if !found {
		t.Error("Should find first piece")
	}
	if pieceIndex != 0 {
		t.Errorf("Expected piece index 0 (MSB first), got %d", pieceIndex)
	}
}

// Test Case 10: Pick piece from sparse bitfield
func TestPickPieceSparse(t *testing.T) {
	// Alternating bits: 10101010
	bitfield := []byte{0xAA}
	totalPieces := uint32(8)

	pieceIndex, found := peer.PickPiece(bitfield, totalPieces)
	if !found {
		t.Error("Should find available piece")
	}
	// Should find first available (piece 0)
	if pieceIndex%2 != 0 {
		t.Errorf("Expected even piece index, got %d", pieceIndex)
	}
}

// Benchmark picking piece from large bitfield
func BenchmarkPickPiece(b *testing.B) {
	totalPieces := uint32(10000)
	bitfield := make([]byte, (totalPieces+7)/8)
	
	// Set middle piece
	middlePiece := totalPieces / 2
	byteIdx := middlePiece / 8
	bitIdx := 7 - (middlePiece % 8)
	bitfield[byteIdx] = 1 << bitIdx

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.PickPiece(bitfield, totalPieces)
	}
}
