package storage

import(
	"fmt"
	"os"
)

func(w *Writer) SaveState(completed []bool) error{
	statePath:=w.path+".state"
	numBytes:= (len(completed) + 7) / 8
	bitfield := make([]byte, numBytes)

	for i, done := range completed {
		if done {
			byteIdx := i / 8
			bitIdx := 7 - (i % 8)
			bitfield[byteIdx] |= (1 << bitIdx)
		}
	}

	return os.WriteFile(statePath, bitfield, 0644)
}

// LoadState reads the .state file and returns the completed map.
// Note: It does NOT verify hashes (main logic should do that).
func (w *Writer) LoadState(totalPieces int) ([]bool, error) {
	statePath := w.path + ".state"
	
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err // File doesn't exist (fresh download)
	}

	completed := make([]bool, totalPieces)
	
	// Expand bytes back to bools
	for i := 0; i < totalPieces; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)

		if byteIdx < len(data) && (data[byteIdx]&(1<<bitIdx)) != 0 {
			completed[i] = true
		}
	}
	
	fmt.Printf("State loaded from %s\n", statePath)
	return completed, nil
}
