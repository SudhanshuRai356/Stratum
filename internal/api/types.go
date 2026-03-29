package api

type TorrentState struct {
	InfoHash        string  `json:"info_hash"`
	Name            string  `json:"name"`
	Progress        float64 `json:"progress"`
	DownloadSpeed   float64 `json:"download_speed"`
	UploadSpeed     float64 `json:"upload_speed,omitempty"`
	State           string  `json:"state"`
	ConnectedPeers  int     `json:"connected_peers"`
	Seeders         int     `json:"seeders,omitempty"`
	Leechers        int     `json:"leechers,omitempty"`
	CompletedPieces int     `json:"completed_pieces,omitempty"`
	TotalPieces     int     `json:"total_pieces,omitempty"`
	Round           int     `json:"round,omitempty"`
}
