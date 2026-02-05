package torrent

import (
	"bytes"
	"crypto/sha1"
	"fmt"

	bencode "github.com/jackpal/bencode-go"
)

// ReconstructTorrentFromMetadata creates a Torrent structure from magnet link and metadata
func ReconstructTorrentFromMetadata(magnet *Mag_link, metadataBytes []byte) (*Torrent, error) {
	// Parse the metadata (which is the bencoded info dictionary)
	var info Info
	if err := bencode.Unmarshal(bytes.NewReader(metadataBytes), &info); err != nil {
		return nil, fmt.Errorf("failed to decode metadata: %w", err)
	}

	// Verify the InfoHash matches
	hash := sha1.Sum(metadataBytes)
	if !bytesEqual(hash[:], magnet.Info_hash) {
		return nil, fmt.Errorf("metadata InfoHash mismatch: expected %x, got %x", magnet.Info_hash, hash[:])
	}

	// Create torrent structure
	torrent := &Torrent{
		Info:     info,
		InfoHash: hash,
	}

	// Set announce URLs from magnet
	if len(magnet.Trackers) > 0 {
		torrent.Announce = magnet.Trackers[0]

		// Convert to announce list format
		torrent.AnnounceList = make([][]string, len(magnet.Trackers))
		for i, tracker := range magnet.Trackers {
			torrent.AnnounceList[i] = []string{tracker}
		}
	}

	return torrent, nil
}

// Helper function to compare byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
