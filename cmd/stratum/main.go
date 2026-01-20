package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/torrent"
	"github.com/Kushal-39/Stratum/internal/tracker"
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

	peers, err := tracker.Announce(t) //connecting to the tracker
	if err != nil {
		panic(err)
	}

	if len(peers) == 0 {
		panic("no peers returned by tracker")
	}

	peerID := generatePeerID() //generating peer ID
	handshakeDone := false

	for i := 0; i < len(peers) && i < 10; i++ {
		p := peer.Peer{
			IP:   peers[i].IP,
			Port: peers[i].Port,
		}

		conn, err := peer.PerformHandshake(p, t.InfoHash, peerID) //trying to perform handshake with known peers
		if err != nil {
			fmt.Println("Handshake failed with", p.IP, ":", err)
			continue
		}
		fmt.Println("Handshake successful with", p.IP)
		handshakeDone = true
		gotBitfield := false
		gotUnchoke := false
		requested := false

		for { //listening to peer's state machine
			msg, err := peer.ReadMessage(conn)
			if err != nil {
				if err == io.EOF {
					fmt.Println("Peer closed connection")
				} else {
					fmt.Println("Read error:", err)
				}
				break
			}

			switch msg.ID {
			case 1:
				fmt.Println("Received unchoke")
				gotUnchoke = true
			case 0:
				fmt.Println("Received choke")
			case 255:
				fmt.Println("Received keep-alive")
			case 4:
				pieceIndex := binary.BigEndian.Uint32(msg.Payload)
				fmt.Println("Peer has piece", pieceIndex)
			case 5:
				fmt.Println("Received bitfield, length:", len(msg.Payload))
				gotBitfield = true
				if err := peer.SendInterested(conn); err != nil {
					fmt.Println("Failed to send interested:", err)
				}
			case 7:
				piece := binary.BigEndian.Uint32(msg.Payload[0:4])
				begin := binary.BigEndian.Uint32(msg.Payload[4:8])
				block := msg.Payload[8:]

				fmt.Println("Received PIECE",
					"piece:", piece,
					"begin:", begin,
					"block length:", len(block))

			default:
				fmt.Println("Received message ID:", msg.ID)
			}
			if gotBitfield && gotUnchoke && !requested {
				// naive: request piece 0
				err := peer.SendReq(conn, 0, 0, peer.Blocksize)
				if err != nil {
					fmt.Println("Request failed:", err)
				}
				requested = true
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
