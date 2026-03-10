package unittests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kushal-39/Stratum/internal/dht"
	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
	bencode "github.com/jackpal/bencode-go"
)

// Integration Test Case 1: Complete torrent workflow
func TestIntegration_TorrentParsing(t *testing.T) {
	// Create test torrent file
	testFile := createIntegrationTestTorrent(t, "integration-test", 16384, false)
	defer os.Remove(testFile)

	// Parse torrent
	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	// Verify core fields
	if tor.Announce == "" {
		t.Error("Announce URL missing")
	}
	if len(tor.InfoHash) != 20 {
		t.Error("InfoHash incorrect")
	}
	if tor.Info.PieceLength <= 0 {
		t.Error("Invalid piece length")
	}
}

// Integration Test Case 2: Storage and state management
func TestIntegration_StorageState(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "integration-storage")
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".state")

	writer, err := storage.New(tmpFile, []storage.FileInfo{{Path: nil, Length: 32768}})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// Simulate downloading pieces
	numPieces := 10
	completed := make([]bool, numPieces)

	// Mark some pieces as completed
	completed[0] = true
	completed[3] = true
	completed[7] = true

	// Save state
	if err := writer.SaveState(completed); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Load state and verify
	loaded, err := writer.LoadState(numPieces)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	for i := 0; i < numPieces; i++ {
		if loaded[i] != completed[i] {
			t.Errorf("Piece %d state mismatch", i)
		}
	}
}

// Integration Test Case 3: DHT node validation pipeline
func TestIntegration_DHTNodeValidation(t *testing.T) {
	nodes := []dht.Node{
		{ID: "0123456789abcdef0123456789abcdef01234567", IP: "192.168.1.1", Port: 6881},
		{ID: "fedcba9876543210fedcba9876543210fedcba98", IP: "192.168.1.2", Port: 6881},
		{ID: "1111111111111111111111111111111111111111", IP: "10.0.0.1", Port: 6882},
	}

	validCount := 0
	for _, node := range nodes {
		if err := node.Validate(); err == nil {
			validCount++
		}
	}

	if validCount != len(nodes) {
		t.Errorf("Expected all %d nodes valid, got %d", len(nodes), validCount)
	}
}

// Integration Test Case 4: Bitfield and piece selection
func TestIntegration_PieceSelection(t *testing.T) {
	totalPieces := uint32(100)
	bitfield := make([]byte, (totalPieces+7)/8)

	// Set every 3rd piece as available
	for i := uint32(0); i < totalPieces; i += 3 {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		bitfield[byteIdx] |= (1 << bitIdx)
	}

	// Pick pieces
	selectedPieces := make(map[uint32]bool)
	for len(selectedPieces) < 5 {
		piece, found := peer.PickPiece(bitfield, totalPieces)
		if !found {
			break
		}
		selectedPieces[piece] = true

		// Mark as picked by clearing bit
		byteIdx := piece / 8
		bitIdx := 7 - (piece % 8)
		bitfield[byteIdx] &^= (1 << bitIdx)
	}

	if len(selectedPieces) < 5 {
		t.Errorf("Expected to select 5 pieces, got %d", len(selectedPieces))
	}
}

// Integration Test Case 5: Full download lifecycle
func TestIntegration_DownloadLifecycle(t *testing.T) {
	// 1. Parse torrent
	testFile := createIntegrationTestTorrent(t, "lifecycle", 16384, false)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Setup storage
	storageFile := filepath.Join(t.TempDir(), "lifecycle-storage")
	defer os.Remove(storageFile)
	defer os.Remove(storageFile + ".state")

	writer, err := storage.New(storageFile, []storage.FileInfo{{Path: nil, Length: tor.Info.Length}})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// 3. Calculate pieces
	numPieces := (tor.Info.Length + int64(tor.Info.PieceLength) - 1) / int64(tor.Info.PieceLength)
	completed := make([]bool, numPieces)

	// 4. Simulate piece selection
	bitfield := make([]byte, (numPieces+7)/8)
	for i := range bitfield {
		bitfield[i] = 0xFF // All pieces available
	}

	// 5. "Download" pieces
	for i := int64(0); i < numPieces; i++ {
		piece, found := peer.PickPiece(bitfield, uint32(numPieces))
		if found {
			completed[piece] = true
			byteIdx := piece / 8
			bitIdx := 7 - (piece % 8)
			bitfield[byteIdx] &^= (1 << bitIdx)
		}
	}

	// 6. Save final state
	if err := writer.SaveState(completed); err != nil {
		t.Fatal(err)
	}

	// 7. Verify all pieces completed
	allCompleted := true
	for _, done := range completed {
		if !done {
			allCompleted = false
			break
		}
	}

	if !allCompleted {
		t.Error("Not all pieces were marked completed")
	}
}

// Helper function to create integration test torrents
func createIntegrationTestTorrent(t *testing.T, name string, size int64, multiFile bool) string {
	tmpFile, err := os.CreateTemp("", "test-*.torrent")
	if err != nil {
		t.Fatal(err)
	}
	defer tmpFile.Close()

	type rawInfo struct {
		Name        string `bencode:"name"`
		PieceLength int    `bencode:"piece length"`
		Pieces      string `bencode:"pieces"`
		Length      int64  `bencode:"length,omitempty"`
		Files       []struct {
			Length int64    `bencode:"length"`
			Path   []string `bencode:"path"`
		} `bencode:"files,omitempty"`
	}

	type rawTorrent struct {
		Announce string  `bencode:"announce"`
		Info     rawInfo `bencode:"info"`
	}

	info := rawInfo{
		Name:        name,
		PieceLength: 16384,
		Pieces:      string(make([]byte, 20)),
	}

	if multiFile {
		info.Files = []struct {
			Length int64    `bencode:"length"`
			Path   []string `bencode:"path"`
		}{
			{Length: 1024, Path: []string{"file1.txt"}},
			{Length: 2048, Path: []string{"dir", "file2.txt"}},
		}
	} else {
		info.Length = size
	}

	data := rawTorrent{
		Announce: "http://tracker.example.com:6969/announce",
		Info:     info,
	}

	if err := bencode.Marshal(tmpFile, data); err != nil {
		t.Fatal(err)
	}

	return tmpFile.Name()
}
