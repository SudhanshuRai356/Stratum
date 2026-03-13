package api

type TorrentState struct {
	InfoHash       string  `json:"info_hash"`
	Name           string  `json:"name"`
	Progress       float64 `json:"progress"`
	DownloadSpeed  float64 `json:"download_speed"`
	State          string  `json:"state"`
	ConnectedPeers int     `json:"connected_peers"`
}
