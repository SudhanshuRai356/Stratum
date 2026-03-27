package torrent

import (
	"bytes"
	"crypto/sha1"
	"os"

	"github.com/zeebo/bencode"
)

type rawInfo struct {
	Name        string `bencode:"name"`
	PieceLength int    `bencode:"piece length"`
	Pieces      string `bencode:"pieces"`

	Length int64 `bencode:"length"`

	Files []File `bencode:"files"`
}

type rawTorrent struct {
	Announce     string     `bencode:"announce"`
	AnnounceList [][]string `bencode:"announce-list"`
	Comment      string     `bencode:"comment"`
	CreatedBy    string     `bencode:"created by"`
	CreationDate int64      `bencode:"creation date"`
	Info         rawInfo    `bencode:"info"`
}

// ParserTorrent parses raw .torrent files, checks info hash integrity 
// returns Torrent struct which we can use everywhere.
func ParseTorrent(path string) (*Torrent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rt rawTorrent
	if err := bencode.NewDecoder(bytes.NewReader(data)).Decode(&rt); err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := bencode.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return nil, err
	}

	infoMap := raw["info"]
	var buf bytes.Buffer
	if err := bencode.NewEncoder(&buf).Encode(infoMap); err != nil {
		return nil, err
	}
	infoHash := sha1.Sum(buf.Bytes())

	var totalLength int64

	// single-file torrent
	if rt.Info.Length > 0 {
		totalLength = rt.Info.Length
	}

	// multi-file torrent
	if len(rt.Info.Files) > 0 {
		for _, f := range rt.Info.Files {
			totalLength += f.Length
		}
	}

	t := &Torrent{
		Announce:     rt.Announce,
		AnnounceList: rt.AnnounceList,
		Comment:      rt.Comment,
		CreatedBy:    rt.CreatedBy,
		CreationDate: rt.CreationDate,
		InfoHash:     infoHash,
		Info: Info{
			Name:        rt.Info.Name,
			PieceLength: rt.Info.PieceLength,
			Pieces:      rt.Info.Pieces,
			Length:      totalLength,
			Files:       rt.Info.Files,
		},
	}

	return t, nil
}
