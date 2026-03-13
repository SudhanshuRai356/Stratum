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
	Info         rawInfo    `bencode:"info"`
}

func ParseTorrent(path string) (*Torrent, error) {
	f, err := os.Open(path) //used to structurally decode the .torrent file
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rt rawTorrent
	if err := bencode.NewDecoder(f).Decode(&rt); err != nil {
		return nil, err
	}

	f2, err := os.Open(path) // second time opening to build raw info hash
	if err != nil {
		return nil, err
	}
	defer f2.Close()

	var raw map[string]interface{}
	if err := bencode.NewDecoder(f2).Decode(&raw); err != nil {
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
		Info: Info{
			Name:        rt.Info.Name,
			PieceLength: rt.Info.PieceLength,
			Pieces:      rt.Info.Pieces,
			Length:      totalLength,
			Files:       rt.Info.Files, 
		},
		InfoHash: infoHash,
	}

	return t, nil
}
