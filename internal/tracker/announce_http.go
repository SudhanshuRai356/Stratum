package tracker

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/zeebo/bencode"

	"github.com/Kushal-39/Stratum/internal/torrent"
)

func announceHTTP(announceURL string, t *torrent.Torrent, left int64) ([]Peer, error) {
	peerID := "-SG0001-123456789012"

	params := url.Values{}
	params.Add("info_hash", string(t.InfoHash[:]))
	params.Add("peer_id", peerID)
	params.Add("port", "6881")
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", strconv.FormatInt(left, 10))

	fullURL := announceURL + "?" + params.Encode()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var respMap map[string]any
	err = bencode.NewDecoder(resp.Body).Decode(&respMap)
	if err != nil {
		return nil, err
	}

	ok := respMap != nil
	if !ok {
		return nil, fmt.Errorf("invalid tracker response format")
	}

	seeders := -1
	leechers := -1
	if complete, ok := respMap["complete"].(int64); ok {
		seeders = int(complete)
	}
	if incomplete, ok := respMap["incomplete"].(int64); ok {
		leechers = int(incomplete)
	}
	if seeders >= 0 || leechers >= 0 {
		updateSwarmStats(seeders, leechers)
	}

	peersVal, ok := respMap["peers"]
	if !ok {
		return nil, fmt.Errorf("no peers field in response")
	}

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

	case []any:
		for _, peerVal := range p {
			peerMap, ok := peerVal.(map[string]any)
			if !ok {
				continue
			}
			ip, ok1 := peerMap["ip"].(string)
			portFloat, ok2 := peerMap["port"].(int64)
			if ok1 && ok2 {
				peers = append(peers, Peer{
					IP:   ip,
					Port: uint16(portFloat),
				})
			}
		}

	default:
		return nil, fmt.Errorf("unknown peers format: %T", peersVal)
	}

	return peers, nil
}
