/*
// This file handles .torrent files. It does the decoding of raw torrent file into usable
// structs that can be used throughout this program. It provides APIs that consumer code can
// utilize to reduce redundancy. The support for magnet links, is in the other files.
//
// A standard .torrent file look like.
/* ```json
{
    "announce": "http://tracker.example.com:8080/announce",
    "announce-list": [
        ["http://tracker.example.com:8080/announce"],
        ["http://backup.example.com:6969/announce"]
    ],
    "creation date": 1700000000,
    "comment": "Created by MakeTorrent",
    "created by": "MyApp/1.0.0",
    "info": {
        "name": "my_videos",
        "piece length": 262144,
        "pieces": "abcdef0123456789...",  # 20 bytes per piece
        "files": [
            {"length": 12345678, "path": ["my_videos", "video_01.mp4"]},
            {"length": 87654321, "path": ["my_videos", "video_02.mp4"]},
            {"length": 1000000,  "path": ["my_videos", "subtitle.srt"]}
        ],
        "private": 1
    }
}
```
*/
// This json like structure is bencoded into human unreadable form, you can use strings and
// read. Info hash is calculated from the .torrent file and checked, hence its not in rawTorrent.

package torrent

import (
	"strings"
)

type Info struct {
	Name string `bencode:"name"`
	// This tag fixes the Magnet Link download!
	PieceLength int    `bencode:"piece length"`
	Pieces      string `bencode:"pieces"`
	Length      int64  `bencode:"length,omitempty"`
	Files       []File `bencode:"files,omitempty"`
}

type File struct {
	Length int64    `bencode:"length"`
	Path   []string `bencode:"path"`
}

type Torrent struct {
	Announce     string
	AnnounceList [][]string
	Comment      string
	CreatedBy    string
	CreationDate int64
	InfoHash     [20]byte
	Info         Info
}

// NumPieces returns how many pieces the torrent has.
func (t *Torrent) NumPieces() int {
	return len(t.Info.Pieces) / 20
}

// PieceLength returns the byte length of the piece at the given index.
// The last piece may be shorter than PieceLength.
func (t *Torrent) PieceLength(index int) int {
	if index == t.NumPieces()-1 {
		remaining := int(t.Info.Length) - index*t.Info.PieceLength
		if remaining > 0 {
			return remaining
		}
	}
	return t.Info.PieceLength
}

// PieceOffset returns the global byte offset where piece index starts.
func (t *Torrent) PieceOffset(index int) int64 {
	return int64(index) * int64(t.Info.PieceLength)
}

// PieceHash returns the expected SHA-1 hash for the piece at the given index.
func (t *Torrent) PieceHash(index int) []byte {
	return []byte(t.Info.Pieces[index*20 : (index+1)*20])
}

// IsMultiFile returns true if this torrent contains multiple files.
func (t *Torrent) IsMultiFile() bool {
	return len(t.Info.Files) > 0
}

// GetAllAnnounceURLs returns list of URLs, for
// which announce is possible.
func (t *Torrent) GetAllAnnounceURLs() []string {
	var urls []string

	if t.Announce != "" {
		urls = append(urls, t.Announce)
	}

	for _, tier := range t.AnnounceList {
		for _, u := range tier {
			if strings.HasPrefix(u, "http://") ||
				strings.HasPrefix(u, "https://") ||
				strings.HasPrefix(u, "udp://") {
				urls = append(urls, u)
			}
		}
	}

	return urls
}
