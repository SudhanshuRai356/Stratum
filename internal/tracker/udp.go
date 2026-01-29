package tracker

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"time"

	"github.com/Kushal-39/Stratum/internal/torrent"
)

func AnnounceUDP(trackerURL string, t *torrent.Torrent) ([]Peer, error) {
	u, err := url.Parse(trackerURL)
	if err != nil {
		return nil, err
	}

	addr := u.Host
	if u.Port() == "" {
		addr += ":80"
	}

	conn, err := net.DialTimeout("udp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	connectTxID := rand.Uint32()

	// ---- CONNECT REQUEST ----
	req := make([]byte, 16)
	binary.BigEndian.PutUint64(req[0:8], 0x41727101980) // protocol ID
	binary.BigEndian.PutUint32(req[8:12], 0)            // action = connect
	binary.BigEndian.PutUint32(req[12:16], connectTxID)

	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	resp := make([]byte, 1500)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, err
	}

	if binary.BigEndian.Uint32(resp[0:4]) != 0 {
		return nil, fmt.Errorf("invalid connect response")
	}

	connectionID := binary.BigEndian.Uint64(resp[8:16])

	// ---- ANNOUNCE REQUEST ----
	announce := make([]byte, 98)
	announceTxID := rand.Uint32()

	binary.BigEndian.PutUint64(announce[0:8], connectionID)
	binary.BigEndian.PutUint32(announce[8:12], 1) // action = announce
	binary.BigEndian.PutUint32(announce[12:16], announceTxID)

	copy(announce[16:36], t.InfoHash[:])
	copy(announce[36:56], []byte("-ST0001-123456789012"))

	binary.BigEndian.PutUint64(announce[56:64], 0)                     // downloaded
	binary.BigEndian.PutUint64(announce[64:72], uint64(t.Info.Length)) // left
	binary.BigEndian.PutUint64(announce[72:80], 0)                     // uploaded
	binary.BigEndian.PutUint32(announce[80:84], 0)                     // event
	binary.BigEndian.PutUint32(announce[84:88], 0)                     // IP
	binary.BigEndian.PutUint32(announce[88:92], rand.Uint32())         // key
	binary.BigEndian.PutUint32(announce[92:96], 0xFFFFFFFF)            // num_want
	binary.BigEndian.PutUint16(announce[96:98], 6881)                  // port

	if _, err := conn.Write(announce); err != nil {
		return nil, err
	}

	n, err = conn.Read(resp)
	if err != nil {
		return nil, err
	}

	if binary.BigEndian.Uint32(resp[0:4]) != 1 {
		return nil, fmt.Errorf("invalid announce response")
	}

	interval := binary.BigEndian.Uint32(resp[8:12])
	leechers := binary.BigEndian.Uint32(resp[12:16])
	seeders := binary.BigEndian.Uint32(resp[16:20])

	fmt.Println("UDP tracker response:")
	fmt.Println(" Interval:", interval)
	fmt.Println(" Leechers:", leechers)
	fmt.Println(" Seeders:", seeders)

	var peers []Peer
	for i := 20; i+6 <= n; i += 6 {
		rawIp := net.IPv4(resp[i], resp[i+1], resp[i+2], resp[i+3])
		ip:=rawIp.String()
		port := binary.BigEndian.Uint16(resp[i+4 : i+6])

		if ip== "0.0.0.0" || port == 0 {
			continue
		}
		if !rawIp.IsGlobalUnicast() || port == 0 {
			continue
		}

		peers = append(peers, Peer{
			IP:   ip,
			Port: port,
		})
	}

	if len(peers) == 0 {
		return nil, fmt.Errorf("no peers from UDP tracker")
	}

	return peers, nil
}
