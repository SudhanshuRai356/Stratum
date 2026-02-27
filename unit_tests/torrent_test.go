package unittests

import (
	"os"
	"testing"

	bencode "github.com/jackpal/bencode-go"
	"github.com/Kushal-39/Stratum/internal/torrent"
)

// Test Case 1: Parse single-file torrent
func TestParseSingleFileTorrent(t *testing.T) {
	testFile := createTestTorrent(t, "single-file", 1024, false)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	if tor.Announce == "" {
		t.Error("Announce URL should not be empty")
	}
	if tor.Info.Name != "single-file" {
		t.Errorf("Expected name 'single-file', got '%s'", tor.Info.Name)
	}
	if tor.Info.Length != 1024 {
		t.Errorf("Expected length 1024, got %d", tor.Info.Length)
	}
}

// Test Case 2: Parse multi-file torrent
func TestParseMultiFileTorrent(t *testing.T) {
	testFile := createTestTorrent(t, "multi-file", 0, true)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatalf("Failed to parse multi-file torrent: %v", err)
	}

	expectedTotal := int64(1024 + 2048)
	if tor.Info.Length != expectedTotal {
		t.Errorf("Expected total length %d, got %d", expectedTotal, tor.Info.Length)
	}
}

// Test Case 3: Verify InfoHash calculation
func TestInfoHashGeneration(t *testing.T) {
	testFile := createTestTorrent(t, "test-hash", 512, false)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// InfoHash must be exactly 20 bytes (SHA-1)
	if len(tor.InfoHash) != 20 {
		t.Errorf("InfoHash length must be 20, got %d", len(tor.InfoHash))
	}

	// Verify not all zeros
	nonZero := false
	for _, b := range tor.InfoHash {
		if b != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Error("InfoHash should contain non-zero bytes")
	}
}

// Test Case 4: Handle non-existent file
func TestParseNonExistentFile(t *testing.T) {
	_, err := torrent.ParseTorrent("/nonexistent/path/file.torrent")
	if err == nil {
		t.Error("Expected error when parsing non-existent file")
	}
}

// Test Case 5: Handle corrupted torrent file
func TestParseCorruptedFile(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "corrupted-*.torrent")
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("invalid bencode data !@#$%")
	tmpFile.Close()

	_, err := torrent.ParseTorrent(tmpFile.Name())
	if err == nil {
		t.Error("Expected error when parsing corrupted file")
	}
}

// Test Case 6: Verify piece length validation
func TestPieceLengthValidation(t *testing.T) {
	testFile := createTestTorrent(t, "piece-test", 16384, false)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatal(err)
	}

	if tor.Info.PieceLength <= 0 {
		t.Error("Piece length must be positive")
	}
	if tor.Info.PieceLength%16384 != 0 {
		t.Error("Piece length should be power of 2 multiple")
	}
}

// Test Case 7: Parse announce list
func TestAnnounceListParsing(t *testing.T) {
	testFile := createTestTorrentWithAnnounceList(t)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatal(err)
	}

	if len(tor.AnnounceList) == 0 {
		t.Error("Announce list should not be empty")
	}
	if len(tor.AnnounceList) < 2 {
		t.Error("Expected multiple announce URLs in list")
	}
}

// Test Case 8: Verify pieces field structure
func TestPiecesStructure(t *testing.T) {
	testFile := createTestTorrent(t, "pieces-test", 1024, false)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// Pieces must be multiple of 20 bytes (SHA-1 hashes)
	if len(tor.Info.Pieces)%20 != 0 {
		t.Errorf("Pieces length should be multiple of 20, got %d", len(tor.Info.Pieces))
	}
}

// Test Case 9: Empty torrent name handling
func TestEmptyTorrentName(t *testing.T) {
	testFile := createTestTorrent(t, "", 1024, false)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// System should handle empty names gracefully
	if tor.Info.Name == "" {
		t.Log("Warning: Torrent name is empty")
	}
}

// Test Case 10: Large file torrent parsing
func TestLargeFileTorrent(t *testing.T) {
	largeSize := int64(10 * 1024 * 1024 * 1024) // 10 GB
	testFile := createTestTorrent(t, "large-file", largeSize, false)
	defer os.Remove(testFile)

	tor, err := torrent.ParseTorrent(testFile)
	if err != nil {
		t.Fatal(err)
	}

	if tor.Info.Length != largeSize {
		t.Errorf("Expected length %d, got %d", largeSize, tor.Info.Length)
	}
}

// Helper: Create test torrent file
func createTestTorrent(t *testing.T, name string, size int64, multiFile bool) string {
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

// Helper: Create torrent with announce list
func createTestTorrentWithAnnounceList(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "announce-*.torrent")
	if err != nil {
		t.Fatal(err)
	}
	defer tmpFile.Close()

	type rawInfo struct {
		Name        string `bencode:"name"`
		PieceLength int    `bencode:"piece length"`
		Pieces      string `bencode:"pieces"`
		Length      int64  `bencode:"length"`
	}

	type rawTorrent struct {
		Announce     string     `bencode:"announce"`
		AnnounceList [][]string `bencode:"announce-list"`
		Info         rawInfo    `bencode:"info"`
	}

	data := rawTorrent{
		Announce: "http://tracker1.example.com/announce",
		AnnounceList: [][]string{
			{"http://tracker1.example.com/announce"},
			{"http://tracker2.example.com/announce", "http://tracker3.example.com/announce"},
		},
		Info: rawInfo{
			Name:        "announce-test",
			PieceLength: 16384,
			Pieces:      string(make([]byte, 20)),
			Length:      2048,
		},
	}

	if err := bencode.Marshal(tmpFile, data); err != nil {
		t.Fatal(err)
	}

	return tmpFile.Name()
}
