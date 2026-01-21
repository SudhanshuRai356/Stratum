package peer

import (
"encoding/binary"
"fmt"
"io"
"net"
)

type PeerSession struct {
	Conn     net.Conn  // Direct net.Conn - standard Go network interface
	Peer     Peer      // Peer address info
	Bitfield []byte    // Which pieces the peer has
	Choked   bool      // Whether peer is choking us
	InfoHash [20]byte  // Torrent info hash
	PeerID   [20]byte  // Our peer ID
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

func (s *PeerSession) Run(dl *Downloader) error {
	defer s.Conn.Close()

	gotBitfield := false
	gotUnchoke := false
	requested := false

	for {
		msg, err := ReadMessage(s.Conn)
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("peer closed connection")
			}
			return fmt.Errorf("read error: %w", err)
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
			done, err := dl.HandlePiece(s.Conn, msg.Payload)
			if err != nil {
				return fmt.Errorf("piece handling failed: %w", err)
			}
			if done {
				fmt.Println("Download complete")
				return nil
			}

		case 255: // keep-alive
			fmt.Println("Received keep-alive")

		default:
			fmt.Println("Received message ID:", msg.ID)
		}

		if gotBitfield && gotUnchoke && !requested {
			dl.StartPipeline(s.Conn)
			requested = true
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
