package tracker

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Kushal-39/Stratum/internal/torrent"
)

func Announce(t *torrent.Torrent, left int64) ([]Peer, error) {
	announceURLs := t.GetAllAnnounceURLs()
	if len(announceURLs) == 0 {
		return nil, fmt.Errorf("no trackers in torrent")
	}

	var wg sync.WaitGroup
	peerChan := make(chan []Peer, len(announceURLs))

	for _, announceURL := range announceURLs {
		wg.Go(func() {
			var peers []Peer
			var err error

			if strings.HasPrefix(announceURL, "udp://") {
				peers, err = announceUDP(announceURL, t, left)
			} else if strings.HasPrefix(announceURL, "http://") || strings.HasPrefix(announceURL, "https://") {
				peers, err = announceHTTP(announceURL, t, left)
			} else {
				return
			}

			if err != nil {
				return
			}

			if len(peers) > 0 {
				fmt.Printf("Tracker %s succeeded, got %d peers\n", announceURL, len(peers))
				peerChan <- peers
			}
		})
	}

	wg.Wait()
	close(peerChan)

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
