package plugins

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DefaultPluginContext provides access to system resources for plugins
type DefaultPluginContext struct {
	downloadDirectory string
	dataDirectory    string
	systemConfig     map[string]interface{}
	httpClient       *http.Client

	// Function pointers to system operations
	getTorrentStateFn    func(string) (*TorrentState, error)
	getAllTorrentStatsFn func() ([]TorrentState, error)
	addTorrentFn         func(string) (string, error)
	removeTorrentFn      func(string) error
	pauseTorrentFn       func(string) error
	resumeTorrentFn      func(string) error
	getTorrentFilesFn    func(string) ([]string, error)
}

// NewPluginContext creates a new plugin context
func NewPluginContext(downloadDir, dataDir string) *DefaultPluginContext {
	return &DefaultPluginContext{
		downloadDirectory: downloadDir,
		dataDirectory:     dataDir,
		systemConfig:      make(map[string]interface{}),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetSystemOperations sets the function pointers for system operations
func (ctx *DefaultPluginContext) SetSystemOperations(
	getTorrentState func(string) (*TorrentState, error),
	getAllTorrentStates func() ([]TorrentState, error),
	addTorrent func(string) (string, error),
	removeTorrent func(string) error,
	pauseTorrent func(string) error,
	resumeTorrent func(string) error,
	getTorrentFiles func(string) ([]string, error),
) {
	ctx.getTorrentStateFn = getTorrentState
	ctx.getAllTorrentStatsFn = getAllTorrentStates
	ctx.addTorrentFn = addTorrent
	ctx.removeTorrentFn = removeTorrent
	ctx.pauseTorrentFn = pauseTorrent
	ctx.resumeTorrentFn = resumeTorrent
	ctx.getTorrentFilesFn = getTorrentFiles
}

// GetTorrentState returns the state of a specific torrent
func (ctx *DefaultPluginContext) GetTorrentState(infoHash string) (*TorrentState, error) {
	if ctx.getTorrentStateFn == nil {
		return nil, fmt.Errorf("torrent state function not available")
	}
	return ctx.getTorrentStateFn(infoHash)
}

// GetAllTorrentStates returns the state of all torrents
func (ctx *DefaultPluginContext) GetAllTorrentStates() ([]TorrentState, error) {
	if ctx.getAllTorrentStatsFn == nil {
		return nil, fmt.Errorf("all torrent states function not available")
	}
	return ctx.getAllTorrentStatsFn()
}

// AddTorrent adds a new torrent
func (ctx *DefaultPluginContext) AddTorrent(source string) (string, error) {
	if ctx.addTorrentFn == nil {
		return "", fmt.Errorf("add torrent function not available")
	}
	return ctx.addTorrentFn(source)
}

// RemoveTorrent removes a torrent
func (ctx *DefaultPluginContext) RemoveTorrent(infoHash string) error {
	if ctx.removeTorrentFn == nil {
		return fmt.Errorf("remove torrent function not available")
	}
	return ctx.removeTorrentFn(infoHash)
}

// PauseTorrent pauses a torrent
func (ctx *DefaultPluginContext) PauseTorrent(infoHash string) error {
	if ctx.pauseTorrentFn == nil {
		return fmt.Errorf("pause torrent function not available")
	}
	return ctx.pauseTorrentFn(infoHash)
}

// ResumeTorrent resumes a torrent
func (ctx *DefaultPluginContext) ResumeTorrent(infoHash string) error {
	if ctx.resumeTorrentFn == nil {
		return fmt.Errorf("resume torrent function not available")
	}
	return ctx.resumeTorrentFn(infoHash)
}

// GetDownloadDirectory returns the download directory
func (ctx *DefaultPluginContext) GetDownloadDirectory() string {
	return ctx.downloadDirectory
}

// GetTorrentFiles returns the files for a specific torrent
func (ctx *DefaultPluginContext) GetTorrentFiles(infoHash string) ([]string, error) {
	if ctx.getTorrentFilesFn == nil {
		return nil, fmt.Errorf("get torrent files function not available")
	}
	return ctx.getTorrentFilesFn(infoHash)
}

// LogInfo logs an info message
func (ctx *DefaultPluginContext) LogInfo(pluginName, message string) {
	log.Printf("[PLUGIN:%s] [INFO] %s", pluginName, message)
}

// LogWarn logs a warning message
func (ctx *DefaultPluginContext) LogWarn(pluginName, message string) {
	log.Printf("[PLUGIN:%s] [WARN] %s", pluginName, message)
}

// LogError logs an error message
func (ctx *DefaultPluginContext) LogError(pluginName, message string) {
	log.Printf("[PLUGIN:%s] [ERROR] %s", pluginName, message)
}

// GetSystemConfig returns the system configuration
func (ctx *DefaultPluginContext) GetSystemConfig() map[string]interface{} {
	return ctx.systemConfig
}

// SetSystemConfig sets the system configuration
func (ctx *DefaultPluginContext) SetSystemConfig(config map[string]interface{}) {
	ctx.systemConfig = config
}

// GetPluginDataDirectory returns the data directory for a specific plugin
func (ctx *DefaultPluginContext) GetPluginDataDirectory(pluginName string) string {
	dir := filepath.Join(ctx.dataDirectory, "plugins", pluginName)
	os.MkdirAll(dir, 0755)
	return dir
}

// GetHTTPClient returns an HTTP client for external requests
func (ctx *DefaultPluginContext) GetHTTPClient() *http.Client {
	return ctx.httpClient
}

// GetDataDirectory returns the main data directory
func (ctx *DefaultPluginContext) GetDataDirectory() string {
	return ctx.dataDirectory
}