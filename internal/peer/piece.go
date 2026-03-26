package peer

import (
	"crypto/sha1"
	"fmt"
)

type PieceAssembler struct {
	Index     uint32
	Length    uint32
	BlockSize uint32
	Data      []byte
	Received  uint32
	Verified  bool
}

func NewPieceAssembler(index, length, blocksize uint32) *PieceAssembler {
	return &PieceAssembler{
		Index:     index,
		Length:    length,
		BlockSize: blocksize,
		Data:      make([]byte, length),
	}
}

func (p *PieceAssembler) AddBlock(begin uint32, block []byte) error {
	if begin+uint32(len(block)) > p.Length {
		return fmt.Errorf("block out of bounds")
	}
	copy(p.Data[begin:], block)
	p.Received += uint32(len(block))
	return nil
}

func (p *PieceAssembler) Verify(expectedHash []byte) bool {
	hash := sha1.Sum(p.Data)
	return string(hash[:]) == string(expectedHash)
}
