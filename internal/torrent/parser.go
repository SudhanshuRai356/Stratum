package torrent

import (
	"bytes"
	"crypto/sha1"
	"os"

	bencode "github.com/jackpal/bencode-go"
)

type rawInfo struct {
	Name        string `bencode:"name"`
	PieceLength int    `bencode:"piece length"`
	Pieces      string `bencode:"pieces"`
	Length      int64  `bencode:"length"`
}

type rawTorrent struct {
	Announce string  `bencode:"announce"`
	Info     rawInfo `bencode:"info"`
}

func ParseTorrent(path string) (*Torrent, error) {
	f, err := os.Open(path) //used to structurally decode the .torrent file
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rt rawTorrent
	if err := bencode.Unmarshal(f, &rt); err != nil {
		return nil, err
	}

	f2, err := os.Open(path) // second time opening to build raw info hash
	if err != nil {
		return nil, err
	}
	defer f2.Close()

	raw, err := bencode.Decode(f2)
	if err != nil {
		return nil, err
	}

	torrentMap := raw.(map[string]interface{})

	infoMap := torrentMap["info"]
	var buf bytes.Buffer
	if err := bencode.Marshal(&buf, infoMap); err != nil {
		return nil, err
	}
	infoHash := sha1.Sum(buf.Bytes())

	t := &Torrent{
		Announce: rt.Announce,
		Info: Info{
			Name:        rt.Info.Name,
			PieceLength: rt.Info.PieceLength,
			Pieces:      rt.Info.Pieces,
			Length:      rt.Info.Length,
		},
		InfoHash: infoHash,
	}
	return t, nil
}
