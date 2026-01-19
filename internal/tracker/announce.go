package tracker

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	bencode "github.com/jackpal/bencode-go"

	"github.com/Kushal-39/Stratum/internal/torrent"
)

func Announce(t *torrent.Torrent) error {
	peerID := "-SG0001-123456789012" // 20 bytes, fixed for now

	params := url.Values{}
	params.Add("info_hash", string(t.InfoHash[:]))
	params.Add("peer_id", peerID)
	params.Add("port", "6881")
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", strconv.FormatInt(t.Info.Length, 10))
	params.Add("compact", "1")

	announceURL := t.Announce + "?" + params.Encode()
	fmt.Println("Announce URL:", announceURL)

	resp, err := http.Get(announceURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	trackerResp, err := bencode.Decode(resp.Body)
	if err != nil {
		return err
	}

	respMap := trackerResp.(map[string]interface{})
	peersVal := respMap["peers"]

	switch p := peersVal.(type) {

	case string:
		fmt.Println("Compact peers")
		fmt.Println("Peers blob length:", len(p))
		fmt.Println("Number of peers:", len(p)/6)

	case []interface{}:
		fmt.Println("Non-compact peers")
		fmt.Println("Number of peers:", len(p))

		for i, peer := range p {
			peerMap := peer.(map[string]interface{})
			ip := peerMap["ip"].(string)
			port := peerMap["port"].(int64)

			fmt.Printf("Peer %d: %s:%d\n", i, ip, port)

			if i >= 4 {
				break
			}
		}

	default:
		return fmt.Errorf("unknown peers format: %T", peersVal)
	}

	return nil
}
