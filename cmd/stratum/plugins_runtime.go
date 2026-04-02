package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kushal-39/Stratum/internal/plugins"
	"github.com/Kushal-39/Stratum/internal/storage"
)

var (
	pluginRuntimeMu          sync.RWMutex
	pluginRuntimeManager     plugins.PluginManager
	pluginRuntimeContext     *plugins.DefaultPluginContext
	pluginRuntimeDownloads   *startedDownloadManager
	pluginRuntimeOutputDir   string
	pluginRuntimeTorrentFile = make(map[string][]string)
)

func normalizePluginInfoHash(infoHash string) string {
	return strings.ToLower(strings.TrimSpace(infoHash))
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, len(values))
	copy(out, values)
	return out
}

func getPluginManager() plugins.PluginManager {
	pluginRuntimeMu.RLock()
	defer pluginRuntimeMu.RUnlock()
	return pluginRuntimeManager
}

func initializePluginSystem(outputDir string, manager *startedDownloadManager) error {
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}

	pluginDataDir := filepath.Join(absOutputDir, ".stratum")
	if err := os.MkdirAll(pluginDataDir, 0o755); err != nil {
		return fmt.Errorf("failed to create plugin data directory: %w", err)
	}

	pluginCtx := plugins.NewPluginContext(absOutputDir, pluginDataDir)
	pluginCtx.SetSystemOperations(
		getPluginTorrentState,
		getAllPluginTorrentStates,
		func(source string) (string, error) {
			if manager == nil {
				return "", fmt.Errorf("download manager unavailable")
			}

			started, _, err := startManagedDownload(source, absOutputDir, manager)
			if err != nil {
				return "", err
			}

			if started.infoHash != "" && started.outputPath != "" {
				registerPluginTorrentFiles(started.infoHash, []string{started.outputPath})
			}

			if started.infoHash != "" {
				return started.infoHash, nil
			}

			return started.key, nil
		},
		func(infoHash string) error {
			if manager == nil {
				return fmt.Errorf("download manager unavailable")
			}
			_, err := manager.stopByInfoHash(infoHash)
			return err
		},
		func(infoHash string) error {
			if manager == nil {
				return fmt.Errorf("download manager unavailable")
			}
			_, err := manager.stopByInfoHash(infoHash)
			return err
		},
		func(infoHash string) error {
			return fmt.Errorf("resume is not supported for detached downloads (%s)", infoHash)
		},
		getPluginTorrentFiles,
	)

	mgr := plugins.NewPluginManager(pluginDataDir, pluginCtx)
	if err := registerBuiltInPlugins(mgr); err != nil {
		return err
	}

	pluginRuntimeMu.Lock()
	pluginRuntimeManager = mgr
	pluginRuntimeContext = pluginCtx
	pluginRuntimeDownloads = manager
	pluginRuntimeOutputDir = absOutputDir
	pluginRuntimeMu.Unlock()

	return nil
}

func registerBuiltInPlugins(manager plugins.PluginManager) error {
	builtIns := []plugins.Plugin{
		plugins.NewStreamingServerPlugin(),
		plugins.NewSearchPlugin(),
		plugins.NewVirusTotalPlugin(),
	}

	for _, builtIn := range builtIns {
		if err := manager.RegisterPlugin(builtIn); err != nil {
			return fmt.Errorf("failed to register plugin %s: %w", builtIn.Name(), err)
		}
	}

	// Keep frontend messaging unchanged while enforcing backend search provider policy.
	if err := manager.UpdatePluginConfig("search", map[string]interface{}{
		"enabled_providers": []interface{}{"the-pirate-bay"},
	}); err != nil {
		return fmt.Errorf("failed to configure search plugin: %w", err)
	}

	return nil
}

func startPlugins() error {
	manager := getPluginManager()
	if manager == nil {
		return fmt.Errorf("plugin manager is not initialized")
	}

	allPlugins := manager.GetAllPlugins()
	names := make([]string, 0, len(allPlugins))
	for name := range allPlugins {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		status, err := manager.GetPluginStatus(name)
		if err != nil {
			return fmt.Errorf("failed to read status for plugin %s: %w", name, err)
		}

		if status.State == plugins.PluginStateRunning {
			continue
		}

		if err := manager.StartPlugin(name); err != nil {
			return err
		}
	}

	return manager.EmitEvent(plugins.Event{
		Type:      plugins.EventSystemStartup,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"started_at": time.Now().Format(time.RFC3339),
		},
	})
}

func registerPluginTorrentFiles(infoHash string, filePaths []string) {
	normalizedInfoHash := normalizePluginInfoHash(infoHash)
	if normalizedInfoHash == "" {
		return
	}

	unique := make(map[string]struct{})
	for _, path := range filePaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		unique[trimmed] = struct{}{}
	}

	if len(unique) == 0 {
		return
	}

	pluginRuntimeMu.Lock()
	defer pluginRuntimeMu.Unlock()

	merged := make(map[string]struct{})
	for _, existing := range pluginRuntimeTorrentFile[normalizedInfoHash] {
		merged[existing] = struct{}{}
	}
	for filePath := range unique {
		merged[filePath] = struct{}{}
	}

	files := make([]string, 0, len(merged))
	for filePath := range merged {
		files = append(files, filePath)
	}
	sort.Strings(files)
	pluginRuntimeTorrentFile[normalizedInfoHash] = files
}

func buildPluginTorrentFileList(outputPath string, storageFiles []storage.FileInfo) []string {
	trimmedOutput := strings.TrimSpace(outputPath)
	if trimmedOutput == "" {
		return nil
	}

	if len(storageFiles) == 0 {
		return []string{trimmedOutput}
	}

	paths := make([]string, 0, len(storageFiles))
	for _, fileInfo := range storageFiles {
		if len(fileInfo.Path) == 0 {
			paths = append(paths, trimmedOutput)
			continue
		}

		joined := filepath.Join(append([]string{trimmedOutput}, fileInfo.Path...)...)
		paths = append(paths, joined)
	}

	return paths
}

func getPluginTorrentState(infoHash string) (*plugins.TorrentState, error) {
	normalizedInfoHash := normalizePluginInfoHash(infoHash)
	if normalizedInfoHash == "" {
		return nil, fmt.Errorf("info hash is required")
	}

	states, err := getAllPluginTorrentStates()
	if err != nil {
		return nil, err
	}

	for _, state := range states {
		if normalizePluginInfoHash(state.InfoHash) == normalizedInfoHash {
			stateCopy := state
			return &stateCopy, nil
		}
	}

	return nil, fmt.Errorf("torrent %s not found", infoHash)
}

func getAllPluginTorrentStates() ([]plugins.TorrentState, error) {
	pluginRuntimeMu.RLock()
	downloads := pluginRuntimeDownloads
	pluginRuntimeMu.RUnlock()

	if downloads == nil {
		return []plugins.TorrentState{}, nil
	}

	states := downloads.snapshotAndUpdate()
	out := make([]plugins.TorrentState, 0, len(states))
	for _, state := range states {
		out = append(out, plugins.TorrentState{
			InfoHash:        state.InfoHash,
			Name:            state.Name,
			Progress:        state.Progress,
			DownloadSpeed:   state.DownloadSpeed,
			UploadSpeed:     state.UploadSpeed,
			State:           state.State,
			ConnectedPeers:  state.ConnectedPeers,
			Seeders:         state.Seeders,
			Leechers:        state.Leechers,
			CompletedPieces: state.CompletedPieces,
			TotalPieces:     state.TotalPieces,
			Round:           state.Round,
		})
	}

	return out, nil
}

func getPluginTorrentFiles(infoHash string) ([]string, error) {
	normalizedInfoHash := normalizePluginInfoHash(infoHash)
	if normalizedInfoHash == "" {
		return nil, fmt.Errorf("info hash is required")
	}

	pluginRuntimeMu.RLock()
	files := cloneStringSlice(pluginRuntimeTorrentFile[normalizedInfoHash])
	downloads := pluginRuntimeDownloads
	pluginRuntimeMu.RUnlock()

	if len(files) > 0 {
		return files, nil
	}

	if downloads != nil {
		downloads.mu.Lock()
		for _, item := range downloads.items {
			if normalizePluginInfoHash(item.infoHash) != normalizedInfoHash && normalizePluginInfoHash(item.key) != normalizedInfoHash {
				continue
			}

			if strings.TrimSpace(item.outputPath) != "" {
				files = []string{item.outputPath}
				break
			}
		}
		downloads.mu.Unlock()
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found for torrent %s", infoHash)
	}

	registerPluginTorrentFiles(normalizedInfoHash, files)
	return files, nil
}
