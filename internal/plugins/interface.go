package plugins

import (
	"context"
	"net/http"
)

// EventType represents the type of event that occurred
type EventType string

const (
	// Torrent lifecycle events
	EventTorrentAdded     EventType = "torrent_added"
	EventTorrentStarted   EventType = "torrent_started"
	EventTorrentProgress  EventType = "torrent_progress"
	EventTorrentCompleted EventType = "torrent_completed"
	EventTorrentPaused    EventType = "torrent_paused"
	EventTorrentResumed   EventType = "torrent_resumed"
	EventTorrentStopped   EventType = "torrent_stopped"
	EventTorrentRemoved   EventType = "torrent_removed"

	// Peer events
	EventPeerConnected    EventType = "peer_connected"
	EventPeerDisconnected EventType = "peer_disconnected"

	// System events
	EventSystemStartup    EventType = "system_startup"
	EventSystemShutdown   EventType = "system_shutdown"
)

// Event represents an event that occurred in the system
type Event struct {
	Type      EventType              `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// TorrentEvent contains torrent-specific event data
type TorrentEvent struct {
	InfoHash     string  `json:"info_hash"`
	Name         string  `json:"name"`
	Progress     float64 `json:"progress,omitempty"`
	State        string  `json:"state,omitempty"`
	Size         int64   `json:"size,omitempty"`
	DownloadPath string  `json:"download_path,omitempty"`
}

// PeerEvent contains peer-specific event data
type PeerEvent struct {
	InfoHash   string `json:"info_hash"`
	PeerIP     string `json:"peer_ip"`
	PeerPort   int    `json:"peer_port"`
	ClientName string `json:"client_name,omitempty"`
}

// Plugin interface that all plugins must implement
type Plugin interface {
	// Plugin metadata
	Name() string
	Version() string
	Description() string
	Author() string

	// Plugin lifecycle
	Initialize(ctx context.Context, config map[string]interface{}) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Cleanup() error

	// Event handling
	HandleEvent(event Event) error

	// Plugin capabilities
	GetCapabilities() []Capability

	// Health check
	IsHealthy() bool

	// Context injection
	SetContext(ctx PluginContext)
}

// Capability represents what a plugin can do
type Capability string

const (
	CapabilityEventHandler Capability = "event_handler"
	CapabilityAPIExtension Capability = "api_extension"
	CapabilityStreamServer Capability = "stream_server"
	CapabilitySearch       Capability = "search"
	CapabilityAnalytics    Capability = "analytics"
	CapabilityStorage      Capability = "storage"
	CapabilityNotification Capability = "notification"
	CapabilitySecurity     Capability = "security"
)

// APIExtension interface for plugins that want to extend the API
type APIExtension interface {
	Plugin
	RegisterRoutes(mux *http.ServeMux) error
	GetAPIPrefix() string
}

// StreamServer interface for plugins that provide streaming capabilities
type StreamServer interface {
	Plugin
	StartStream(infoHash string, filePath string) (string, error) // returns stream URL
	StopStream(infoHash string) error
	GetStreamURL(infoHash string, fileIndex int) (string, error)
	IsStreamingAvailable(infoHash string) bool
}

// SearchProvider interface for plugins that provide search capabilities
type SearchProvider interface {
	Plugin
	Search(query string, category string) ([]SearchResult, error)
	GetSupportedCategories() []string
	GetProviderName() string
}

// SearchResult represents a search result from a search provider
type SearchResult struct {
	Name        string `json:"name"`
	InfoHash    string `json:"info_hash"`
	MagnetLink  string `json:"magnet_link"`
	TorrentURL  string `json:"torrent_url,omitempty"` // Direct .torrent file download
	Size        int64  `json:"size"`
	Seeders     int    `json:"seeders"`
	Leechers    int    `json:"leechers"`
	Category    string `json:"category"`
	Provider    string `json:"provider"`
	UploadDate  string `json:"upload_date,omitempty"`
	Description string `json:"description,omitempty"`
}

// AnalyticsProvider interface for plugins that provide analytics capabilities
type AnalyticsProvider interface {
	Plugin
	GetMetrics(infoHash string) (map[string]interface{}, error)
	GetGlobalMetrics() (map[string]interface{}, error)
	GetRealtimeStats() (map[string]interface{}, error)
}

// NotificationProvider interface for plugins that handle notifications
type NotificationProvider interface {
	Plugin
	SendNotification(title, message, level string) error
	GetNotificationHistory() ([]Notification, error)
	RegisterNotificationTarget(target NotificationTarget) error
}

// NotificationTarget represents where notifications should be sent
type NotificationTarget struct {
	Type   string                 `json:"type"`   // email, slack, discord, etc.
	Config map[string]interface{} `json:"config"` // target-specific configuration
}

// Notification represents a notification
type Notification struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Level     string `json:"level"`
	Timestamp int64  `json:"timestamp"`
	Read      bool   `json:"read"`
}

// PluginManager interface for managing plugins
type PluginManager interface {
	// Plugin management
	RegisterPlugin(plugin Plugin) error
	UnregisterPlugin(name string) error
	GetPlugin(name string) (Plugin, error)
	GetAllPlugins() map[string]Plugin

	// Plugin lifecycle
	StartPlugin(name string) error
	StopPlugin(name string) error
	RestartPlugin(name string) error

	// Event system
	EmitEvent(event Event) error
	SubscribeToEvents(pluginName string, eventTypes []EventType) error

	// Plugin discovery
	DiscoverPlugins(directory string) error
	LoadPlugin(path string) (Plugin, error)

	// Configuration
	GetPluginConfig(name string) (map[string]interface{}, error)
	UpdatePluginConfig(name string, config map[string]interface{}) error

	// Plugin capabilities
	GetPluginsByCapability(capability Capability) []Plugin

	// Status and health
	GetPluginStatus(name string) (PluginStatus, error)
	GetAllPluginStatuses() map[string]PluginStatus
}

// PluginStatus represents the current status of a plugin
type PluginStatus struct {
	Name          string                 `json:"name"`
	Version       string                 `json:"version"`
	State         PluginState            `json:"state"`
	Healthy       bool                   `json:"healthy"`
	LastError     string                 `json:"last_error,omitempty"`
	StartTime     int64                  `json:"start_time,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
	Capabilities  []Capability           `json:"capabilities"`
}

// PluginState represents the current state of a plugin
type PluginState string

const (
	PluginStateUnregistered PluginState = "unregistered"
	PluginStateRegistered   PluginState = "registered"
	PluginStateStarting     PluginState = "starting"
	PluginStateRunning      PluginState = "running"
	PluginStateStopping     PluginState = "stopping"
	PluginStateStopped      PluginState = "stopped"
	PluginStateError        PluginState = "error"
)

// PluginContext provides access to system resources for plugins
type PluginContext interface {
	// API access
	GetTorrentState(infoHash string) (*TorrentState, error)
	GetAllTorrentStates() ([]TorrentState, error)

	// Torrent operations
	AddTorrent(source string) (string, error)
	RemoveTorrent(infoHash string) error
	PauseTorrent(infoHash string) error
	ResumeTorrent(infoHash string) error

	// File system access
	GetDownloadDirectory() string
	GetTorrentFiles(infoHash string) ([]string, error)

	// Logging
	LogInfo(pluginName, message string)
	LogWarn(pluginName, message string)
	LogError(pluginName, message string)

	// Configuration
	GetSystemConfig() map[string]interface{}
	GetPluginDataDirectory(pluginName string) string

	// HTTP client for external requests
	GetHTTPClient() *http.Client
}