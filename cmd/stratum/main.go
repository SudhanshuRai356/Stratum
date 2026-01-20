package main

import (
	"fmt"
	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/torrent"
	"github.com/Kushal-39/Stratum/internal/tracker"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: stratum <path-to-torrent-file>")
		os.Exit(1)
	}

	torrentPath := os.Args[1]

	t, err := torrent.ParseTorrent(torrentPath) //parsing the .torrent
	if err != nil {
		panic(err)
	}

	fmt.Println("Torrent parsed:", t.Info.Name)
	fmt.Println("Size:", t.Info.Length)

	peers, err := tracker.Announce(t)  //connecting to the tracker
	if err != nil {
		panic(err)
	}

	if len(peers) == 0 {
		panic("no peers returned by tracker")
	}

	peerID := generatePeerID()  //generating peer ID
	handshakeDone := false

	for i := 0; i < len(peers) && i < 10; i++ {
		p := peer.Peer{
			IP:   peers[i].IP,
			Port: peers[i].Port,
		}

		conn, err := peer.PerformHandshake(p, t.InfoHash, peerID)  //trying to perform handshake with known peers
		if err != nil {
			fmt.Println("Handshake failed with", p.IP, ":", err)
			continue
		}
		fmt.Println("Handshake successful with", p.IP)
		handshakeDone = true

		for { //listening to peer's state machine
			msg, err := peer.ReadMessage(conn)
			if err != nil {
				fmt.Println("Read error:", err)
				break
			}

			switch msg.ID {
			case 5:
				fmt.Println("Received bitfield, length:", len(msg.Payload))
			case 1:
				fmt.Println("Received unchoke")
			case 0:
				fmt.Println("Received choke")
			case 255:
				fmt.Println("Received keep-alive")
			default:
				fmt.Println("Received message ID:", msg.ID)
			}
		}
		conn.Close()
		break

	}

	if !handshakeDone {
		fmt.Println("No successful handshakes")
	}

}

func generatePeerID() [20]byte {
	var id [20]byte
	copy(id[:], "-ST0001-123456789012")
	return id
}
