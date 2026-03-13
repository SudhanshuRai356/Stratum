package peer

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	MsgChoke         = 0
	MsgUnchoke       = 1
	MsgInterested    = 2
	MsgNotInterested = 3
	MsgHave          = 4
	MsgBitfield      = 5
	MsgRequest       = 6
	MsgPiece         = 7
	MsgCancel        = 8
	// BEP 10 Extension Protocol
	MsgExtended = 20

	// MaxMessageLen caps the allocation from a peer-supplied length prefix.
	// Largest legitimate message: PIECE = 4(len) + 1(id) + 4(index) + 4(begin) + 16384(block).
	// 2 MiB leaves headroom for extension messages and large bitfields.
	MaxMessageLen = 2 * 1024 * 1024
)

// ExtendedHandshakePayload is the bencoded dictionary for the handshake
type ExtendedHandshakePayload struct {
	M map[string]int `bencode:"m"` // Supported extensions
}

// Metadata message types for BEP 09
const (
	MetadataRequest = 0
	MetadataData    = 1
	MetadataReject  = 2
)

// MetadataMessage represents a ut_metadata message
type MetadataMessage struct {
	MsgType int `bencode:"msg_type"`
	Piece   int `bencode:"piece"`
	// TotalSize only present in data messages
	TotalSize int `bencode:"total_size,omitempty"`
}

type Message struct {
	ID      byte
	Payload []byte
}

func ReadMessage(r io.Reader) (*Message, error) { //reads a single bittorent message from the peer
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	//keep-alive
	if length == 0 {
		return &Message{ID: 255}, nil
	}

	if length > MaxMessageLen {
		return nil, fmt.Errorf("peer message length %d exceeds maximum %d", length, MaxMessageLen)
	}

	msgBuf := make([]byte, length)
	if _, err := io.ReadFull(r, msgBuf); err != nil {
		return nil, err
	}
	return &Message{
		ID:      msgBuf[0],
		Payload: msgBuf[1:],
	}, nil

}
