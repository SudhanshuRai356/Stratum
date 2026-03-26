package peer

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

const (
	protocolString = "BitTorrent protocol"
	handshakeLen   = 68
)

// Returns conn, reserved_bytes, error
func PerformHandshake(p Peer, infoHash [20]byte, peerID [20]byte) (net.Conn, []byte, error) {
	address := net.JoinHostPort(p.IP, strconv.Itoa(int(p.Port)))
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, nil, err
	}
	if err := sendHandshake(conn, infoHash, peerID); err != nil {
		conn.Close()
		return nil, nil, err
	}
	// Capture reserved bytes here
	reserved, err := receiveHandshake(conn, infoHash)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, reserved, nil
}

func sendHandshake(conn net.Conn, infoHash [20]byte, peerID [20]byte) error {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(len(protocolString))) // pstrlen
	buf.WriteString(protocolString)          // pstr

	// NEW: Create reserved bytes and set the extension bit
	reserved := make([]byte, 8)
	reserved[5] |= 0x10 // Set 20th bit from right (Byte 5, 0x10)
	buf.Write(reserved)

	buf.Write(infoHash[:]) // infohash
	buf.Write(peerID[:])   // peerID
	_, err := conn.Write(buf.Bytes())
	return err
}

// Changed return type to ([]byte, error)
func receiveHandshake(conn net.Conn, expected_infoHash [20]byte) ([]byte, error) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp := make([]byte, handshakeLen)

	// Use ReadFull to ensure we get all 68 bytes
	_, err := io.ReadFull(conn, resp)
	if err != nil {
		return nil, err
	}
	if resp[0] != byte(len(protocolString)) {
		return nil, errors.New("invalid protocol string length")
	}
	if string(resp[1:20]) != protocolString {
		return nil, errors.New("invalid protocol string")
	}
	// Extract infohash
	peerInfoHash := resp[28:48]
	if !bytes.Equal(peerInfoHash, expected_infoHash[:]) {
		return nil, errors.New("info hash mismatch")
	}

	// RETURN THE RESERVED BYTES (Indices 20-28)
	return resp[20:28], nil
}
