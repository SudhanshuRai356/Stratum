package peer

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

type PeerSession struct {
	Conn       net.Conn // Direct net.Conn - standard Go network interface
	Peer       Peer     // Peer address info
	Bitfield   []byte   // Which pieces the peer has
	Choked     bool     // Whether peer is choking us
	InfoHash   [20]byte // Torrent info hash
	PeerID     [20]byte // Our peer ID
	Downloader *Downloader
}

func pickNextPiece(bitfield []byte, completed []bool) (uint32, bool) {
	for i := uint32(0); i < uint32(len(completed)); i++ {
		if completed[i] {
			continue
		}
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		if byteIdx < uint32(len(bitfield)) && (bitfield[byteIdx]&(1<<bitIdx)) != 0 {
			return i, true
		}
	}
	return 0, false
}

func NewPeerSession(conn net.Conn, peer Peer, infoHash [20]byte, peerID [20]byte) *PeerSession {
	return &PeerSession{
		Conn:     conn,
		Peer:     peer,
		Choked:   true, // Start as choked (default BitTorrent state)
		InfoHash: infoHash,
		PeerID:   peerID,
	}
}

func (s *PeerSession) Run(
	torrentPieces int,
	completed []bool,
	newDownloader func(pieceIndex uint32) *Downloader,
) error {
	defer s.Conn.Close()

	gotBitfield := false
	gotUnchoke := false

	for {
		msg, err := ReadMessage(s.Conn)
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("peer closed connection")
			}
			fmt.Println("Peer error:", err)
			return nil // allow main to try next peer
		}

		switch msg.ID {
		case 0: // choke
			s.Choked = true
			fmt.Println("Received choke")

		case 1: // unchoke
			s.Choked = false
			gotUnchoke = true
			fmt.Println("Received unchoke")

		case 4: // have
			pieceIndex := binary.BigEndian.Uint32(msg.Payload)
			s.updateBitfield(pieceIndex)
			fmt.Println("Peer has piece", pieceIndex)

		case 5: // bitfield
			s.Bitfield = msg.Payload
			gotBitfield = true
			fmt.Println("Received bitfield, length:", len(msg.Payload))

			if err := SendInterested(s.Conn); err != nil {
				return fmt.Errorf("failed to send interested: %w", err)
			}

		case 7: // piece
			if s.Downloader == nil {
				return fmt.Errorf("received PIECE with no active downloader")
			}

			done, err := s.Downloader.HandlePiece(
				msg.Payload,
				func(offset, length uint32) error {
					return SendReq(s.Conn, s.Downloader.PieceIndex, offset, length)
				},
			)
			if err != nil {
				return fmt.Errorf("piece handling failed: %w", err)
			}

			if done {
				completed[s.Downloader.PieceIndex] = true
				fmt.Println("Completed piece", s.Downloader.PieceIndex)

				s.Downloader = nil // ready for next piece

				continue
			}

		case 255: // keep-alive
			fmt.Println("Received keep-alive")

		default:
			fmt.Println("Received message ID:", msg.ID)
		}

		if gotBitfield && gotUnchoke && s.Downloader == nil {
			pieceIndex, ok := pickNextPiece(s.Bitfield, completed)
			if !ok {
				return fmt.Errorf("peer has no usable pieces")
			}

			s.Downloader = newDownloader(pieceIndex)

			fmt.Println("Starting piece", pieceIndex)

			s.Downloader.StartPipeline(func(offset, length uint32) error {
				return SendReq(s.Conn, pieceIndex, offset, length)
			})
		}

	}
}

// updateBitfield sets a bit in the bitfield for a given piece index.
func (s *PeerSession) updateBitfield(pieceIndex uint32) {
	byteIndex := pieceIndex / 8
	bitIndex := 7 - (pieceIndex % 8)

	if byteIndex >= uint32(len(s.Bitfield)) {
		// Expand bitfield if needed
		newBitfield := make([]byte, byteIndex+1)
		copy(newBitfield, s.Bitfield)
		s.Bitfield = newBitfield
	}

	s.Bitfield[byteIndex] |= 1 << bitIndex
}

func (s *PeerSession) Close() error {
	return s.Conn.Close()
}

func (s *PeerSession) HasPiece(pieceIndex uint32) bool {
	byteIndex := pieceIndex / 8
	bitIndex := 7 - (pieceIndex % 8)

	if byteIndex >= uint32(len(s.Bitfield)) {
		return false
	}

	return (s.Bitfield[byteIndex]>>bitIndex)&1 == 1
}
