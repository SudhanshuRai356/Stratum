package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kushal-39/Stratum/internal/plugins"
)

// PluginAPIServer provides HTTP endpoints for plugin management
type PluginAPIServer struct {
	pluginManager plugins.PluginManager
}

// NewPluginAPIServer creates a new plugin API server
func NewPluginAPIServer(pluginManager plugins.PluginManager) *PluginAPIServer {
	return &PluginAPIServer{
		pluginManager: pluginManager,
	}
}

// RegisterPluginRoutes registers plugin management routes with the HTTP server
func (pas *PluginAPIServer) RegisterPluginRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/plugins", pas.handlePlugins)
	mux.HandleFunc("/api/plugins/", pas.handlePluginAction)
	mux.HandleFunc("/api/plugins/search", pas.handleSearch)
	mux.HandleFunc("/api/plugins/stream", pas.handleStream)
	mux.HandleFunc("/api/download-torrent", pas.handleDownloadTorrent)
}

// handlePlugins handles GET /api/plugins - list all plugins and their status
func (pas *PluginAPIServer) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	statuses := pas.pluginManager.GetAllPluginStatuses()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"plugins": statuses,
	})
}

// handlePluginAction handles plugin-specific actions
func (pas *PluginAPIServer) handlePluginAction(w http.ResponseWriter, r *http.Request) {
	// Parse URL: /api/plugins/{name}/{action}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/plugins/"), "/")
	if len(pathParts) < 2 {
		http.Error(w, "Invalid plugin action URL", http.StatusBadRequest)
		return
	}

	pluginName := pathParts[0]
	action := pathParts[1]

	switch action {
	case "status":
		pas.handlePluginStatus(w, r, pluginName)
	case "start":
		pas.handlePluginStart(w, r, pluginName)
	case "stop":
		pas.handlePluginStop(w, r, pluginName)
	case "restart":
		pas.handlePluginRestart(w, r, pluginName)
	case "config":
		pas.handlePluginConfig(w, r, pluginName)
	case "results":
		pas.handlePluginResults(w, r, pluginName)
	default:
		http.Error(w, "Unknown plugin action", http.StatusBadRequest)
	}
}

// handlePluginStatus handles GET /api/plugins/{name}/status
func (pas *PluginAPIServer) handlePluginStatus(w http.ResponseWriter, r *http.Request, pluginName string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status, err := pas.pluginManager.GetPluginStatus(pluginName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"plugin": status,
	})
}

// handlePluginStart handles POST /api/plugins/{name}/start
func (pas *PluginAPIServer) handlePluginStart(w http.ResponseWriter, r *http.Request, pluginName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := pas.pluginManager.StartPlugin(pluginName); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Plugin %s started successfully", pluginName),
	})
}

// handlePluginStop handles POST /api/plugins/{name}/stop
func (pas *PluginAPIServer) handlePluginStop(w http.ResponseWriter, r *http.Request, pluginName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := pas.pluginManager.StopPlugin(pluginName); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Plugin %s stopped successfully", pluginName),
	})
}

// handlePluginRestart handles POST /api/plugins/{name}/restart
func (pas *PluginAPIServer) handlePluginRestart(w http.ResponseWriter, r *http.Request, pluginName string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := pas.pluginManager.RestartPlugin(pluginName); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Plugin %s restarted successfully", pluginName),
	})
}

// handlePluginResults handles GET /api/plugins/{name}/results
func (pas *PluginAPIServer) handlePluginResults(w http.ResponseWriter, r *http.Request, pluginName string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	plugin, err := pas.pluginManager.GetPlugin(pluginName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Type-assert to VirusTotalPlugin to get results
	type resultProvider interface {
		GetAllScanResults() []plugins.ScanResult
	}

	if provider, ok := plugin.(resultProvider); ok {
		results := provider.GetAllScanResults()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"results": results,
			"count":   len(results),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": []plugins.ScanResult{},
		"count":   0,
		"message": "Plugin does not provide scan results",
	})
}

// handlePluginConfig handles GET/PUT /api/plugins/{name}/config
func (pas *PluginAPIServer) handlePluginConfig(w http.ResponseWriter, r *http.Request, pluginName string) {
	switch r.Method {
	case http.MethodGet:
		config, err := pas.pluginManager.GetPluginConfig(pluginName)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"config": config,
		})

	case http.MethodPut:
		defer r.Body.Close()
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		if err := pas.pluginManager.UpdatePluginConfig(pluginName, payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": fmt.Sprintf("Configuration updated for plugin %s", pluginName),
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSearch handles POST /api/plugins/search
func (pas *PluginAPIServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	var payload struct {
		Query    string `json:"query"`
		Category string `json:"category"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(payload.Query) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "Query cannot be empty",
		})
		return
	}

	// Get search plugins
	searchPlugins := pas.pluginManager.GetPluginsByCapability(plugins.CapabilitySearch)
	if len(searchPlugins) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "No search plugins available",
		})
		return
	}

	// Use the first available search plugin
	searchPlugin := searchPlugins[0]
	if searchProvider, ok := searchPlugin.(plugins.SearchProvider); ok {
		results, err := searchProvider.Search(payload.Query, payload.Category)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"results":  results,
			"query":    payload.Query,
			"category": payload.Category,
		})
		return
	}

	writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
		"error": "Search plugin interface not supported",
	})
}

// handleStream handles streaming operations
func (pas *PluginAPIServer) handleStream(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		pas.handleStartStream(w, r)
	case http.MethodDelete:
		pas.handleStopStream(w, r)
	case http.MethodGet:
		pas.handleGetStream(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStartStream handles POST /api/plugins/stream - start a new stream
func (pas *PluginAPIServer) handleStartStream(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var payload struct {
		InfoHash string `json:"info_hash"`
		FilePath string `json:"file_path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Get streaming plugins
	streamPlugins := pas.pluginManager.GetPluginsByCapability(plugins.CapabilityStreamServer)
	if len(streamPlugins) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "No streaming plugins available",
		})
		return
	}

	// Use the first available streaming plugin
	streamPlugin := streamPlugins[0]
	if streamServer, ok := streamPlugin.(plugins.StreamServer); ok {
		streamURL, err := streamServer.StartStream(payload.InfoHash, payload.FilePath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"stream_url": streamURL,
			"info_hash":  payload.InfoHash,
			"file_path":  payload.FilePath,
		})
		return
	}

	writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
		"error": "Streaming plugin interface not supported",
	})
}

// handleStopStream handles DELETE /api/plugins/stream?info_hash=xxx
func (pas *PluginAPIServer) handleStopStream(w http.ResponseWriter, r *http.Request) {
	infoHash := r.URL.Query().Get("info_hash")
	if infoHash == "" {
		http.Error(w, "info_hash parameter required", http.StatusBadRequest)
		return
	}

	// Get streaming plugins
	streamPlugins := pas.pluginManager.GetPluginsByCapability(plugins.CapabilityStreamServer)
	if len(streamPlugins) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "No streaming plugins available",
		})
		return
	}

	// Stop stream on all streaming plugins
	for _, streamPlugin := range streamPlugins {
		if streamServer, ok := streamPlugin.(plugins.StreamServer); ok {
			if err := streamServer.StopStream(infoHash); err != nil {
				// Log error but continue with other plugins
				continue
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Stream stopped for %s", infoHash),
	})
}

// handleGetStream handles GET /api/plugins/stream?info_hash=xxx&file_index=0
func (pas *PluginAPIServer) handleGetStream(w http.ResponseWriter, r *http.Request) {
	infoHash := r.URL.Query().Get("info_hash")
	if infoHash == "" {
		http.Error(w, "info_hash parameter required", http.StatusBadRequest)
		return
	}

	fileIndexStr := r.URL.Query().Get("file_index")
	fileIndex := 0
	if fileIndexStr != "" {
		if index, err := strconv.Atoi(fileIndexStr); err == nil {
			fileIndex = index
		}
	}

	// Get streaming plugins
	streamPlugins := pas.pluginManager.GetPluginsByCapability(plugins.CapabilityStreamServer)
	if len(streamPlugins) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "No streaming plugins available",
		})
		return
	}

	// Try to get stream URL from streaming plugins
	for _, streamPlugin := range streamPlugins {
		if streamServer, ok := streamPlugin.(plugins.StreamServer); ok {
			streamURL, err := streamServer.GetStreamURL(infoHash, fileIndex)
			if err == nil {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"stream_url": streamURL,
					"info_hash":  infoHash,
					"file_index": fileIndex,
					"available":  streamServer.IsStreamingAvailable(infoHash),
				})
				return
			}
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]interface{}{
		"error":      "Stream not found",
		"info_hash":  infoHash,
		"file_index": fileIndex,
	})
}

// handleDownloadTorrent handles GET /api/download-torrent?url=...
// This proxies .torrent file downloads from torrent sites
func (pas *PluginAPIServer) handleDownloadTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	torrentURL := r.URL.Query().Get("url")
	if torrentURL == "" {
		http.Error(w, "url parameter required", http.StatusBadRequest)
		return
	}

	// Fetch the .torrent file from the provided URL
	client := &http.Client{}
	req, err := http.NewRequest("GET", torrentURL, nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Set a realistic user agent
	req.Header.Set("User-Agent", "Stratum/1.0")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to fetch torrent file", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Failed to download torrent: HTTP %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	// Set appropriate headers for download
	w.Header().Set("Content-Type", "application/x-bittorrent")
	w.Header().Set("Content-Disposition", "attachment; filename=download.torrent")

	// Copy the file content to response
	if _, err := io.Copy(w, resp.Body); err != nil {
		// Error already started writing, can't send error response
		// Just log it if we had a context
		return
	}
}
