package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	bencode "github.com/jackpal/bencode-go"
	"io"
	"net"
	"time"
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
	ExtensionIDs       map[string]int
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
	conn.SetDeadline(time.Time{})

	supportsExt := false
	if len(peerReserved) == 8 && (peerReserved[5]&0x10) != 0 {
		supportsExt = true
	}

	s := &PeerSession{
		Conn:               conn,
		Peer:               peer,
		Choked:             true,
		InfoHash:           infoHash,
		PeerID:             peerID,
		SupportsExtensions: supportsExt,
		ExtensionIDs:       make(map[string]int),
	}

	if s.SupportsExtensions {
		SendExtHandshake(s.Conn, true)
	}

	return s
}
func send_Message(conn net.Conn, msgID byte, payload []byte) error {
	leng := uint32(1 + len(payload))
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, leng)
	_, e1 := conn.Write(buf)
	if e1 != nil {
		return e1
	}
	_, e2 := conn.Write([]byte{msgID})
	if e2 != nil {
		return e2
	}
	_, e3 := conn.Write(payload)
	return e3
}
func SendExtHandshake(conn net.Conn, supportsExtensions bool) error {
	ext_map := map[string]int{
		"ut_metadata": 1,
	}
	ext_handshake := map[string]interface{}{
		"m": ext_map,
	}
	var buf bytes.Buffer
	err := bencode.Marshal(&buf, ext_handshake)
	if err != nil {
		return err
	}
	payload := append([]byte{0}, buf.Bytes()...)
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
			if len(msg.Payload) > 0 && msg.Payload[0] == 0 {
				// Use the struct we defined in messages.go
				var payload ExtendedHandshakePayload
				e2 := bencode.Unmarshal(bytes.NewReader(msg.Payload[1:]), &payload)
				if e2 != nil {
					return fmt.Errorf("failed to decode extended message: %w", e2)
				}

				// Extract ut_metadata ID directly from the typed map
				if id, ok := payload.M["ut_metadata"]; ok {
					s.ExtensionIDs["ut_metadata"] = id
					// Optional: fmt.Printf("Peer supports ut_metadata with ID %d\n", id)
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
		case MsgExtended:
			// We are busy downloading; just ignore extension messages (like PEX or metadata requests)
			// Consuming them here prevents them from hitting 'default'
		case 255: // keep-alive
			// ignore
		default:
			fmt.Println("RunSinglePiece: received message ID:", msg.ID)
		}
	}
}

// FetchMetadata fetches torrent metadata using BEP 09 ut_metadata protocol
func (s *PeerSession) FetchMetadata(infoHash [20]byte) ([]byte, error) {
	peerExtensionID, hasMetadata := s.ExtensionIDs["ut_metadata"]
	if !hasMetadata {
		return nil, fmt.Errorf("peer does not support ut_metadata extension")
	}

	const ourExtensionID = 1

	fmt.Printf("Requesting metadata using peer's extension ID %d (expecting reply on ID %d)\n", peerExtensionID, ourExtensionID)

	// First request piece 0 to get total size
	if err := s.sendMetadataRequest(peerExtensionID, 0); err != nil {
		return nil, fmt.Errorf("failed to send metadata request: %w", err)
	}

	var totalSize int
	var metadata []byte

	// Initial strict deadline
	s.Conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	defer s.Conn.SetReadDeadline(time.Time{})

	// Wait for metadata pieces
	for {
		msg, err := ReadMessage(s.Conn)
		if err != nil {
			return nil, fmt.Errorf("failed to read message: %w", err)
		}

		s.Conn.SetReadDeadline(time.Now().Add(15 * time.Second))

		if msg.ID == MsgExtended {
			if len(msg.Payload) > 0 {
				incomingID := int(msg.Payload[0])

				if incomingID == ourExtensionID {
					var done bool
					var err error
					metadata, totalSize, done, err = s.handleMetadataMessage(msg.Payload[1:], metadata, totalSize)
					if err != nil {
						// Don't kill connection on decode error, might be partial data
						fmt.Printf("Error handling metadata message: %v\n", err)
						continue
					}
					if done {
						return metadata, nil
					}

				} else {
					fmt.Printf("Ignored unexpected MsgExtended ID: %d (Expected %d)\n", incomingID, ourExtensionID)
				}
			}
		} else {
		}
	}
}

// sendMetadataRequest sends a metadata request for a specific piece
func (s *PeerSession) sendMetadataRequest(extID, piece int) error {
	metaMsg := MetadataMessage{
		MsgType: MetadataRequest,
		Piece:   piece,
	}

	var encodedBuf bytes.Buffer
	if err := bencode.Marshal(&encodedBuf, metaMsg); err != nil {
		return fmt.Errorf("failed to encode metadata request: %w", err)
	}
	encoded := encodedBuf.Bytes()

	// Create extended message: [ext_id][bencoded_dict]
	payload := make([]byte, 1+len(encoded))
	payload[0] = byte(extID)
	copy(payload[1:], encoded)

	return send_Message(s.Conn, MsgExtended, payload)
}

// handleMetadataMessage processes a metadata response
func (s *PeerSession) handleMetadataMessage(payload []byte, metadata []byte, totalSize int) ([]byte, int, bool, error) {
	// 1. Manually find the end of the Bencode dictionary.
	// We cannot use bencode.Unmarshal(bytes.NewBuffer(...)) because it buffers
	// (over-reads) and loses the start of the binary data.

	headerEnd, err := findBencodeEnd(payload)
	if err != nil {
		return metadata, totalSize, false, fmt.Errorf("invalid bencode structure: %w", err)
	}

	// 2. Decode strictly the header part
	headerBytes := payload[:headerEnd]
	data := payload[headerEnd:] // This is the binary data, preserved intact!

	var metaMsg MetadataMessage
	if err := bencode.Unmarshal(bytes.NewReader(headerBytes), &metaMsg); err != nil {
		return metadata, totalSize, false, fmt.Errorf("failed to decode metadata header: %w", err)
	}

	switch metaMsg.MsgType {
	case MetadataData:
		if totalSize == 0 {
			totalSize = metaMsg.TotalSize
			if totalSize <= 0 {
				return metadata, totalSize, false, fmt.Errorf("invalid metadata size: %d", totalSize)
			}
			metadata = make([]byte, totalSize)
			fmt.Printf("Metadata total size: %d bytes\n", totalSize)
		}

		// Calculate offsets
		pieceSize := 16384
		start := metaMsg.Piece * pieceSize

		if start >= totalSize {
			return metadata, totalSize, false, fmt.Errorf("piece index %d out of bounds (size %d)", metaMsg.Piece, totalSize)
		}

		end := start + len(data)
		if end > totalSize {
			end = totalSize
		}

		copy(metadata[start:end], data)
		fmt.Printf("Received metadata piece %d (%d bytes)\n", metaMsg.Piece, len(data))

		// Check completion by piece index
		lastPieceIndex := (totalSize - 1) / pieceSize
		if metaMsg.Piece >= lastPieceIndex {
			return metadata, totalSize, true, nil
		}

		// Request next piece
		nextPiece := metaMsg.Piece + 1
		extID := s.ExtensionIDs["ut_metadata"]
		if err := s.sendMetadataRequest(extID, nextPiece); err != nil {
			return metadata, totalSize, false, fmt.Errorf("failed to request next piece: %w", err)
		}

	case MetadataReject:
		return metadata, totalSize, false, fmt.Errorf("metadata request rejected for piece %d", metaMsg.Piece)
	}

	return metadata, totalSize, false, nil
}

// findBencodeEnd scans the byte slice to find the index where the first bencode object ends.
func findBencodeEnd(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("empty data")
	}

	idx := 0
	depth := 0

	// Helper to read integer/length
	// Returns the index after the 'e' or ':'
	scanInt := func(start int, term byte) (int, int, error) {
		i := start
		for i < len(data) && data[i] >= '0' && data[i] <= '9' {
			i++
		}
		if i == start {
			return 0, 0, fmt.Errorf("expected number at %d", start)
		}
		if i >= len(data) || data[i] != term {
			return 0, 0, fmt.Errorf("expected terminator '%c' at %d", term, i)
		}
		// Parse number
		numVal := 0
		// A real implementation would parse generic int, here we just need positive for string length
		// For loop to parse int
		for j := start; j < i; j++ {
			numVal = numVal*10 + int(data[j]-'0')
		}
		return i + 1, numVal, nil
	}

	for idx < len(data) {
		char := data[idx]

		switch char {
		case 'd', 'l':
			depth++
			idx++
		case 'i':
			// Integer: i<number>e. Scan until 'e'
			idx++
			next, _, err := scanInt(idx, 'e')
			if err != nil {
				return 0, err
			}
			idx = next
			if depth == 0 {
				return idx, nil
			} // Should not happen for integer at root
		case 'e':
			depth--
			idx++
			if depth == 0 {
				return idx, nil // Found end of root object
			}
		default:
			if char >= '0' && char <= '9' {
				// String: <len>:<string>
				next, strLen, err := scanInt(idx, ':')
				if err != nil {
					return 0, err
				}
				idx = next + strLen
				if idx > len(data) {
					return 0, fmt.Errorf("string length out of bounds")
				}
				if depth == 0 {
					return idx, nil
				} // Should not happen for string at root
			} else {
				return 0, fmt.Errorf("unexpected character '%c' at %d", char, idx)
			}
		}
	}
	return 0, fmt.Errorf("incomplete bencode")
}
