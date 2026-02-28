package unittests

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
)

// TestE2E_SeedAndDownload is a full end-to-end test:
// 1. Creates a fake torrent with known data
// 2. Starts a Seeder serving all pieces
// 3. A "leecher" connects, handshakes, and downloads every piece
// 4. Verifies every block matches the original data
// 5. Checks upload counter matches total bytes served
func TestE2E_SeedAndDownload(t *testing.T) {
	numPieces := 8
	pieceLength := 32768 // 32 KB per piece, 2 blocks each
	blockSize := uint32(16384)
	totalLen := int64(numPieces * pieceLength)

	// --- Setup: create torrent data on disk and build a seeder ---
	tmpFile := t.TempDir() + "/e2e-test-file"
	writer, err := storage.New(tmpFile, totalLen)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// Write random data per piece, compute SHA1 hashes
	originalData := make([]byte, totalLen) // keep a full copy for verification
	allHashes := make([]byte, 0, numPieces*20)
	for i := 0; i < numPieces; i++ {
		off := i * pieceLength
		rand.Read(originalData[off : off+pieceLength])
		if err := writer.WriteAt(int64(off), originalData[off:off+pieceLength]); err != nil {
			t.Fatal(err)
		}
		h := sha1.Sum(originalData[off : off+pieceLength])
		allHashes = append(allHashes, h[:]...)
	}

	infoHash := sha1.Sum([]byte("e2e-test-torrent"))
	tor := &torrent.Torrent{
		InfoHash: infoHash,
		Info: torrent.Info{
			Name:        "e2e-test-file",
			PieceLength: pieceLength,
			Pieces:      string(allHashes),
			Length:      totalLen,
		},
	}

	completed := make([]bool, numPieces)
	for i := range completed {
		completed[i] = true
	}

	var seedID [20]byte
	copy(seedID[:], "-ST0001-seedE2E12345")

	seeder, err := peer.NewSeeder(tor, writer, completed, seedID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	// --- Leecher: connect and download all pieces ---
	conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
	if err != nil {
		t.Fatalf("Leecher failed to connect: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Handshake
	sendTestHandshake(t, conn, infoHash)
	readTestHandshake(t, conn)

	// Read bitfield
	bfMsg, err := peer.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read bitfield: %v", err)
	}
	if bfMsg.ID != peer.MsgBitfield {
		t.Fatalf("Expected bitfield (5), got %d", bfMsg.ID)
	}
	// All 8 pieces → bitfield should be 0xFF
	if bfMsg.Payload[0] != 0xFF {
		t.Fatalf("Expected full bitfield 0xFF, got 0x%02X", bfMsg.Payload[0])
	}

	// Send interested
	peer.SendInterested(conn)

	// Expect unchoke
	unchoke, err := peer.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read unchoke: %v", err)
	}
	if unchoke.ID != peer.MsgUnchoke {
		t.Fatalf("Expected unchoke (1), got %d", unchoke.ID)
	}

	// Download every piece, block by block
	downloaded := make([]byte, totalLen)
	totalBlocks := 0

	for piece := 0; piece < numPieces; piece++ {
		blocksPerPiece := pieceLength / int(blockSize)
		for block := 0; block < blocksPerPiece; block++ {
			begin := uint32(block) * blockSize
			peer.SendReq(conn, uint32(piece), begin, blockSize)

			msg, err := peer.ReadMessage(conn)
			if err != nil {
				t.Fatalf("Failed to read piece %d block %d: %v", piece, block, err)
			}
			if msg.ID != peer.MsgPiece {
				t.Fatalf("Expected piece msg (7), got %d for piece %d block %d", msg.ID, piece, block)
			}

			// Parse response
			respIdx := binary.BigEndian.Uint32(msg.Payload[0:4])
			respBegin := binary.BigEndian.Uint32(msg.Payload[4:8])
			data := msg.Payload[8:]

			if respIdx != uint32(piece) {
				t.Fatalf("Piece index mismatch: expected %d, got %d", piece, respIdx)
			}
			if respBegin != begin {
				t.Fatalf("Begin mismatch: expected %d, got %d", begin, respBegin)
			}
			if len(data) != int(blockSize) {
				t.Fatalf("Block size mismatch: expected %d, got %d", blockSize, len(data))
			}

			// Copy into downloaded buffer
			fileOffset := int(respIdx)*pieceLength + int(respBegin)
			copy(downloaded[fileOffset:fileOffset+int(blockSize)], data)
			totalBlocks++
		}
	}

	// --- Verify: downloaded data matches original ---
	for i := 0; i < int(totalLen); i++ {
		if downloaded[i] != originalData[i] {
			t.Fatalf("Data mismatch at byte %d (piece %d, offset %d)", i, i/pieceLength, i%pieceLength)
		}
	}

	// Also verify SHA1 of each piece
	for i := 0; i < numPieces; i++ {
		off := i * pieceLength
		got := sha1.Sum(downloaded[off : off+pieceLength])
		var expected [20]byte
		copy(expected[:], allHashes[i*20:(i+1)*20])
		if got != expected {
			t.Fatalf("SHA1 mismatch for piece %d", i)
		}
	}

	// --- Verify upload counter ---
	conn.Close() // close so seeder finishes processing
	time.Sleep(200 * time.Millisecond)

	expectedUploaded := int64(totalBlocks) * int64(blockSize)
	actualUploaded := seeder.GetUploaded()
	if actualUploaded != expectedUploaded {
		t.Errorf("Upload counter: expected %d, got %d", expectedUploaded, actualUploaded)
	}

	fmt.Printf("E2E Test passed: downloaded %d pieces (%d blocks, %d bytes) from seeder\n",
		numPieces, totalBlocks, totalLen)
	fmt.Printf("Seeder uploaded: %d bytes\n", actualUploaded)
}
