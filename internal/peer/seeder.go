package peer

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
)

// maxConcurrentConns limits how many peer connections the seeder handles at once.
const maxConcurrentConns = 200

// Seeder listens for incoming peer connections and uploads pieces
type Seeder struct {
	listener    net.Listener
	tor         *torrent.Torrent
	writer      *storage.Writer
	bitfield    []byte // our completed pieces
	peerID      [20]byte
	running     bool
	mu          sync.RWMutex
	activeConns sync.WaitGroup
	uploaded    int64 // total bytes uploaded
	uploadMu    sync.Mutex
	peers       map[net.Conn]bool
	peerMu      sync.RWMutex
	connSem     chan struct{} // limits concurrent connections
}

// NewSeeder creates a new seeder that will serve pieces to leechers
func NewSeeder(tor *torrent.Torrent, writer *storage.Writer, completed []bool, peerID [20]byte, port int) (*Seeder, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	// Build bitfield from completed pieces
	numBytes := (len(completed) + 7) / 8
	bitfield := make([]byte, numBytes)
	for i, done := range completed {
		if done {
			byteIdx := i / 8
			bitIdx := 7 - (i % 8)
			bitfield[byteIdx] |= (1 << uint(bitIdx))
		}
	}

	return &Seeder{
		listener: listener,
		tor:      tor,
		writer:   writer,
		bitfield: bitfield,
		peerID:   peerID,
		running:  false,
		peers:    make(map[net.Conn]bool),
		connSem:  make(chan struct{}, maxConcurrentConns),
	}, nil
}

// Start begins accepting incoming connections
func (s *Seeder) Start() {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	fmt.Printf("Seeding on %s\n", s.listener.Addr().String())

	go s.acceptLoop()
}

// Stop gracefully shuts down the seeder
func (s *Seeder) Stop() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	s.listener.Close()
	s.activeConns.Wait() // wait for all active uploads to finish
	fmt.Printf("Seeder stopped. Total uploaded: %d bytes\n", s.uploaded)
}

// MarkPieceAvailable safely updates the seeder's bitfield when a new piece is downloaded
func (s *Seeder) MarkPieceAvailable(pieceIndex uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	byteIdx := pieceIndex / 8
	bitIdx := 7 - (pieceIndex % 8)

	// Ensure we don't panic if the bitfield is somehow sized incorrectly
	if byteIdx < uint32(len(s.bitfield)) {
		s.bitfield[byteIdx] |= (1 << uint(bitIdx))
	}
}

// IsRunning returns whether seeder is active
func (s *Seeder) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetUploaded returns total bytes uploaded
func (s *Seeder) GetUploaded() int64 {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	return s.uploaded
}

// Addr returns the listener address as a string (host:port)
func (s *Seeder) Addr() string {
	return s.listener.Addr().String()
}

func (s *Seeder) acceptLoop() {
	for {
		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()
		if !running {
			return
		}

		// Set a deadline so we can check running periodically
		s.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))
		conn, err := s.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // just a timeout, check running flag and retry
			}
			s.mu.RLock()
			if s.running {
				fmt.Printf("Accept error: %v\n", err)
			}
			s.mu.RUnlock()
			return
		}

		s.activeConns.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Seeder) handleConnection(conn net.Conn) {
	defer s.activeConns.Done()

	// Enforce concurrent connection limit
	select {
	case s.connSem <- struct{}{}:
		defer func() { <-s.connSem }()
	default:
		conn.Close()
		return
	}
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	fmt.Printf("Incoming peer connection from %s\n", remoteAddr)

	// Step 1: Receive handshake from leecher
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	infoHash, err := receiveIncomingHandshake(conn)
	if err != nil {
		fmt.Printf("Handshake failed from %s: %v\n", remoteAddr, err)
		return
	}

	// Verify infohash matches our torrent
	if infoHash != s.tor.InfoHash {
		fmt.Printf("InfoHash mismatch from %s\n", remoteAddr)
		return
	}

	// Step 2: Send our handshake back
	if err := sendHandshake(conn, s.tor.InfoHash, s.peerID); err != nil {
		fmt.Printf("Failed to send handshake to %s: %v\n", remoteAddr, err)
		return
	}

	conn.SetDeadline(time.Time{}) // clear deadline for normal operation

	// Step 3: Send our bitfield
	if err := s.sendBitfield(conn); err != nil {
		fmt.Printf("Failed to send bitfield to %s: %v\n", remoteAddr, err)
		return
	}

	// NEW: Register peer for broadcasts
	s.peerMu.Lock()
	s.peers[conn] = true
	s.peerMu.Unlock()

	defer func() {
		s.peerMu.Lock()
		delete(s.peers, conn)
		s.peerMu.Unlock()
	}()

	// Step 4: Handle messages from leecher (requests for pieces)
	s.serveLoop(conn, remoteAddr)
}

// BroadcastHave tells all connected leechers that we just acquired a new piece
func (s *Seeder) BroadcastHave(pieceIndex uint32) {
	s.peerMu.RLock()
	defer s.peerMu.RUnlock()

	if len(s.peers) == 0 {
		return // No one to tell
	}

	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, pieceIndex)

	for conn := range s.peers {
		// MsgHave is usually ID 4. Assuming your send_Message takes (conn, ID, payload)
		// and MsgHave is defined in your peer package.
		go send_Message(conn, MsgHave, payload)
	}
}

// receiveIncomingHandshake reads a handshake from an incoming connection
func receiveIncomingHandshake(conn net.Conn) ([20]byte, error) {
	var infoHash [20]byte

	buf := make([]byte, handshakeLen)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return infoHash, fmt.Errorf("failed to read handshake: %w", err)
	}

	if buf[0] != byte(len(protocolString)) {
		return infoHash, fmt.Errorf("invalid protocol string length")
	}
	if string(buf[1:20]) != protocolString {
		return infoHash, fmt.Errorf("invalid protocol string")
	}

	copy(infoHash[:], buf[28:48])
	return infoHash, nil
}

func (s *Seeder) sendBitfield(conn net.Conn) error {
	return send_Message(conn, MsgBitfield, s.bitfield)
}

func (s *Seeder) serveLoop(conn net.Conn, remoteAddr string) {
	choked := true // leecher starts choked

	for {
		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()
		if !running {
			return
		}

		conn.SetReadDeadline(time.Now().Add(2 * time.Minute)) // keep-alive timeout
		msg, err := ReadMessage(conn)
		if err != nil {
			return // peer disconnected
		}

		switch msg.ID {
		case MsgInterested:
			// Leecher is interested, unchoke them
			if choked {
				if err := s.sendUnchoke(conn); err != nil {
					return
				}
				choked = false
				fmt.Printf("Unchoked peer %s\n", remoteAddr)
			}

		case MsgNotInterested:
			// Choke them back
			if !choked {
				s.sendChoke(conn)
				choked = true
			}

		case MsgRequest:
			if choked {
				continue // ignore requests while choked
			}
			if len(msg.Payload) != 12 {
				continue
			}
			pieceIndex := binary.BigEndian.Uint32(msg.Payload[0:4])
			begin := binary.BigEndian.Uint32(msg.Payload[4:8])
			length := binary.BigEndian.Uint32(msg.Payload[8:12])

			// Sanity check the request
			if length > 32*1024 { // max 32KB per block
				continue
			}

			if err := s.sendPiece(conn, pieceIndex, begin, length); err != nil {
				fmt.Printf("Failed to send piece %d to %s: %v\n", pieceIndex, remoteAddr, err)
				return
			}

		case MsgCancel:
			// We don't queue requests so cancel is a no-op
			continue

		case 255: // keep-alive
			continue

		default:
			// ignore unknown messages
		}
	}
}

func (s *Seeder) sendUnchoke(conn net.Conn) error {
	return send_Message(conn, MsgUnchoke, nil)
}

func (s *Seeder) sendChoke(conn net.Conn) error {
	return send_Message(conn, MsgChoke, nil)
}

func (s *Seeder) sendPiece(conn net.Conn, pieceIndex, begin, length uint32) error {
	// Check if we actually have this piece (THREAD SAFE)
	byteIdx := pieceIndex / 8
	bitIdx := 7 - (pieceIndex % 8)

	s.mu.RLock()
	hasPiece := byteIdx < uint32(len(s.bitfield)) && (s.bitfield[byteIdx]&(1<<bitIdx)) != 0
	s.mu.RUnlock()

	if !hasPiece {
		return fmt.Errorf("we don't have piece %d", pieceIndex)
	}

	// Read the data from disk
	offset := s.tor.PieceOffset(int(pieceIndex)) + int64(begin)
	data := make([]byte, length)
	_, err := s.writer.ReadAt(data, offset)
	if err != nil {
		return fmt.Errorf("failed to read piece data: %w", err)
	}

	// Build piece message payload: index(4) + begin(4) + block(variable)
	payload := make([]byte, 8+len(data))
	binary.BigEndian.PutUint32(payload[0:4], pieceIndex)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	copy(payload[8:], data)

	err = send_Message(conn, MsgPiece, payload)
	if err != nil {
		return err
	}

	// Track uploaded bytes
	s.uploadMu.Lock()
	s.uploaded += int64(length)
	s.uploadMu.Unlock()

	return nil
}
