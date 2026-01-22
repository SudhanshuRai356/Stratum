package tracker

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	bencode "github.com/jackpal/bencode-go"

	"github.com/Kushal-39/Stratum/internal/torrent"
)

func Announce(t *torrent.Torrent) ([]Peer, error) {
	peerID := "-SG0001-123456789012"

	params := url.Values{}
	params.Add("info_hash", string(t.InfoHash[:]))
	params.Add("peer_id", peerID)
	params.Add("port", "6881")
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", strconv.FormatInt(t.Info.Length, 10))
	params.Add("compact", "1")

	announceURLs := t.GetAllAnnounceURLs()
	if len(announceURLs) == 0 {
		return nil, fmt.Errorf("no trackers in torrent")
	}
	var lastErr error

	for _, announce := range announceURLs {
		fmt.Println("Trying tracker:", announce)

		var peers []Peer
		var err error

		if strings.HasPrefix(announce, "udp://") {
			peers, err = AnnounceUDP(announce, t)
		} else if strings.HasPrefix(announce, "http://") || strings.HasPrefix(announce, "https://") {
			announceURL := announce + "?" + params.Encode()
			peers, err = announceHTTP(announceURL)
		} else {
			continue
		}

		if err != nil {
			fmt.Println("Tracker failed:", err)
			lastErr = err
			continue
		}

		if len(peers) == 0 {
			fmt.Println("Tracker returned no peers")
			continue
		}

		fmt.Println("Tracker succeeded:", announce)
		return peers, nil
	}

	return nil, fmt.Errorf("all trackers failed: %v", lastErr)

}

func announceHTTP(announceURL string) ([]Peer, error) {
	resp, err := http.Get(announceURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	trackerResp, err := bencode.Decode(resp.Body)
	if err != nil {
		return nil, err
	}

	respMap := trackerResp.(map[string]interface{})
	peersVal := respMap["peers"]

	var peers []Peer

	switch p := peersVal.(type) {
	case string:
		for i := 0; i+6 <= len(p); i += 6 {
			ip := fmt.Sprintf("%d.%d.%d.%d",
				p[i], p[i+1], p[i+2], p[i+3],
			)
			port := uint16(p[i+4])<<8 | uint16(p[i+5])

			peers = append(peers, Peer{
				IP:   ip,
				Port: port,
			})
		}

	case []interface{}:
		for _, peerVal := range p {
			peerMap := peerVal.(map[string]interface{})
			ip := peerMap["ip"].(string)
			port := uint16(peerMap["port"].(int64))

			peers = append(peers, Peer{
				IP:   ip,
				Port: port,
			})
		}

	default:
		return nil, fmt.Errorf("unknown peers format: %T", peersVal)
	}

	return peers, nil
}
