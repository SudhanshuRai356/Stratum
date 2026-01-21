package torrent

type Info struct {
	Name        string
	PieceLength int
	Pieces      string
	Length      int64
}

type Torrent struct {
	Announce string
	AnnounceList [][]string 
	Info     Info
	InfoHash [20]byte
}
