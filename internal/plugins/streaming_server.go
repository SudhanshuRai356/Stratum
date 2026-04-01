package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StreamingServerPlugin provides HTTP streaming for torrents while they download
type StreamingServerPlugin struct {
	name        string
	version     string
	description string
	author      string

	// Plugin state
	running     bool
	healthy     bool
	mu          sync.RWMutex
	lastError   string

	// Configuration
	port       int
	bindAddr   string
	maxStreams int

	// HTTP server
	server *http.Server

	// Active streams
	activeStreams map[string]*StreamInfo
	streamsMu     sync.RWMutex

	// Plugin context
	context PluginContext
}

// StreamInfo contains information about an active stream
type StreamInfo struct {
	InfoHash     string    `json:"info_hash"`
	FilePath     string    `json:"file_path"`
	FileIndex    int       `json:"file_index"`
	StartTime    time.Time `json:"start_time"`
	LastAccessed time.Time `json:"last_accessed"`
	URL          string    `json:"url"`
	MimeType     string    `json:"mime_type"`
	FileSize     int64     `json:"file_size"`
}

// NewStreamingServerPlugin creates a new streaming server plugin
func NewStreamingServerPlugin() Plugin {
	return &StreamingServerPlugin{
		name:          "streaming-server",
		version:       "1.0.0",
		description:   "HTTP streaming server for torrents",
		author:        "Stratum Team",
		activeStreams: make(map[string]*StreamInfo),
	}
}

// Plugin interface implementation

func (p *StreamingServerPlugin) Name() string {
	return p.name
}

func (p *StreamingServerPlugin) Version() string {
	return p.version
}

func (p *StreamingServerPlugin) Description() string {
	return p.description
}

func (p *StreamingServerPlugin) Author() string {
	return p.author
}

func (p *StreamingServerPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Set default configuration
	p.port = 8081
	p.bindAddr = "127.0.0.1"
	p.maxStreams = 10

	// Parse configuration
	if portVal, ok := config["port"]; ok {
		if port, ok := portVal.(float64); ok {
			p.port = int(port)
		}
	}

	if addrVal, ok := config["bind_addr"]; ok {
		if addr, ok := addrVal.(string); ok {
			p.bindAddr = addr
		}
	}

	if maxVal, ok := config["max_streams"]; ok {
		if max, ok := maxVal.(float64); ok {
			p.maxStreams = int(max)
		}
	}

	p.healthy = true
	return nil
}

func (p *StreamingServerPlugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("streaming server is already running")
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", p.handleStream)
	mux.HandleFunc("/streams", p.handleStreams)
	mux.HandleFunc("/health", p.handleHealth)

	p.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", p.bindAddr, p.port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // No timeout for streaming
		IdleTimeout:  60 * time.Second,
	}

	// Start the server in a goroutine
	go func() {
		if p.context != nil {
			p.context.LogInfo(p.name, fmt.Sprintf("Starting streaming server on %s", p.server.Addr))
		}
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.mu.Lock()
			p.lastError = err.Error()
			p.healthy = false
			p.mu.Unlock()
			if p.context != nil {
				p.context.LogError(p.name, fmt.Sprintf("Server error: %v", err))
			}
		}
	}()

	// Start cleanup goroutine for inactive streams
	go p.cleanupInactiveStreams(ctx)

	p.running = true
	if p.context != nil {
		p.context.LogInfo(p.name, fmt.Sprintf("Streaming server started on port %d", p.port))
	}

	return nil
}

func (p *StreamingServerPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	if p.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := p.server.Shutdown(shutdownCtx); err != nil {
			if p.context != nil {
				p.context.LogError(p.name, fmt.Sprintf("Error shutting down server: %v", err))
			}
		}
	}

	p.running = false
	if p.context != nil {
		p.context.LogInfo(p.name, "Streaming server stopped")
	}

	return nil
}

func (p *StreamingServerPlugin) Cleanup() error {
	p.streamsMu.Lock()
	defer p.streamsMu.Unlock()

	p.activeStreams = make(map[string]*StreamInfo)
	return nil
}

func (p *StreamingServerPlugin) HandleEvent(event Event) error {
	switch event.Type {
	case EventTorrentCompleted:
		// When a torrent completes, we can optimize streaming
		p.context.LogInfo(p.name, "Torrent completed, optimizing streaming...")

	case EventTorrentStopped, EventTorrentRemoved:
		// Remove streams for stopped/removed torrents
		if torrentData, ok := event.Data["torrent"].(map[string]interface{}); ok {
			if infoHash, ok := torrentData["info_hash"].(string); ok {
				p.removeStreamsByInfoHash(infoHash)
			}
		}
	}

	return nil
}

func (p *StreamingServerPlugin) GetCapabilities() []Capability {
	return []Capability{CapabilityStreamServer, CapabilityEventHandler}
}

func (p *StreamingServerPlugin) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.healthy
}

// SetContext injects the plugin context
func (p *StreamingServerPlugin) SetContext(ctx PluginContext) {
	p.context = ctx
}

// StreamServer interface implementation

func (p *StreamingServerPlugin) StartStream(infoHash string, filePath string) (string, error) {
	p.streamsMu.Lock()
	defer p.streamsMu.Unlock()

	if len(p.activeStreams) >= p.maxStreams {
		return "", fmt.Errorf("maximum number of streams reached")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		return "", fmt.Errorf("file not found: %s", filePath)
	}

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	// Generate stream ID
	streamID := fmt.Sprintf("%s_%d", infoHash, time.Now().UnixNano())

	// Determine MIME type
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Create stream info
	streamInfo := &StreamInfo{
		InfoHash:     infoHash,
		FilePath:     filePath,
		StartTime:    time.Now(),
		LastAccessed: time.Now(),
		URL:          fmt.Sprintf("http://%s:%d/stream/%s", p.bindAddr, p.port, streamID),
		MimeType:     mimeType,
		FileSize:     fileInfo.Size(),
	}

	p.activeStreams[streamID] = streamInfo

	p.context.LogInfo(p.name, fmt.Sprintf("Started stream for %s: %s", infoHash, streamInfo.URL))

	return streamInfo.URL, nil
}

func (p *StreamingServerPlugin) StopStream(infoHash string) error {
	p.streamsMu.Lock()
	defer p.streamsMu.Unlock()

	for id, stream := range p.activeStreams {
		if stream.InfoHash == infoHash {
			delete(p.activeStreams, id)
			p.context.LogInfo(p.name, fmt.Sprintf("Stopped stream for %s", infoHash))
		}
	}

	return nil
}

func (p *StreamingServerPlugin) GetStreamURL(infoHash string, fileIndex int) (string, error) {
	p.streamsMu.RLock()
	defer p.streamsMu.RUnlock()

	for _, stream := range p.activeStreams {
		if stream.InfoHash == infoHash && stream.FileIndex == fileIndex {
			return stream.URL, nil
		}
	}

	return "", fmt.Errorf("no stream found for torrent %s file %d", infoHash, fileIndex)
}

func (p *StreamingServerPlugin) IsStreamingAvailable(infoHash string) bool {
	// For now, assume streaming is available if we can get the torrent files
	files, err := p.context.GetTorrentFiles(infoHash)
	return err == nil && len(files) > 0
}

// HTTP handlers

func (p *StreamingServerPlugin) handleStream(w http.ResponseWriter, r *http.Request) {
	// Extract stream ID from URL
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid stream ID", http.StatusBadRequest)
		return
	}

	streamID := parts[2]

	p.streamsMu.RLock()
	stream, exists := p.activeStreams[streamID]
	if !exists {
		p.streamsMu.RUnlock()
		http.Error(w, "Stream not found", http.StatusNotFound)
		return
	}

	// Update last accessed time
	stream.LastAccessed = time.Now()
	p.streamsMu.RUnlock()

	// Open the file
	file, err := os.Open(stream.FilePath)
	if err != nil {
		p.context.LogError(p.name, fmt.Sprintf("Failed to open file %s: %v", stream.FilePath, err))
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, "Failed to get file info", http.StatusInternalServerError)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", stream.MimeType)
	w.Header().Set("Accept-Ranges", "bytes")

	// Handle range requests for seeking support
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		p.handleRangeRequest(w, r, file, fileInfo.Size(), rangeHeader)
		return
	}

	// Set content length for full file
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	// Stream the file
	w.WriteHeader(http.StatusOK)
	io.Copy(w, file)
}

func (p *StreamingServerPlugin) handleRangeRequest(w http.ResponseWriter, r *http.Request, file *os.File, fileSize int64, rangeHeader string) {
	// Parse range header (simplified implementation)
	ranges := strings.TrimPrefix(rangeHeader, "bytes=")
	rangeParts := strings.Split(ranges, "-")

	var start, end int64
	var err error

	if rangeParts[0] == "" {
		// Suffix range: -500 (last 500 bytes)
		if len(rangeParts) > 1 {
			suffix, err := strconv.ParseInt(rangeParts[1], 10, 64)
			if err == nil {
				start = fileSize - suffix
				end = fileSize - 1
			}
		}
	} else {
		// Standard range: 200-1023 or 200-
		start, err = strconv.ParseInt(rangeParts[0], 10, 64)
		if err != nil {
			http.Error(w, "Invalid range", http.StatusBadRequest)
			return
		}

		if len(rangeParts) > 1 && rangeParts[1] != "" {
			end, err = strconv.ParseInt(rangeParts[1], 10, 64)
			if err != nil {
				http.Error(w, "Invalid range", http.StatusBadRequest)
				return
			}
		} else {
			end = fileSize - 1
		}
	}

	// Validate range
	if start < 0 || start >= fileSize || end >= fileSize || start > end {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
		http.Error(w, "Range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Seek to start position
	file.Seek(start, 0)

	// Set headers for partial content
	contentLength := end - start + 1
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.WriteHeader(http.StatusPartialContent)

	// Stream the requested range
	io.CopyN(w, file, contentLength)
}

func (p *StreamingServerPlugin) handleStreams(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	p.streamsMu.RLock()
	streams := make([]*StreamInfo, 0, len(p.activeStreams))
	for _, stream := range p.activeStreams {
		streams = append(streams, stream)
	}
	p.streamsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_streams": streams,
		"max_streams":    p.maxStreams,
	})
}

func (p *StreamingServerPlugin) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	p.mu.RLock()
	status := map[string]interface{}{
		"healthy":        p.healthy,
		"running":        p.running,
		"last_error":     p.lastError,
		"active_streams": len(p.activeStreams),
		"port":           p.port,
	}
	p.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Helper methods

func (p *StreamingServerPlugin) removeStreamsByInfoHash(infoHash string) {
	p.streamsMu.Lock()
	defer p.streamsMu.Unlock()

	for id, stream := range p.activeStreams {
		if stream.InfoHash == infoHash {
			delete(p.activeStreams, id)
		}
	}
}

func (p *StreamingServerPlugin) cleanupInactiveStreams(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.streamsMu.Lock()
			cutoff := time.Now().Add(-30 * time.Minute) // Remove streams inactive for 30 minutes

			for id, stream := range p.activeStreams {
				if stream.LastAccessed.Before(cutoff) {
					delete(p.activeStreams, id)
					p.context.LogInfo(p.name, fmt.Sprintf("Cleaned up inactive stream: %s", id))
				}
			}
			p.streamsMu.Unlock()
		}
	}
}