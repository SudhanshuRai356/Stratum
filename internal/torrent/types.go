package torrent

type Info struct {
	Name        string `bencode:"name"`
	// This tag fixes the Magnet Link download!
	PieceLength int    `bencode:"piece length"` 
	Pieces      string `bencode:"pieces"`
	Length      int64    `bencode:"length,omitempty"`
	Files       []File `bencode:"files,omitempty"`
}

type File struct {
	Length int64      `bencode:"length"`
	Path   []string `bencode:"path"`
}

type Torrent struct {
	Announce     string
	AnnounceList [][]string
	InfoHash     [20]byte
	Info         Info
}