package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

type FileInfo struct {
	Path   []string
	Length int64
}

type FileEntry struct {
	Path         string
	Length       int64
	GlobalOffset int64
	File         *os.File
}

type Writer struct {
	path     string // Base path for the .state file
	files    []FileEntry
	totalLen int64
}

func New(basePath string, files []FileInfo) (*Writer, error) {
	w := &Writer{
		path:  basePath,
		files: make([]FileEntry, 0, len(files)),
	}

	var currentGlobalOffset int64 = 0

	for _, fInfo := range files {
		fullPath := basePath
		for _, p := range fInfo.Path {
			fullPath = filepath.Join(fullPath, p)
		}

		dir := filepath.Dir(fullPath)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directories for %s: %w", fullPath, err)
			}
		}

		file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %w", fullPath, err)
		}

		// Truncate/allocate space
		if err := file.Truncate(fInfo.Length); err != nil {
			return nil, fmt.Errorf("failed to truncate %s: %w", fullPath, err)
		}

		w.files = append(w.files, FileEntry{
			Path:         fullPath,
			Length:       fInfo.Length,
			GlobalOffset: currentGlobalOffset,
			File:         file,
		})

		currentGlobalOffset += fInfo.Length
	}

	w.totalLen = currentGlobalOffset
	return w, nil
}

// WriteAt maps a global offset to local files and writes the chunk
func (w *Writer) WriteAt(globalOffset int64, data []byte) error {
	dataOffset := 0
	dataLen := len(data)

	for _, f := range w.files {
		fileStart := f.GlobalOffset
		fileEnd := f.GlobalOffset + f.Length

		// Does any part of the data belong in this file?
		if int64(globalOffset) < fileEnd && int64(globalOffset)+int64(dataLen) > fileStart {

			// Calculate the overlap boundaries
			writeStart := maxInt64(int64(globalOffset)+int64(dataOffset), fileStart)
			writeEnd := minInt64(int64(globalOffset)+int64(dataLen), fileEnd)

			localOffset := writeStart - fileStart

			// Slice the data chunk for this specific file
			chunkStart := dataOffset + int(writeStart-(int64(globalOffset)+int64(dataOffset)))
			chunkEnd := chunkStart + int(writeEnd-writeStart)
			chunk := data[chunkStart:chunkEnd]

			_, err := f.File.WriteAt(chunk, localOffset)
			if err != nil {
				return err
			}

			dataOffset += len(chunk)
			if dataOffset == dataLen {
				break
			}
		}
	}

	if dataOffset < dataLen {
		return fmt.Errorf("WriteAt: data exceeds total torrent boundaries")
	}
	return nil
}

// ReadAt maps a global offset to local files and reads the chunk
func (w *Writer) ReadAt(b []byte, globalOffset int64) (int, error) {
	bytesRead := 0
	dataLen := len(b)

	for _, f := range w.files {
		fileStart := f.GlobalOffset
		fileEnd := f.GlobalOffset + f.Length
		// Does the requested read overlap with this file?
		if int64(globalOffset) < fileEnd && int64(globalOffset)+int64(dataLen) > fileStart {

			readStart := maxInt64(int64(globalOffset)+int64(bytesRead), fileStart)
			readEnd := minInt64(int64(globalOffset)+int64(dataLen), fileEnd)

			localOffset := readStart - fileStart

			chunkStart := bytesRead + int(readStart-(int64(globalOffset)+int64(bytesRead)))
			chunkEnd := chunkStart + int(readEnd-readStart)
			chunk := b[chunkStart:chunkEnd]

			n, err := f.File.ReadAt(chunk, localOffset)
			bytesRead += n
			if err != nil {
				return bytesRead, err
			}

			if bytesRead == dataLen {
				break
			}
		}
	}

	return bytesRead, nil
}

// Close gracefully closes all open file handles
func (w *Writer) Close() error {
	var lastErr error
	for _, f := range w.files {
		if err := f.File.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Math helpers
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
