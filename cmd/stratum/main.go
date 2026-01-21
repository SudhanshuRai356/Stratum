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

const PipelineDepth = 5

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
		gotUnchoke := false
		requested := false

		var bitfield []byte
		haveAny := false
		totalPieces := uint32(len(t.Info.Pieces) / 20)
		var (
			pieceIndex uint32
			assembler  *peer.PieceAssembler
			nextOffset uint32
			inflight   uint32
		)

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
			case 4: // HAVE
				piece := binary.BigEndian.Uint32(msg.Payload)
				haveAny = true

				if bitfield == nil {
					bitfield = make([]byte, (totalPieces+7)/8)
				}

				byteIndex := piece / 8
				bitIndex := 7 - (piece % 8)
				bitfield[byteIndex] |= 1 << bitIndex

			case 5: // bitfield
				fmt.Println("Received bitfield, length:", len(msg.Payload))
				bitfield = msg.Payload
				haveAny=true

				if err := peer.SendInterested(conn); err != nil {
					fmt.Println("Failed to send interested:", err)
				}

			case 7: // PIECE
				p := binary.BigEndian.Uint32(msg.Payload[0:4])
				begin := binary.BigEndian.Uint32(msg.Payload[4:8])
				block := msg.Payload[8:]

				fmt.Println(
					"Received PIECE",
					"piece:", p,
					"begin:", begin,
					"block length:", len(block),
				)

				// add block to assembler
				err := assembler.AddBlock(begin, block)
				if err != nil {
					fmt.Println("AddBlock failed:", err)
					break
				}

				// one request fulfilled
				inflight--

				// send next request if piece not complete
				if nextOffset < assembler.Length {
					blockSize := uint32(peer.Blocksize)
					if nextOffset+blockSize > assembler.Length {
						blockSize = assembler.Length - nextOffset
					}

					err := peer.SendReq(conn, pieceIndex, nextOffset, blockSize)
					if err != nil {
						fmt.Println("Request failed:", err)
					} else {
						nextOffset += blockSize
						inflight++
					}
				}
				if assembler.Received == assembler.Length {
					fmt.Println("Piece fully downloaded")

					expected := []byte(
						t.Info.Pieces[pieceIndex*20 : (pieceIndex+1)*20],
					)

					if assembler.Verify(expected) {
						fmt.Println("Piece hash VERIFIED")
					} else {
						fmt.Println("Piece hash MISMATCH")
					}

					return
				}

			default:
				fmt.Println("Received message ID:", msg.ID)
			}
			if gotUnchoke && !requested && haveAny {
				picked, ok := peer.PickPiece(bitfield, totalPieces)
				if !ok {
					fmt.Println("Peer advertised pieces but none usable")
					break
				}

				pieceIndex = picked
				fmt.Println("Selected piece:", pieceIndex)

				pieceLength := uint32(t.Info.PieceLength)
				assembler = peer.NewPieceAssembler(pieceIndex, pieceLength, peer.Blocksize)

				nextOffset = 0
				inflight = 0

				for i := 0; i < PipelineDepth && nextOffset < assembler.Length; i++ {
					blockSize := uint32(peer.Blocksize)
					if nextOffset+blockSize > assembler.Length {
						blockSize = assembler.Length - nextOffset
					}

					err := peer.SendReq(conn, pieceIndex, nextOffset, blockSize)
					if err != nil {
						fmt.Println("Request failed:", err)
						break
					}

					nextOffset += blockSize
					inflight++
				}

				requested = true
			}

		}
		conn.Close()
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
