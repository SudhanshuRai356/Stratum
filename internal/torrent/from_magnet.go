// Parsing of magnet link to match the fields of Torrent struct.
// Part implementation of BEP 9

package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/zeebo/bencode"
)

type Mag_link struct {
	Trackers  []string
	Info_hash []byte
	Name      string
}

func Parse_Mag_URL(urli string) (*Mag_link, error) {
	trimmed := strings.TrimSpace(urli)
	if !strings.HasPrefix(strings.ToLower(trimmed), "magnet:") {
		return nil, errors.New("Invalid magnet URL")
	}
	parsed_url, err := url.Parse(trimmed)
	if err != nil {
		return nil, errors.New("Failed to parse magnet URL")
	}
	mag := &Mag_link{}
	params := parsed_url.Query()
	for keys, vals := range params {
		switch keys {
		case "xt":
			for _, val := range vals {
				if !strings.HasPrefix(strings.ToLower(val), "urn:btih:") {
					continue
				}
				hash_str := val[9:]
				if len(hash_str) == 40 {
					info_hash, err := hex.DecodeString(hash_str)
					if err == nil {
						mag.Info_hash = info_hash
					}
				} else if len(hash_str) > 0 {
					hash_str = strings.ToUpper(hash_str) //some weird torrent base32 padding rule man don't question it
					info_hash, err := base32.StdEncoding.DecodeString(hash_str)
					if err == nil {
						mag.Info_hash = info_hash
					}
				}
			}
		case "dn":
			if len(vals) > 0 {
				mag.Name = vals[0]
			}
		case "tr":
			mag.Trackers = append(mag.Trackers, vals...)
		}
	}
	if len(mag.Info_hash) == 0 {
		return nil, errors.New("magnet URL missing valid btih info hash")
	}
	return mag, nil
}

// ReconstructTorrentFromMetadata creates a Torrent structure from magnet link and metadata
func ReconstructTorrentFromMetadata(magnet *Mag_link, metadataBytes []byte) (*Torrent, error) {
	// Parse the metadata (which is the bencoded info dictionary)
	var info Info
	if err := bencode.NewDecoder(bytes.NewReader(metadataBytes)).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode metadata: %w", err)
	}

	// Verify the InfoHash matches
	hash := sha1.Sum(metadataBytes)
	if !bytes.Equal(hash[:], magnet.Info_hash) {
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
