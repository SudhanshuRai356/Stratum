package unittests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kushal-39/Stratum/internal/storage"
)

// Test Case 1: Save state with all pieces completed
func TestSaveState_AllCompleted(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test-download")
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".state")

	writer, err := storage.New(tmpFile, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	completed := []bool{true, true, true, true}
	err = writer.SaveState(completed)
	if err != nil {
		t.Errorf("Failed to save state: %v", err)
	}

	// Verify state file exists
	if _, err := os.Stat(tmpFile + ".state"); os.IsNotExist(err) {
		t.Error("State file was not created")
	}
}

// Test Case 2: Save state with no pieces completed
func TestSaveState_NoneCompleted(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test-empty")
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".state")

	writer, err := storage.New(	tmpFile, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	completed := []bool{false, false, false, false}
	err = writer.SaveState(completed)
	if err != nil {
		t.Errorf("Failed to save empty state: %v", err)
	}
}

// Test Case 3: Load state from existing file
func TestLoadState_Existing(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test-load")
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".state")

	writer, err := storage.New(tmpFile, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// Save state first
	original := []bool{true, false, true, true, false}
	writer.SaveState(original)

	// Load state back
	loaded, err := writer.LoadState(len(original))
	if err != nil {
		t.Errorf("Failed to load state: %v", err)
	}

	// Compare
	for i := range original {
		if loaded[i] != original[i] {
			t.Errorf("Piece %d mismatch: expected %v, got %v", i, original[i], loaded[i])
		}
	}
}

// Test Case 4: Load state from non-existent file
func TestLoadState_NonExistent(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test-nostate")
	defer os.Remove(tmpFile)

	writer, err := storage.New(tmpFile, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	_, err = writer.LoadState(10)
	if err == nil {
		t.Error("Expected error when loading non-existent state")
	}
}

// Test Case 5: Save and load large state
func TestSaveLoadState_Large(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test-large")
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".state")

	writer, err := storage.New(tmpFile, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// Create large state (10000 pieces)
	totalPieces := 10000
	completed := make([]bool, totalPieces)
	for i := 0; i < totalPieces; i++ {
		completed[i] = (i%2 == 0) // Every other piece
	}

	if err := writer.SaveState(completed); err != nil {
		t.Fatal(err)
	}

	loaded, err := writer.LoadState(totalPieces)
	if err != nil {
		t.Fatal(err)
	}

	// Verify all pieces
	for i := 0; i < totalPieces; i++ {
		if loaded[i] != completed[i] {
			t.Errorf("Piece %d mismatch", i)
			break
		}
	}
}

// Benchmark saving state
func BenchmarkSaveState(b *testing.B) {
	tmpFile := filepath.Join(b.TempDir(), "bench-save")
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".state")

	writer, _ := storage.New(tmpFile, 1000*512)
	defer writer.Close()

	completed := make([]bool, 1000)
	for i := range completed {
		completed[i] = (i%2 == 0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.SaveState(completed)
	}
}

// Benchmark loading state
func BenchmarkLoadState(b *testing.B) {
	tmpFile := filepath.Join(b.TempDir(), "bench-load")
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".state")

	writer, _ := storage.New(tmpFile, 1000*512)
	defer writer.Close()

	completed := make([]bool, 1000)
	for i := range completed {
		completed[i] = (i%2 == 0)
	}
	writer.SaveState(completed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.LoadState(1000)
	}
}
