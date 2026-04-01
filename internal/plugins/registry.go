package plugins

import (
	"context"
	"fmt"
	"plugin"
	"sync"
	"time"
)

// DefaultPluginManager implements the PluginManager interface
type DefaultPluginManager struct {
	mu               sync.RWMutex
	plugins          map[string]Plugin
	pluginStates     map[string]PluginState
	pluginConfigs    map[string]map[string]interface{}
	pluginErrors     map[string]string
	pluginStartTimes map[string]int64
	eventBus         *EventBus
	context          PluginContext
	dataDirectory    string
}

// NewPluginManager creates a new plugin manager
func NewPluginManager(dataDirectory string, context PluginContext) *DefaultPluginManager {
	return &DefaultPluginManager{
		plugins:          make(map[string]Plugin),
		pluginStates:     make(map[string]PluginState),
		pluginConfigs:    make(map[string]map[string]interface{}),
		pluginErrors:     make(map[string]string),
		pluginStartTimes: make(map[string]int64),
		eventBus:         NewEventBus(),
		context:          context,
		dataDirectory:    dataDirectory,
	}
}

// RegisterPlugin registers a new plugin
func (pm *DefaultPluginManager) RegisterPlugin(plugin Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	name := plugin.Name()
	if _, exists := pm.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	// Inject the plugin context
	plugin.SetContext(pm.context)

	pm.plugins[name] = plugin
	pm.pluginStates[name] = PluginStateRegistered
	pm.context.LogInfo("PluginManager", fmt.Sprintf("Registered plugin: %s v%s", name, plugin.Version()))

	return nil
}

// UnregisterPlugin unregisters a plugin
func (pm *DefaultPluginManager) UnregisterPlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plugin, exists := pm.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// Stop the plugin if it's running
	if pm.pluginStates[name] == PluginStateRunning {
		if err := plugin.Stop(context.Background()); err != nil {
			pm.context.LogError("PluginManager", fmt.Sprintf("Error stopping plugin %s: %v", name, err))
		}
	}

	// Clean up the plugin
	if err := plugin.Cleanup(); err != nil {
		pm.context.LogError("PluginManager", fmt.Sprintf("Error cleaning up plugin %s: %v", name, err))
	}

	delete(pm.plugins, name)
	delete(pm.pluginStates, name)
	delete(pm.pluginConfigs, name)
	delete(pm.pluginErrors, name)
	delete(pm.pluginStartTimes, name)

	pm.context.LogInfo("PluginManager", fmt.Sprintf("Unregistered plugin: %s", name))
	return nil
}

// GetPlugin returns a plugin by name
func (pm *DefaultPluginManager) GetPlugin(name string) (Plugin, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plugin, exists := pm.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	return plugin, nil
}

// GetAllPlugins returns all registered plugins
func (pm *DefaultPluginManager) GetAllPlugins() map[string]Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[string]Plugin)
	for name, plugin := range pm.plugins {
		result[name] = plugin
	}

	return result
}

// StartPlugin starts a plugin
func (pm *DefaultPluginManager) StartPlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plugin, exists := pm.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if pm.pluginStates[name] == PluginStateRunning {
		return fmt.Errorf("plugin %s is already running", name)
	}

	pm.pluginStates[name] = PluginStateStarting
	pm.pluginErrors[name] = ""

	config := pm.pluginConfigs[name]
	if config == nil {
		config = make(map[string]interface{})
	}

	ctx := context.Background()

	// Initialize the plugin
	if err := plugin.Initialize(ctx, config); err != nil {
		pm.pluginStates[name] = PluginStateError
		pm.pluginErrors[name] = err.Error()
		return fmt.Errorf("failed to initialize plugin %s: %w", name, err)
	}

	// Start the plugin
	if err := plugin.Start(ctx); err != nil {
		pm.pluginStates[name] = PluginStateError
		pm.pluginErrors[name] = err.Error()
		return fmt.Errorf("failed to start plugin %s: %w", name, err)
	}

	pm.pluginStates[name] = PluginStateRunning
	pm.pluginStartTimes[name] = time.Now().Unix()

	pm.context.LogInfo("PluginManager", fmt.Sprintf("Started plugin: %s", name))
	return nil
}

// StopPlugin stops a plugin
func (pm *DefaultPluginManager) StopPlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plugin, exists := pm.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if pm.pluginStates[name] != PluginStateRunning {
		return fmt.Errorf("plugin %s is not running", name)
	}

	pm.pluginStates[name] = PluginStateStopping

	ctx := context.Background()
	if err := plugin.Stop(ctx); err != nil {
		pm.pluginStates[name] = PluginStateError
		pm.pluginErrors[name] = err.Error()
		return fmt.Errorf("failed to stop plugin %s: %w", name, err)
	}

	pm.pluginStates[name] = PluginStateStopped
	delete(pm.pluginStartTimes, name)

	pm.context.LogInfo("PluginManager", fmt.Sprintf("Stopped plugin: %s", name))
	return nil
}

// RestartPlugin restarts a plugin
func (pm *DefaultPluginManager) RestartPlugin(name string) error {
	if err := pm.StopPlugin(name); err != nil {
		return err
	}

	time.Sleep(1 * time.Second) // Give it a moment

	return pm.StartPlugin(name)
}

// EmitEvent emits an event to all interested plugins
func (pm *DefaultPluginManager) EmitEvent(event Event) error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for name, plugin := range pm.plugins {
		if pm.pluginStates[name] == PluginStateRunning {
			go func(pluginName string, p Plugin) {
				if err := p.HandleEvent(event); err != nil {
					pm.context.LogError("PluginManager",
						fmt.Sprintf("Plugin %s failed to handle event %s: %v", pluginName, event.Type, err))
				}
			}(name, plugin)
		}
	}

	return nil
}

// SubscribeToEvents subscribes a plugin to specific event types
func (pm *DefaultPluginManager) SubscribeToEvents(pluginName string, eventTypes []EventType) error {
	// For now, all plugins receive all events
	// In the future, we could implement selective event subscription
	return nil
}

// DiscoverPlugins discovers plugins in a directory
func (pm *DefaultPluginManager) DiscoverPlugins(directory string) error {
	pm.context.LogInfo("PluginManager", fmt.Sprintf("Discovering plugins in: %s", directory))

	// For now, we'll focus on the built-in plugins
	// In the future, we can implement dynamic plugin loading from .so files

	return nil
}

// LoadPlugin loads a plugin from a file
func (pm *DefaultPluginManager) LoadPlugin(path string) (Plugin, error) {
	// Load plugin as Go plugin (.so file)
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	// Look for the NewPlugin symbol
	symPlugin, err := p.Lookup("NewPlugin")
	if err != nil {
		return nil, fmt.Errorf("plugin %s does not export NewPlugin function: %w", path, err)
	}

	// Assert that NewPlugin is a function that returns a Plugin
	newPlugin, ok := symPlugin.(func() Plugin)
	if !ok {
		return nil, fmt.Errorf("plugin %s NewPlugin function has wrong signature", path)
	}

	return newPlugin(), nil
}

// GetPluginConfig returns the configuration for a plugin
func (pm *DefaultPluginManager) GetPluginConfig(name string) (map[string]interface{}, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	config, exists := pm.pluginConfigs[name]
	if !exists {
		return make(map[string]interface{}), nil
	}

	return config, nil
}

// UpdatePluginConfig updates the configuration for a plugin
func (pm *DefaultPluginManager) UpdatePluginConfig(name string, config map[string]interface{}) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.plugins[name]; !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	pm.pluginConfigs[name] = config
	pm.context.LogInfo("PluginManager", fmt.Sprintf("Updated config for plugin: %s", name))

	return nil
}

// GetPluginsByCapability returns plugins that have a specific capability
func (pm *DefaultPluginManager) GetPluginsByCapability(capability Capability) []Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []Plugin
	for _, plugin := range pm.plugins {
		for _, cap := range plugin.GetCapabilities() {
			if cap == capability {
				result = append(result, plugin)
				break
			}
		}
	}

	return result
}

// GetPluginStatus returns the status of a plugin
func (pm *DefaultPluginManager) GetPluginStatus(name string) (PluginStatus, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plugin, exists := pm.plugins[name]
	if !exists {
		return PluginStatus{}, fmt.Errorf("plugin %s not found", name)
	}

	status := PluginStatus{
		Name:          plugin.Name(),
		Version:       plugin.Version(),
		State:         pm.pluginStates[name],
		Healthy:       plugin.IsHealthy(),
		LastError:     pm.pluginErrors[name],
		Configuration: pm.pluginConfigs[name],
		Capabilities:  plugin.GetCapabilities(),
	}

	if startTime, exists := pm.pluginStartTimes[name]; exists {
		status.StartTime = startTime
	}

	return status, nil
}

// GetAllPluginStatuses returns the status of all plugins
func (pm *DefaultPluginManager) GetAllPluginStatuses() map[string]PluginStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[string]PluginStatus)
	for name := range pm.plugins {
		if status, err := pm.GetPluginStatus(name); err == nil {
			result[name] = status
		}
	}

	return result
}

// Shutdown stops all plugins and cleans up
func (pm *DefaultPluginManager) Shutdown() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.context.LogInfo("PluginManager", "Shutting down all plugins...")

	var errs []error
	for name, plugin := range pm.plugins {
		if pm.pluginStates[name] == PluginStateRunning {
			if err := plugin.Stop(context.Background()); err != nil {
				errs = append(errs, err)
				pm.context.LogError("PluginManager", fmt.Sprintf("Error stopping plugin %s: %v", name, err))
			}
		}

		if err := plugin.Cleanup(); err != nil {
			errs = append(errs, err)
			pm.context.LogError("PluginManager", fmt.Sprintf("Error cleaning up plugin %s: %v", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors occurred during shutdown: %v", errs)
	}

	pm.context.LogInfo("PluginManager", "All plugins shut down successfully")
	return nil
}