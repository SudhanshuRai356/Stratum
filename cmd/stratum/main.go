package main

import (
	"flag"
	"fmt"
	"github.com/Kushal-39/Stratum/internal/peer"
	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
	"github.com/Kushal-39/Stratum/internal/tracker"
	"os"
	"path/filepath"
)

const PipelineDepth = 5

func allPiecesDone(completed []bool) bool {
	for _, done := range completed {
		if !done {
			return false
		}
	}
	return true
}

func main() {
	outDir := flag.String("o", ".", "output directory")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: stratum [-o output_dir] <path-to-torrent-file>")
		os.Exit(1)
	}

	torrentPath := flag.Arg(0)

	// Step 1: Parse the torrent file
	t, err := torrent.ParseTorrent(torrentPath)
	if err != nil {
		panic(err)
	}

	fmt.Println("Torrent parsed:", t.Info.Name)
	fmt.Println("Size:", t.Info.Length)
	absOutDir, err := filepath.Abs(*outDir)
	if err != nil {
		panic(err)
	}

	outputPath := filepath.Join(absOutDir, t.Info.Name)

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

	writer, err := storage.New(outputPath, t.Info.Length) //create a writer to write the downloaded things to the disk
	if err != nil {
		panic(err)
	}
	defer writer.Close()

	for !allPiecesDone(completed) {
		for i := 0; i < len(peers); i++ {
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

			if allPiecesDone(completed) {
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
}

func generatePeerID() [20]byte {
	var id [20]byte
	copy(id[:], "-ST0001-123456789012")
	return id
}
