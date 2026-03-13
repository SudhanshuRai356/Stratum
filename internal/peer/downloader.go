package peer

import (
	"encoding/binary"
	"fmt"

	"github.com/Kushal-39/Stratum/internal/storage"
	"github.com/Kushal-39/Stratum/internal/torrent"
)

const DefaultPipelineDepth = 5

type Downloader struct {
	PieceIndex    uint32
	Torrent       *torrent.Torrent
	Assembler     *PieceAssembler
	NextOffset    uint32
	Inflight      uint32
	PipelineDepth int
	Writer        *storage.Writer
}

func NewDownloader(
	t *torrent.Torrent,
	pieceIndex uint32,
	pipelineDepth int,
	w *storage.Writer,
) *Downloader {

	if pipelineDepth <= 0 {
		pipelineDepth = DefaultPipelineDepth
	}

	// FIX: Calculate exact length for the final piece
	pieceLength := t.Info.PieceLength
	totalPieces := len(t.Info.Pieces) / 20

	if totalPieces <= 0 || t.Info.PieceLength <= 0 || t.Info.Length <= 0 {
		return nil // metadata invariant violation - caller must handle nil
	}

	if int(pieceIndex) == totalPieces-1 {
		// It's the last piece, calculate the remainder
		pieceLength = int(t.Info.Length - int64(pieceIndex)*int64(t.Info.PieceLength))
	}

	if pieceLength <= 0 || pieceLength > t.Info.PieceLength {
		return nil // underflow or overflow detected
	}

	// Bounds-check the hash slice before we ever use it
	hashStart := int(pieceIndex) * 20
	hashEnd := hashStart + 20
	if hashEnd > len(t.Info.Pieces) {
		return nil
	}

	return &Downloader{
		PieceIndex:    pieceIndex,
		Torrent:       t,
		Assembler:     NewPieceAssembler(pieceIndex, uint32(pieceLength), Blocksize),
		PipelineDepth: pipelineDepth,
		Writer:        w,
	}
}

// StartPipeline sends the initial burst of requests
func (d *Downloader) StartPipeline(
	request func(offset, length uint32) error,
) error {

	for i := 0; i < d.PipelineDepth && d.NextOffset < d.Assembler.Length; i++ {
		if err := d.sendNext(request); err != nil {
			return err
		}
	}
	return nil
}

// HandlePiece processes a PIECE message payload
func (d *Downloader) HandlePiece(
	payload []byte,
	request func(offset, length uint32) error,
) (bool, error) {

	if len(payload) < 8 {
		return false, fmt.Errorf("piece payload too short")
	}

	piece := binary.BigEndian.Uint32(payload[0:4])
	begin := binary.BigEndian.Uint32(payload[4:8])
	block := payload[8:]

	if piece != d.PieceIndex {
		return false, fmt.Errorf("unexpected piece index %d", piece)
	}

	if err := d.Assembler.AddBlock(begin, block); err != nil {
		return false, err
	}

	d.Inflight--

	// keep pipeline full
	if d.NextOffset < d.Assembler.Length {
		if err := d.sendNext(request); err != nil {
			return false, err
		}
	}

	if d.Assembler.Received == d.Assembler.Length {
		fmt.Println("Piece fully downloaded")

		hashStart := int(d.PieceIndex) * 20
		hashEnd := hashStart + 20
		if hashEnd > len(d.Torrent.Info.Pieces) {
			return false, fmt.Errorf("piece index %d hash out of bounds", d.PieceIndex)
		}
		expected := []byte(d.Torrent.Info.Pieces[hashStart:hashEnd])

		if d.Assembler.Verify(expected) {
			offset := int64(d.PieceIndex) * int64(d.Torrent.Info.PieceLength)

			if err := d.Writer.WriteAt(offset, d.Assembler.Data); err != nil {
				return false, err
			}

			fmt.Println("Piece written to disk")
			return true, nil
		}

		return false, fmt.Errorf("piece hash mismatch")
	}

	return false, nil
}

// sendNext issues the next block request
func (d *Downloader) sendNext(
	request func(offset, length uint32) error,
) error {

	blockSize := uint32(Blocksize)
	if d.NextOffset+blockSize > d.Assembler.Length {
		blockSize = d.Assembler.Length - d.NextOffset
	}

	if err := request(d.NextOffset, blockSize); err != nil {
		return err
	}

	d.NextOffset += blockSize
	d.Inflight++
	return nil
}
