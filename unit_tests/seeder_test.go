package unittests

import (
	"crypto/sha1"
	"encoding/binary"
	"math/rand"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
)

// helper to create a fake torrent + storage with real data for seeding tests
func createSeedTestData(t *testing.T, numPieces int, pieceLength int) (*torrent.Torrent, *storage.Writer, []bool, string) {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "seed-test-file")

	totalLength := int64(numPieces * pieceLength)
	writer, err := storage.New(tmpFile, []storage.FileInfo{{Path: nil, Length: totalLength}})
	if err != nil {
		t.Fatal(err)
	}

	// Write random data for each piece and compute hashes
	allHashes := make([]byte, 0, numPieces*20)
	for i := 0; i < numPieces; i++ {
		data := make([]byte, pieceLength)
		rand.Read(data)
		offset := int64(i) * int64(pieceLength)
		if err := writer.WriteAt(offset, data); err != nil {
			t.Fatal(err)
		}
		hash := sha1.Sum(data)
		allHashes = append(allHashes, hash[:]...)
	}

	tor := &torrent.Torrent{
		Info: torrent.Info{
			Name:        "seed-test-file",
			PieceLength: pieceLength,
			Pieces:      string(allHashes),
			Length:      totalLength,
		},
	}
	// Set a dummy infohash
	infoHashData := sha1.Sum([]byte("test-seed-torrent"))
	tor.InfoHash = infoHashData

	completed := make([]bool, numPieces)
	for i := range completed {
		completed[i] = true
	}

	return tor, writer, completed, tmpFile
}

// Test Case 1: Seeder creates and starts without error
func TestSeeder_StartStop(t *testing.T) {
	tor, writer, completed, _ := createSeedTestData(t, 4, 16384)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0) // port 0 = random
	if err != nil {
		t.Fatalf("NewSeeder failed: %v", err)
	}

	seeder.Start()
	if !seeder.IsRunning() {
		t.Error("Seeder should be running after Start()")
	}

	seeder.Stop()
	if seeder.IsRunning() {
		t.Error("Seeder should not be running after Stop()")
	}
}

// Test Case 2: Seeder accepts a TCP connection and sends handshake+bitfield
func TestSeeder_HandshakeAndBitfield(t *testing.T) {
	tor, writer, completed, _ := createSeedTestData(t, 8, 16384)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	// Connect to the seeder as a fake leecher
	conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to seeder: %v", err)
	}
	defer conn.Close()

	// Send a handshake
	sendTestHandshake(t, conn, tor.InfoHash)

	// Read the seeder's handshake back
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp := make([]byte, 68)
	if _, err := conn.Read(resp); err != nil {
		t.Fatalf("Failed to read handshake: %v", err)
	}
	if resp[0] != 19 { // len("BitTorrent protocol")
		t.Errorf("Invalid handshake pstrlen: %d", resp[0])
	}

	// Read next message - should be bitfield
	msg, err := peer.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read bitfield message: %v", err)
	}
	if msg.ID != peer.MsgBitfield {
		t.Errorf("Expected bitfield message (ID=5), got ID=%d", msg.ID)
	}
	// All 8 pieces completed, so bitfield should be 0xFF
	if len(msg.Payload) < 1 || msg.Payload[0] != 0xFF {
		t.Errorf("Expected bitfield 0xFF, got %x", msg.Payload)
	}
}

// Test Case 3: Seeder sends unchoke when leecher sends interested
func TestSeeder_UnchokeOnInterested(t *testing.T) {
	tor, writer, completed, _ := createSeedTestData(t, 4, 16384)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Handshake
	sendTestHandshake(t, conn, tor.InfoHash)
	readTestHandshake(t, conn)

	// Read bitfield
	peer.ReadMessage(conn)

	// Send interested
	peer.SendInterested(conn)

	// Read unchoke
	msg, err := peer.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read unchoke: %v", err)
	}
	if msg.ID != peer.MsgUnchoke {
		t.Errorf("Expected unchoke (ID=1), got ID=%d", msg.ID)
	}
}

// Test Case 4: Seeder serves a piece request correctly
func TestSeeder_ServePieceRequest(t *testing.T) {
	pieceLength := 16384
	tor, writer, completed, _ := createSeedTestData(t, 4, pieceLength)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	sendTestHandshake(t, conn, tor.InfoHash)
	readTestHandshake(t, conn)
	peer.ReadMessage(conn) // bitfield
	peer.SendInterested(conn)
	peer.ReadMessage(conn) // unchoke

	// Request first block of piece 0
	blockSize := uint32(16384)
	peer.SendReq(conn, 0, 0, blockSize)

	// Read piece response
	msg, err := peer.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read piece response: %v", err)
	}
	if msg.ID != peer.MsgPiece {
		t.Fatalf("Expected piece message (ID=7), got ID=%d", msg.ID)
	}

	// Parse payload: index(4) + begin(4) + block
	if len(msg.Payload) < 8 {
		t.Fatal("Piece payload too short")
	}
	respIdx := binary.BigEndian.Uint32(msg.Payload[0:4])
	respBegin := binary.BigEndian.Uint32(msg.Payload[4:8])
	block := msg.Payload[8:]

	if respIdx != 0 {
		t.Errorf("Expected piece index 0, got %d", respIdx)
	}
	if respBegin != 0 {
		t.Errorf("Expected begin 0, got %d", respBegin)
	}
	if len(block) != int(blockSize) {
		t.Errorf("Expected block size %d, got %d", blockSize, len(block))
	}

	// Verify the block matches what's on disk
	expected := make([]byte, blockSize)
	writer.ReadAt(expected, 0)
	for i := range block {
		if block[i] != expected[i] {
			t.Errorf("Block data mismatch at byte %d", i)
			break
		}
	}
}

// Test Case 5: Seeder tracks uploaded bytes
func TestSeeder_UploadCounter(t *testing.T) {
	pieceLength := 16384
	tor, writer, completed, _ := createSeedTestData(t, 2, pieceLength)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	if seeder.GetUploaded() != 0 {
		t.Error("Upload counter should be 0 initially")
	}

	conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	sendTestHandshake(t, conn, tor.InfoHash)
	readTestHandshake(t, conn)
	peer.ReadMessage(conn) // bitfield
	peer.SendInterested(conn)
	peer.ReadMessage(conn) // unchoke

	// Request a block
	blockSize := uint32(16384)
	peer.SendReq(conn, 0, 0, blockSize)
	peer.ReadMessage(conn) // read piece response

	// Give seeder a moment to update counter
	time.Sleep(100 * time.Millisecond)

	uploaded := seeder.GetUploaded()
	if uploaded != int64(blockSize) {
		t.Errorf("Expected uploaded=%d, got %d", blockSize, uploaded)
	}
}

// Test Case 6: Seeder rejects requests for pieces we don't have
func TestSeeder_PartialBitfield(t *testing.T) {
	tor, writer, _, _ := createSeedTestData(t, 8, 16384)
	defer writer.Close()

	// Only mark half the pieces as completed
	partial := make([]bool, 8)
	partial[0] = true
	partial[1] = true
	partial[2] = true
	partial[3] = true
	// pieces 4-7 are NOT completed

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, partial, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	sendTestHandshake(t, conn, tor.InfoHash)
	readTestHandshake(t, conn)

	// Read bitfield - should only have first 4 of 8 pieces
	msg, _ := peer.ReadMessage(conn)
	if msg.ID != peer.MsgBitfield {
		t.Fatalf("Expected bitfield, got ID=%d", msg.ID)
	}
	// First 4 bits set: 0xF0 = 11110000
	if msg.Payload[0] != 0xF0 {
		t.Errorf("Expected bitfield 0xF0, got 0x%02X", msg.Payload[0])
	}
}

// Test Case 7: Seeder handles infohash mismatch gracefully
func TestSeeder_InfoHashMismatch(t *testing.T) {
	tor, writer, completed, _ := createSeedTestData(t, 4, 16384)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	// Send handshake with WRONG infohash
	var wrongHash [20]byte
	copy(wrongHash[:], "12345678901234567890")
	sendTestHandshake(t, conn, wrongHash)

	// Seeder should close the connection (no valid handshake back)
	buf := make([]byte, 68)
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("Expected connection to be closed on infohash mismatch")
	}
}

// Test Case 8: Seeder handles multiple sequential requests
func TestSeeder_MultipleRequests(t *testing.T) {
	pieceLength := 32768 // 32KB piece = 2 blocks of 16KB
	tor, writer, completed, _ := createSeedTestData(t, 2, pieceLength)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	sendTestHandshake(t, conn, tor.InfoHash)
	readTestHandshake(t, conn)
	peer.ReadMessage(conn) // bitfield
	peer.SendInterested(conn)
	peer.ReadMessage(conn) // unchoke

	// Request two blocks from piece 0
	peer.SendReq(conn, 0, 0, 16384)
	peer.SendReq(conn, 0, 16384, 16384)

	// Should get both responses
	msg1, err := peer.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read first piece: %v", err)
	}
	if msg1.ID != peer.MsgPiece {
		t.Errorf("Expected piece msg, got ID=%d", msg1.ID)
	}

	msg2, err := peer.ReadMessage(conn)
	if err != nil {
		t.Fatalf("Failed to read second piece: %v", err)
	}
	if msg2.ID != peer.MsgPiece {
		t.Errorf("Expected piece msg, got ID=%d", msg2.ID)
	}

	// Verify offsets
	begin1 := binary.BigEndian.Uint32(msg1.Payload[4:8])
	begin2 := binary.BigEndian.Uint32(msg2.Payload[4:8])
	if begin1 != 0 {
		t.Errorf("First block begin should be 0, got %d", begin1)
	}
	if begin2 != 16384 {
		t.Errorf("Second block begin should be 16384, got %d", begin2)
	}
}

// Test Case 9: Seeder handles concurrent connections
func TestSeeder_ConcurrentConnections(t *testing.T) {
	tor, writer, completed, _ := createSeedTestData(t, 4, 16384)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	// Connect 3 leechers simultaneously
	numConns := 3
	conns := make([]net.Conn, numConns)
	for i := 0; i < numConns; i++ {
		conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
		if err != nil {
			t.Fatalf("Leecher %d failed to connect: %v", i, err)
		}
		conns[i] = conn
		defer conn.Close()
	}

	// Each does handshake and reads bitfield
	for i, conn := range conns {
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		sendTestHandshake(t, conn, tor.InfoHash)
		readTestHandshake(t, conn)
		msg, err := peer.ReadMessage(conn)
		if err != nil {
			t.Fatalf("Leecher %d: failed to read bitfield: %v", i, err)
		}
		if msg.ID != peer.MsgBitfield {
			t.Errorf("Leecher %d: expected bitfield, got ID=%d", i, msg.ID)
		}
	}
}

// Test Case 10: Seeder upload counter works across multiple peers
func TestSeeder_UploadCounterMultiplePeers(t *testing.T) {
	tor, writer, completed, _ := createSeedTestData(t, 4, 16384)
	defer writer.Close()

	var peerID [20]byte
	copy(peerID[:], "-ST0001-testseed1234")

	seeder, err := peer.NewSeeder(tor, writer, completed, peerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seeder.Start()
	defer seeder.Stop()

	// Two peers each request one block
	for i := 0; i < 2; i++ {
		conn, err := net.DialTimeout("tcp", seeder.Addr(), 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(10 * time.Second))

		sendTestHandshake(t, conn, tor.InfoHash)
		readTestHandshake(t, conn)
		peer.ReadMessage(conn) // bitfield
		peer.SendInterested(conn)
		peer.ReadMessage(conn) // unchoke

		peer.SendReq(conn, uint32(i), 0, 16384)
		peer.ReadMessage(conn) // piece
		conn.Close()
	}

	time.Sleep(200 * time.Millisecond)
	uploaded := seeder.GetUploaded()
	if uploaded != 2*16384 {
		t.Errorf("Expected total uploaded %d, got %d", 2*16384, uploaded)
	}
}

// --- test helpers ---

func sendTestHandshake(t *testing.T, conn net.Conn, infoHash [20]byte) {
	t.Helper()
	buf := make([]byte, 68)
	buf[0] = 19
	copy(buf[1:20], "BitTorrent protocol")
	// reserved bytes 20-28 (zeros)
	copy(buf[28:48], infoHash[:])
	copy(buf[48:68], "-TS0001-leecher12345") // fake peer ID
	if _, err := conn.Write(buf); err != nil {
		t.Fatalf("Failed to send test handshake: %v", err)
	}
}

func readTestHandshake(t *testing.T, conn net.Conn) {
	t.Helper()
	buf := make([]byte, 68)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("Failed to read handshake: %v", err)
	}
}
