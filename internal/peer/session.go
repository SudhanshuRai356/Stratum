package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/zeebo/bencode"
)

type PeerSession struct {
	Conn               net.Conn // Direct net.Conn - standard Go network interface
	Peer               Peer     // Peer address info
	Bitfield           []byte   // Which pieces the peer has
	Choked             bool     // Whether peer is choking us
	InfoHash           [20]byte // Torrent info hash
	PeerID             [20]byte // Our peer ID
	Downloader         *Downloader
	SupportsExtensions bool
	ExtensionIDs map[string]int
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

// Now accepts peerReserved []byte as the last argument
func NewPeerSession(conn net.Conn, peer Peer, infoHash [20]byte, peerID [20]byte, peerReserved []byte) *PeerSession {
	// Check byte 5, bit 0x10 (20th bit from right) for extension support
	supportsExt := false
	if len(peerReserved) == 8 && (peerReserved[5]&0x10) != 0 {
		supportsExt = true
	}

	return &PeerSession{
		Conn:               conn,
		Peer:               peer,
		Choked:             true,
		InfoHash:           infoHash,
		PeerID:             peerID,
		SupportsExtensions: supportsExt, // Store this flag
		ExtensionIDs: 	 make(map[string]int),
	}
}
func send_Message(conn net.Conn, msgID byte, payload []byte) error {
	leng:=uint32(1+len(payload))
	buf:=make([]byte, 4)
	binary.BigEndian.PutUint32(buf, leng)
	_,e1:=conn.Write(buf)
	if e1!=nil{
		return e1
	}
	_,e2:=conn.Write([]byte{msgID})
	if e2!=nil{
		return e2
	}
	_,e3:=conn.Write(payload)
	return e3
}
func SendExtHandshake(conn net.Conn, supportsExtensions bool) error {
	ext_map:=map[string]int{
		"ut_metadata": 1,
	}
	ext_handshake:=map[string]interface{}{
		"m": ext_map,
	}
	var buf bytes.Buffer
	err:=bencode.NewEncoder(&buf).Encode(ext_handshake)
	if err!=nil{
		return err
	}
	payload:=append([]byte{0}, buf.Bytes()...)
	return send_Message(conn, MsgExtended, payload)
}
// WaitForReady reads messages until we have bitfield and unchoke.
// This must be called before downloading pieces.
func (s *PeerSession) WaitForReady() error {
	for s.Bitfield == nil || s.Choked {
		msg, err := ReadMessage(s.Conn)
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("peer closed connection before ready")
			}
			return fmt.Errorf("error reading message: %w", err)
		}

		switch msg.ID {
		case 0: // choke
			s.Choked = true
			fmt.Println("Received choke")
		case 1: // unchoke
			s.Choked = false
			fmt.Println("Received unchoke")
		case 4: // have
			pieceIndex := binary.BigEndian.Uint32(msg.Payload)
			s.updateBitfield(pieceIndex)
		case 5: // bitfield
			s.Bitfield = msg.Payload
			fmt.Println("Received bitfield, length:", len(msg.Payload))
			// Send INTERESTED immediately after receiving bitfield
			if err := SendInterested(s.Conn); err != nil {
				return fmt.Errorf("failed to send interested: %w", err)
			}
			fmt.Println("Sent INTERESTED")
		case MsgExtended:
			if len(msg.Payload) > 0 && msg.Payload[0] == 0{
				err:=SendExtHandshake(s.Conn, s.SupportsExtensions)
				if err != nil {
					return fmt.Errorf("failed to send extended handshake: %w", err)
				}
				var extHandshake map[string]interface{}
				e2:=bencode.DecodeBytes(msg.Payload[1:], &extHandshake)
				if e2 != nil {
					return fmt.Errorf("failed to decode extended message: %w", e2)
				}
				if mfield, ok := extHandshake["m"]; ok{
					if ext,ok:=mfield.(map[string]interface{}); ok {
						for k,v:=range ext{
							if id,ok:=v.(int64);ok{
								s.ExtensionIDs[k]=int(id)
							}
						}
					}
				}
			}
		case 255: // keep-alive
			// ignore
		default:
			fmt.Println("WaitForReady: ignoring message ID:", msg.ID)
		}
	}
	fmt.Println("Peer is ready (have bitfield + unchoked)")
	return nil
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

				// Pick next piece from SAME peer
				nextPiece, ok := pickNextPiece(s.Bitfield, completed)
				if !ok {
					fmt.Println("Peer has no more useful pieces")
					return nil // now we exit session
				}

				s.Downloader = newDownloader(nextPiece)

				fmt.Println("Starting piece", nextPiece)

				s.Downloader.StartPipeline(func(offset, length uint32) error {
					return SendReq(s.Conn, nextPiece, offset, length)
				})
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
func (s *PeerSession) RunSinglePiece(pidx uint32, comp []bool, new_downloader func(pieceIndex uint32) *Downloader) error {
	// NOTE: Connection is NOT closed here - caller manages connection lifecycle
	// Check if peer has this piece
	if !s.HasPiece(pidx) {
		return fmt.Errorf("peer doesn't have piece %d", pidx)
	}

	// Start the downloader for this piece
	s.Downloader = new_downloader(pidx)
	fmt.Println("Starting piece", pidx)
	s.Downloader.StartPipeline(func(offset, length uint32) error {
		return SendReq(s.Conn, pidx, offset, length)
	})

	// Read messages until piece is complete
	for {
		msg, e := ReadMessage(s.Conn)
		if e != nil {
			if e == io.EOF {
				return fmt.Errorf("peer closed connection")
			}
			return fmt.Errorf("peer error: %w", e)
		}
		switch msg.ID {
		case 0: // choke
			s.Choked = true
			fmt.Println("Received choke during download")
			return fmt.Errorf("peer choked us during download")
		case 1: // unchoke
			s.Choked = false
		case 4: // have
			pieceIndex := binary.BigEndian.Uint32(msg.Payload)
			s.updateBitfield(pieceIndex)
		case 5: // bitfield
			s.Bitfield = msg.Payload
		case 7: // piece
			if s.Downloader == nil {
				return fmt.Errorf("received PIECE with no active downloader")
			}
			done, e := s.Downloader.HandlePiece(msg.Payload, func(offset, length uint32) error {
				return SendReq(s.Conn, s.Downloader.PieceIndex, offset, length)
			})
			if e != nil {
				return fmt.Errorf("failed to handle piece: %w", e)
			}
			if done {
				comp[s.Downloader.PieceIndex] = true
				s.Downloader = nil // Clear downloader for next piece
				return nil
			}
		case 255: // keep-alive
			// ignore
		default:
			fmt.Println("RunSinglePiece: received message ID:", msg.ID)
		}
	}
}

// FetchMetadata fetches torrent metadata using BEP 09 ut_metadata protocol
func (s *PeerSession) FetchMetadata(infoHash [20]byte) ([]byte, error) {
	// Check if peer supports ut_metadata
	metadataExtID, hasMetadata := s.ExtensionIDs["ut_metadata"]
	if !hasMetadata {
		return nil, fmt.Errorf("peer does not support ut_metadata extension")
	}

	fmt.Printf("Requesting metadata using extension ID %d\n", metadataExtID)

	// First request piece 0 to get total size
	if err := s.sendMetadataRequest(metadataExtID, 0); err != nil {
		return nil, fmt.Errorf("failed to send metadata request: %w", err)
	}

	var totalSize int
	var metadata []byte

	// Wait for metadata pieces
	for {
		msg, err := ReadMessage(s.Conn)
		if err != nil {
			return nil, fmt.Errorf("failed to read message: %w", err)
		}

		if msg.ID == MsgExtended && len(msg.Payload) > 0 && int(msg.Payload[0]) == metadataExtID {
			// Parse metadata message
			var done bool
			var err error
			metadata, totalSize, done, err = s.handleMetadataMessage(msg.Payload[1:], metadata, totalSize)
			if err != nil {
				return nil, fmt.Errorf("failed to handle metadata message: %w", err)
			}
			if done {
				return metadata, nil
			}
		}
	}
}

// sendMetadataRequest sends a metadata request for a specific piece
func (s *PeerSession) sendMetadataRequest(extID, piece int) error {
	metaMsg := MetadataMessage{
		MsgType: MetadataRequest,
		Piece:   piece,
	}

	encoded, err := bencode.EncodeBytes(metaMsg)
	if err != nil {
		return fmt.Errorf("failed to encode metadata request: %w", err)
	}

	// Create extended message: [ext_id][bencoded_dict]
	payload := make([]byte, 1+len(encoded))
	payload[0] = byte(extID)
	copy(payload[1:], encoded)

	return send_Message(s.Conn, MsgExtended, payload)
}

// handleMetadataMessage processes a metadata response
func (s *PeerSession) handleMetadataMessage(payload []byte, metadata []byte, totalSize int) ([]byte, int, bool, error) {
	// Find where bencode ends and data begins
	var metaMsg MetadataMessage
	decoder := bencode.NewDecoder(bytes.NewReader(payload))
	dataStart := 0
	
	if err := decoder.Decode(&metaMsg); err != nil {
		return metadata, totalSize, false, fmt.Errorf("failed to decode metadata message: %w", err)
	}

	// Calculate where data starts (after bencode dict)
	for i := 0; i < len(payload); i++ {
		if payload[i] == 'e' { // End of bencode dict
			dataStart = i + 1
			break
		}
	}

	switch metaMsg.MsgType {
	case MetadataData:
		if totalSize == 0 {
			totalSize = metaMsg.TotalSize
			metadata = make([]byte, totalSize)
		}

		// Copy piece data
		pieceSize := 16384 // Standard metadata piece size
		start := metaMsg.Piece * pieceSize
		end := start + len(payload[dataStart:])
		if end > totalSize {
			end = totalSize
		}

		copy(metadata[start:end], payload[dataStart:])
		fmt.Printf("Received metadata piece %d (%d bytes)\n", metaMsg.Piece, len(payload[dataStart:]))

		// Check if we have all pieces
		if end >= totalSize {
			return metadata, totalSize, true, nil
		}

		// Request next piece
		nextPiece := metaMsg.Piece + 1
		if start+pieceSize < totalSize {
			extID := s.ExtensionIDs["ut_metadata"]
			if err := s.sendMetadataRequest(extID, nextPiece); err != nil {
				return metadata, totalSize, false, fmt.Errorf("failed to request next piece: %w", err)
			}
		}

	case MetadataReject:
		return metadata, totalSize, false, fmt.Errorf("metadata request rejected for piece %d", metaMsg.Piece)
	}

	return metadata, totalSize, false, nil
}
