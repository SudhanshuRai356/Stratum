package main

import (
	"fmt"
	"os"

	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
	"github.com/Kushal-39/Stratum/internal/tracker"
)

const PipelineDepth = 5

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: stratum <path-to-torrent-file>")
		os.Exit(1)
	}

	torrentPath := os.Args[1]

	// Step 1: Parse the torrent file
	t, err := torrent.ParseTorrent(torrentPath)
	if err != nil {
		panic(err)
	}

	fmt.Println("Torrent parsed:", t.Info.Name)
	fmt.Println("Size:", t.Info.Length)

	// Step 2: Get peers from tracker
	peers, err := tracker.Announce(t)
	if err != nil {
		panic(err)
	}

	if len(peers) == 0 {
		panic("no peers returned by tracker")
	}

	// Step 3: Try to connect to peers and download
	peerID := generatePeerID()
	handshakeDone := false
	totalPieces := int(len(t.Info.Pieces) / 20)
	completed := make([]bool, totalPieces)

	for i := 0; i < len(peers) ; i++ {
		p := peer.Peer{
			IP:   peers[i].IP,
			Port: peers[i].Port,
		}

		// Perform handshake using net.Conn
		conn, err := peer.PerformHandshake(p, t.InfoHash, peerID)
		if err != nil {
			fmt.Println("Handshake failed with", p.IP, ":", err)
			continue
		}
		fmt.Println("Handshake successful with", p.IP)
		handshakeDone = true

		// Step 4: Create session and downloader
		session := peer.NewPeerSession(conn, p, t.InfoHash, peerID)
		// Create storage writer for the output file
		writer, err := storage.New(t.Info.Name, t.Info.Length)
		if err != nil {
			fmt.Println("Failed to create writer:", err)
			continue
		}
		defer writer.Close()

		err = session.Run(
			totalPieces,
			completed,
			func(pieceIndex uint32) *peer.Downloader {
				return peer.NewDownloader(
					t,
					pieceIndex,
					PipelineDepth,
					writer,
				)
			},
		)
		if err != nil {
			fmt.Println("Session error:", err)
		}

		allDone := true
		for _, done := range completed {
			if !done {
				allDone = false
				break
			}
		}

		if allDone {
			fmt.Println("All pieces downloaded")
			return
		}

		// otherwise, try next peer
		continue

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
