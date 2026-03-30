# Stratum Plugin Development Guide

## Quick Start

### Step 1: Create Your Plugin File

Create a new Go file in the `internal/plugins/` directory:

```bash
touch internal/plugins/myplugin.go
```

### Step 2: Implement the Plugin Interface

```go
package plugins

import (
	"context"
	"sync"
)

type MyAwesomePlugin struct {
	name        string
	version     string
	description string
	author      string
	running     bool
	healthy     bool
	mu          sync.RWMutex
	lastError   string
	context     PluginContext
}

func NewMyAwesomePlugin() Plugin {
	return &MyAwesomePlugin{
		name:        "my-awesome-plugin",
		version:     "1.0.0",
		description: "My awesome custom plugin",
		author:      "Your Name",
		healthy:     true,
	}
}

// Required methods

func (p *MyAwesomePlugin) Name() string                                                    { return p.name }
func (p *MyAwesomePlugin) Version() string                                                { return p.version }
func (p *MyAwesomePlugin) Description() string                                            { return p.description }
func (p *MyAwesomePlugin) Author() string                                                 { return p.author }
func (p *MyAwesomePlugin) IsHealthy() bool                                                {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.healthy
}

func (p *MyAwesomePlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	// Initialize plugin with configuration
	// config contains settings from plugin config file or API
	// Set default values, validate configuration, etc.
	return nil
}

func (p *MyAwesomePlugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("plugin already running")
	}

	p.running = true
	p.context.LogInfo(p.name, "Plugin started successfully")
	return nil
}

func (p *MyAwesomePlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.running = false
	p.context.LogInfo(p.name, "Plugin stopped")
	return nil
}

func (p *MyAwesomePlugin) Cleanup() error {
	// Clean up any resources (files, connections, etc.)
	return nil
}

func (p *MyAwesomePlugin) HandleEvent(event Event) error {
	// Handle system events
	// Example: react to torrent completion, pause, etc.
	return nil
}

func (p *MyAwesomePlugin) GetCapabilities() []Capability {
	return []Capability{
		CapabilityEventHandler,
		// Add other capabilities your plugin provides
	}
}
```

### Step 3: Register Your Plugin

Edit `cmd/stratum/plugins.go` and modify the `registerBuiltInPlugins` function:

```go
func registerBuiltInPlugins() error {
	// ... existing plugins ...

	// Register your plugin
	myPlugin := plugins.NewMyAwesomePlugin()
	if err := pluginManager.RegisterPlugin(myPlugin); err != nil {
		return err
	}

	log.Println("Your plugin registered successfully")
	return nil
}
```

### Step 4: Test Your Plugin

Build and run Stratum:

```bash
go run ./cmd/stratum
```

Check the plugins page in the frontend to see your plugin and its status.

## Common Plugin Patterns

### Pattern 1: Event Handler Plugin

Listen to torrent events and take action:

```go
func (p *MyPlugin) HandleEvent(event Event) error {
	switch event.Type {
	case EventTorrentCompleted:
		// Handle torrent completion
		torrentData := event.Data["torrent"].(map[string]interface{})
		infoHash := torrentData["info_hash"].(string)
		p.context.LogInfo(p.name, fmt.Sprintf("Torrent %s completed!", infoHash))

	case EventTorrentPaused:
		// Handle torrent pause
		torrentData := event.Data["torrent"].(map[string]interface{})
		infoHash := torrentData["info_hash"].(string)
		p.context.LogInfo(p.name, fmt.Sprintf("Torrent %s paused", infoHash))
	}
	return nil
}
```

### Pattern 2: API Extension Plugin

Add custom API endpoints:

```go
type MyAPIPlugin struct {
	// ... fields ...
}

func (p *MyAPIPlugin) RegisterRoutes(mux *http.ServeMux) error {
	mux.HandleFunc(p.GetAPIPrefix()+"/stats", p.handleStats)
	mux.HandleFunc(p.GetAPIPrefix()+"/config", p.handleConfig)
	return nil
}

func (p *MyAPIPlugin) GetAPIPrefix() string {
	return "/api/my-plugin"
}

func (p *MyAPIPlugin) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get torrent statistics
	torrents, _ := p.context.GetAllTorrentStates()

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_torrents": len(torrents),
		"torrents": torrents,
	})
}
```

### Pattern 3: Monitor Plugin

Track and log torrent metrics:

```go
func (p *MyMonitorPlugin) Start(ctx context.Context) error {
	p.running = true

	// Start monitoring goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				torrents, err := p.context.GetAllTorrentStates()
				if err != nil {
					continue
				}

				for _, torrent := range torrents {
					p.context.LogInfo(p.name, fmt.Sprintf(
						"Torrent %s: %v%% @ %.2f MB/s",
						torrent.Name,
						int(torrent.Progress),
						torrent.DownloadSpeed/(1024*1024),
					))
				}
			}
		}
	}()

	return nil
}
```

### Pattern 4: Filter/Action Plugin

Filter torrents and take automated actions:

```go
func (p *MyFilterPlugin) HandleEvent(event Event) error {
	if event.Type != EventTorrentProgress {
		return nil
	}

	torrentData := event.Data["torrent"].(map[string]interface{})
	infoHash := torrentData["info_hash"].(string)

	state, err := p.context.GetTorrentState(infoHash)
	if err != nil {
		return err
	}

	// Example: Pause torrents larger than 10GB
	if state.Size > 10*1024*1024*1024 {
		p.context.LogInfo(p.name, fmt.Sprintf("Pausing large torrent: %s", state.Name))
		return p.context.PauseTorrent(infoHash)
	}

	return nil
}
```

## Configuration Management

### Loading Configuration

```go
func (p *MyPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	// Extract configuration values
	if nameVal, ok := config["name"]; ok {
		if name, ok := nameVal.(string); ok {
			p.customName = name
		}
	}

	if enabledVal, ok := config["enabled"]; ok {
		if enabled, ok := enabledVal.(bool); ok {
			p.enabled = enabled
		}
	}

	if numbersVal, ok := config["numbers"]; ok {
		if num, ok := numbersVal.(float64); ok {
			p.count = int(num)
		}
	}

	return nil
}
```

### Storing Plugin Data

```go
func (p *MyPlugin) Start(ctx context.Context) error {
	// Get plugin-specific data directory
	dataDir := p.context.GetPluginDataDirectory(p.name)

	// Create a file for storing data
	dataFile := filepath.Join(dataDir, "plugin-data.json")

	// Save data
	data := map[string]interface{}{
		"timestamp": time.Now(),
		"torrent_count": 42,
	}

	jsonData, _ := json.Marshal(data)
	ioutil.WriteFile(dataFile, jsonData, 0644)

	return nil
}
```

## Debugging Plugins

### Logging

Use the context logging methods:

```go
// Info level
p.context.LogInfo(p.name, "Operation completed successfully")

// Warning level
p.context.LogWarn(p.name, "This is unusual but not critical")

// Error level
p.context.LogError(p.name, "An error occurred: " + err.Error())
```

### Health Status

Update plugin health status:

```go
func (p *MyPlugin) monitorHealth() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.Lock()

		// Check system resources, connections, etc.
		isHealthy := p.systemHealthCheck()

		if isHealthy {
			p.healthy = true
			p.lastError = ""
		} else {
			p.healthy = false
			p.lastError = "Health check failed"
		}

		p.mu.Unlock()
	}
}
```

## Testing Plugins

### Unit Testing

```go
func TestMyPluginInitialize(t *testing.T) {
	plugin := NewMyAwesomePlugin()

	config := map[string]interface{}{
		"enabled": true,
		"timeout": 30,
	}

	err := plugin.Initialize(context.Background(), config)
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	if !plugin.IsHealthy() {
		t.Fatal("Plugin should be healthy after initialization")
	}
}

func TestMyPluginLifecycle(t *testing.T) {
	ctx := context.Background()
	plugin := NewMyAwesomePlugin()

	// Initialize
	err := plugin.Initialize(ctx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Start
	err = plugin.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop
	err = plugin.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Cleanup
	err = plugin.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
}
```

## API Interaction Examples

### Frontend JavaScript - Getting Plugin Status

```javascript
async function getPlugins() {
  const response = await fetch('http://127.0.0.1:8080/api/ws-auth');
  const { token } = await response.json();

  const pluginsResponse = await fetch(
    `http://127.0.0.1:8080/api/plugins?token=${token}`
  );
  const data = await pluginsResponse.json();

  console.log('Installed plugins:', data.plugins);
}
```

### Frontend JavaScript - Start Plugin

```javascript
async function startPlugin(pluginName) {
  const response = await fetch('http://127.0.0.1:8080/api/ws-auth');
  const { token } = await response.json();

  const startResponse = await fetch(
    `http://127.0.0.1:8080/api/plugins/${pluginName}/start?token=${token}`,
    { method: 'POST' }
  );

  if (startResponse.ok) {
    console.log(`Plugin ${pluginName} started`);
  }
}
```

### cURL Examples

```bash
# Get all plugins
curl "http://127.0.0.1:8080/api/plugins?token=$(curl -s http://127.0.0.1:8080/api/ws-auth | jq -r '.token')"

# Start a plugin
curl -X POST "http://127.0.0.1:8080/api/plugins/my-plugin/start?token=$(curl -s http://127.0.0.1:8080/api/ws-auth | jq -r '.token')"

# Get plugin config
curl "http://127.0.0.1:8080/api/plugins/streaming-server/config?token=$(curl -s http://127.0.0.1:8080/api/ws-auth | jq -r '.token')"

# Update plugin config
curl -X PUT "http://127.0.0.1:8080/api/plugins/streaming-server/config?token=$(curl -s http://127.0.0.1:8080/api/ws-auth | jq -r '.token')" \
  -H "Content-Type: application/json" \
  -d '{"port": 8082, "max_streams": 20}'
```

## Performance Tips

1. **Avoid blocking operations** in event handlers
2. **Use goroutines** for long-running tasks
3. **Cache frequently accessed data** to reduce system calls
4. **Clean up resources** properly to avoid memory leaks
5. **Use sync.RWMutex** for concurrent access to plugin state
6. **Monitor function execution time** for bottlenecks

## Common Mistakes to Avoid

1. **Not checking health in IsHealthy()** - Always return accurate health status
2. **Ignoring context cancellation** - Respect context deadlines in goroutines
3. **Not locking mutex** - Use proper synchronization for concurrent access
4. **Memory leaks** - Always cleanup resources in Stop() and Cleanup()
5. **Blocking operations** - Keep event handlers fast and non-blocking
6. **Panics in goroutines** - Always recover from panics
7. **Ignoring configuration errors** - Validate config early in Initialize()

## Next Steps

- Check the full [Plugin System Documentation](./PLUGIN_SYSTEM.md)
- Review the source code of built-in plugins for examples
- Create your first plugin!
- Share your plugin with the community