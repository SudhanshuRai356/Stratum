package peer

import (
"encoding/binary"
"fmt"
"net"
)

const DefaultPipelineDepth = 5

type Downloader struct {
	PieceIndex    uint32           // Current piece being downloaded
	PieceLength   uint32           // Length of the piece
	PieceHashes   []byte           // All piece hashes (20 bytes each)
	Assembler     *PieceAssembler  // Assembles blocks into complete piece
	NextOffset    uint32           // Next block offset to request
	Inflight      uint32           // Number of requests in flight
	PipelineDepth int              // Max concurrent requests
}

func NewDownloader(pieceIndex, pieceLength uint32, pieceHashes []byte, pipelineDepth int) *Downloader {
	if pipelineDepth <= 0 {
		pipelineDepth = DefaultPipelineDepth
	}

	return &Downloader{
		PieceIndex:    pieceIndex,
		PieceLength:   pieceLength,
		PieceHashes:   pieceHashes,
		Assembler:     NewPieceAssembler(pieceIndex, pieceLength, Blocksize),
		PipelineDepth: pipelineDepth,
	}
}

func (d *Downloader) StartPipeline(conn net.Conn) {
	for i := 0; i < d.PipelineDepth && d.NextOffset < d.Assembler.Length; i++ {
		blockSize := uint32(Blocksize)
		if d.NextOffset+blockSize > d.Assembler.Length {
			blockSize = d.Assembler.Length - d.NextOffset
		}

		err := SendReq(conn, d.PieceIndex, d.NextOffset, blockSize)
		if err != nil {
			fmt.Println("Request failed:", err)
			continue
		}
		d.NextOffset += blockSize
		d.Inflight++
	}
}

// HandlePiece processes an incoming PIECE message.
func (d *Downloader) HandlePiece(conn net.Conn, payload []byte) (bool, error) {
	if len(payload) < 8 {
		return false, fmt.Errorf("piece payload too short")
	}

	pieceIdx := binary.BigEndian.Uint32(payload[0:4])
	begin := binary.BigEndian.Uint32(payload[4:8])
	block := payload[8:]

	fmt.Println(
"Received PIECE",
"piece:", pieceIdx,
"begin:", begin,
"block length:", len(block),
)

	// Verify this is the piece we're downloading
if pieceIdx != d.PieceIndex {
return false, fmt.Errorf("unexpected piece index: got %d, want %d", pieceIdx, d.PieceIndex)
}

if err := d.Assembler.AddBlock(begin, block); err != nil {
return false, fmt.Errorf("AddBlock failed: %w", err)
}

d.Inflight--

if d.NextOffset < d.Assembler.Length {
blockSize := uint32(Blocksize)
if d.NextOffset+blockSize > d.Assembler.Length {
blockSize = d.Assembler.Length - d.NextOffset
}

if err := SendReq(conn, d.PieceIndex, d.NextOffset, blockSize); err != nil {
fmt.Println("Request failed:", err)
} else {
d.NextOffset += blockSize
d.Inflight++
}
}

// Check if piece is complete
if d.Assembler.Received == d.Assembler.Length {
fmt.Println("Piece fully downloaded")

expectedHash := d.getExpectedHash()
if d.Assembler.Verify(expectedHash) {
fmt.Println("Piece hash VERIFIED")
return true, nil
} else {
return false, fmt.Errorf("piece hash MISMATCH")
}
}

return false, nil
}

func (d *Downloader) getExpectedHash() []byte {
start := d.PieceIndex * 20
end := start + 20
if int(end) > len(d.PieceHashes) {
return nil
}
return d.PieceHashes[start:end]
}

func (d *Downloader) GetData() []byte {
return d.Assembler.Data
}

func (d *Downloader) IsComplete() bool {
return d.Assembler.Received == d.Assembler.Length
}
