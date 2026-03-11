package tracker

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

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

	var wg sync.WaitGroup
	// Create a channel to collect peers from all successful trackers
	peerChan := make(chan []Peer, len(announceURLs))

	// FIRE THE SHOTGUN: Query all trackers simultaneously
	for _, announce := range announceURLs {
		wg.Add(1)
		go func(trackerURL string) {
			defer wg.Done()

			var peers []Peer
			var err error

			if strings.HasPrefix(trackerURL, "udp://") {
				peers, err = AnnounceUDP(trackerURL, t)
			} else if strings.HasPrefix(trackerURL, "http://") || strings.HasPrefix(trackerURL, "https://") {
				fullURL := trackerURL + "?" + params.Encode()
				peers, err = announceHTTP(fullURL)
			} else {
				return // Unsupported protocol
			}

			if err != nil {
				// Silently drop dead trackers to avoid spamming the terminal
				return
			}

			if len(peers) > 0 {
				fmt.Printf("Tracker %s succeeded, got %d peers\n", trackerURL, len(peers))
				peerChan <- peers
			}
		}(announce)
	}

	// Wait for all tracker requests to finish (or timeout)
	wg.Wait()
	close(peerChan)

	// Deduplicate the peers using a map
	peerMap := make(map[string]Peer)
	for peerList := range peerChan {
		for _, p := range peerList {
			peerKey := fmt.Sprintf("%s:%d", p.IP, p.Port)
			peerMap[peerKey] = p
		}
	}

	var finalPeers []Peer
	for _, p := range peerMap {
		finalPeers = append(finalPeers, p)
	}

	if len(finalPeers) == 0 {
		return nil, fmt.Errorf("all trackers timed out or returned 0 peers")
	}

	return finalPeers, nil
}

func announceHTTP(announceURL string) ([]Peer, error) {
	// FIX: Never use default http.Get. Always enforce a strict timeout.
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(announceURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	trackerResp, err := bencode.Decode(resp.Body)
	if err != nil {
		return nil, err
	}

	respMap, ok := trackerResp.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid tracker response format")
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

	case []interface{}:
		for _, peerVal := range p {
			peerMap, ok := peerVal.(map[string]interface{})
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