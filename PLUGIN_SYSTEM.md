# Stratum Plugin System Documentation

## Overview

The Stratum plugin system provides a powerful extensibility mechanism for adding custom functionality to the BitTorrent client without modifying core code. The system supports event-driven architecture, API extensions, and specialized plugin types for streaming, search, analytics, and more.

## Architecture

### Plugin Manager

The `PluginManager` is the central component that manages the lifecycle of all plugins:

- **Registration**: Plugins must be registered with the manager before they can be used
- **Lifecycle Management**: Start, stop, restart plugins
- **Event Distribution**: Route system events to interested plugins
- **Configuration**: Store and manage plugin-specific configurations
- **Status Monitoring**: Track plugin health and state

### Plugin Context

The `PluginContext` provides plugins with access to system resources:

- **Torrent State**: Query current torrent information
- **Torrent Operations**: Add, remove, pause, resume torrents
- **File System**: Access download directory and torrent files
- **HTTP Client**: Make external HTTP requests
- **Logging**: Log messages for debugging

## Plugin Types

### 1. Streaming Server Plugin

Provides HTTP streaming for media files while they're being downloaded.

**Capabilities**:
- `stream_server` - HTTP streaming service
- `event_handler` - Event processing

**API Endpoints**:
- `POST /api/plugins/stream` - Start a stream
- `DELETE /api/plugins/stream?info_hash=xxx` - Stop a stream
- `GET /api/plugins/stream?info_hash=xxx&file_index=0` - Get stream URL

**Features**:
- HTTP range request support for seeking
- MIME type detection
- Automatic inactive stream cleanup
- Concurrent stream limiting

**Configuration**:
```json
{
  "port": 8081,
  "bind_addr": "127.0.0.1",
  "max_streams": 10
}
```

### 2. Search Plugin

Multi-provider torrent search engine with extensible architecture.

**Capabilities**:
- `search` - Search functionality

**API Endpoints**:
- `POST /api/plugins/search` - Search for torrents

**Features**:
- Multiple search providers (The Pirate Bay, Nyaa, 1337x)
- Configurable providers
- Provider-specific category mapping
- Result deduplication
- Rate limiting per provider

**Search Request**:
```json
{
  "query": "linux iso",
  "category": "all"
}
```

## Event System

### Event Types

Plugins can subscribe to and handle various system events:

**Torrent Events**:
- `torrent_added` - Torrent was added to the client
- `torrent_started` - Download/seeding started
- `torrent_progress` - Download progress updated
- `torrent_completed` - Download completed
- `torrent_paused` - Torrent paused
- `torrent_resumed` - Torrent resumed
- `torrent_stopped` - Download stopped
- `torrent_removed` - Torrent removed

**Peer Events**:
- `peer_connected` - Peer connected
- `peer_disconnected` - Peer disconnected

**System Events**:
- `system_startup` - System started
- `system_shutdown` - System shutting down

## Plugin Management API

### List All Plugins

`GET /api/plugins`

**Response**:
```json
{
  "plugins": {
    "streaming-server": {
      "name": "streaming-server",
      "version": "1.0.0",
      "state": "running",
      "healthy": true,
      "capabilities": ["stream_server", "event_handler"]
    },
    "search": {
      "name": "search",
      "version": "1.0.0",
      "state": "running",
      "healthy": true,
      "capabilities": ["search"]
    }
  }
}
```

### Start Plugin

`POST /api/plugins/{name}/start`

### Stop Plugin

`POST /api/plugins/{name}/stop`

### Restart Plugin

`POST /api/plugins/{name}/restart`

## Plugin Best Practices

1. **Thread Safety**: Use mutexes for shared state
2. **Error Handling**: Gracefully handle errors in event handlers
3. **Logging**: Use the provided logging methods
4. **Resource Cleanup**: Properly clean up resources
5. **Health Checks**: Implement meaningful IsHealthy() checks
6. **Configuration Validation**: Validate configuration early

## File Structure

Plugin system files are located in:

```
internal/plugins/
├── interface.go        # Plugin interfaces and types
├── registry.go         # Plugin manager implementation
├── events.go           # Event bus and helpers
├── context.go          # Plugin context implementation
├── streaming_server.go # Streaming server plugin
└── search.go           # Search plugin
```

## Creating Custom Plugins

All plugins must implement the `Plugin` interface defined in `interface.go`:

```go
type Plugin interface {
  Name() string
  Version() string
  Description() string
  Author() string
  Initialize(ctx context.Context, config map[string]interface{}) error
  Start(ctx context.Context) error
  Stop(ctx context.Context) error
  Cleanup() error
  HandleEvent(event Event) error
  GetCapabilities() []Capability
  IsHealthy() bool
}
```

For full examples and detailed documentation, see this file and the source code in `internal/plugins/`.
