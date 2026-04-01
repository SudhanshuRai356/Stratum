package plugins

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

type VirusTotalPlugin struct {
	name        string
	version     string
	description string
	author      string

	running   bool
	healthy   bool
	mu        sync.RWMutex
	lastError string

	apiKey  string
	context PluginContext
	client  *http.Client

	scanResults map[string]ScanResult
	resultMu    sync.RWMutex
}

type ScanResult struct {
	InfoHash      string `json:"info_hash"`
	FileName      string `json:"file_name"`
	SHA1          string `json:"sha1"`
	SHA256        string `json:"sha256"`
	DetectionRate string `json:"detection_rate"`
	TotalEngines  int    `json:"total_engines"`
	Detections    int    `json:"detections"`
	ScanDate      string `json:"scan_date"`
	Permalink     string `json:"permalink"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"error,omitempty"`
}

func NewVirusTotalPlugin() Plugin {
	return &VirusTotalPlugin{
		name:        "virustotal",
		version:     "1.0.0",
		description: "VirusTotal file scanner for completed torrents",
		author:      "Stratum Team",
		healthy:     true,
		scanResults: make(map[string]ScanResult),
	}
}

func (p *VirusTotalPlugin) Name() string        { return p.name }
func (p *VirusTotalPlugin) Version() string     { return p.version }
func (p *VirusTotalPlugin) Description() string { return p.description }
func (p *VirusTotalPlugin) Author() string      { return p.author }

func (p *VirusTotalPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.client = &http.Client{Timeout: 10 * time.Second}

	// Priority: env var > config map > mock
	p.apiKey = os.Getenv("VT_API_KEY")

	if p.apiKey == "" {
		if apiKeyVal, ok := config["api_key"]; ok {
			if apiKey, ok := apiKeyVal.(string); ok {
				p.apiKey = apiKey
			}
		}
	}

	if p.apiKey == "" {
		p.context.LogWarn(p.name, "VirusTotal API key not set — using mock data (set VT_API_KEY env var)")
	}

	return nil
}

func (p *VirusTotalPlugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("virustotal plugin already running")
	}

	p.running = true
	p.healthy = true
	p.context.LogInfo(p.name, "VirusTotal plugin started")
	return nil
}

func (p *VirusTotalPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.running = false
	p.context.LogInfo(p.name, "VirusTotal plugin stopped")
	return nil
}

func (p *VirusTotalPlugin) Cleanup() error {
	return nil
}

func (p *VirusTotalPlugin) HandleEvent(event Event) error {
	p.context.LogInfo(p.name, fmt.Sprintf("HandleEvent received: type=%s", event.Type))

	if event.Type != EventTorrentCompleted {
		return nil
	}

	torrentData, ok := event.Data["torrent"].(map[string]interface{})
	if !ok {
		p.context.LogError(p.name, "Failed to extract torrent data from event")
		return nil
	}

	infoHash, ok := torrentData["info_hash"].(string)
	if !ok {
		p.context.LogError(p.name, "Failed to extract info_hash from event")
		return nil
	}

	p.context.LogInfo(p.name, fmt.Sprintf("Received torrent_completed event for %s", infoHash))
	go p.scanTorrent(infoHash)
	return nil
}

func (p *VirusTotalPlugin) GetCapabilities() []Capability {
	return []Capability{CapabilitySecurity}
}

func (p *VirusTotalPlugin) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.healthy
}

func (p *VirusTotalPlugin) SetContext(ctx PluginContext) {
	p.context = ctx
}

func (p *VirusTotalPlugin) scanTorrent(infoHash string) {
	p.context.LogInfo(p.name, fmt.Sprintf("Scanning torrent %s", infoHash))

	var result ScanResult

	if p.apiKey == "" {
		result = p.mockScan(infoHash)
	} else {
		sha1Hash, err := p.computeFileHash(infoHash)
		if err != nil {
			result = ScanResult{
				InfoHash:     infoHash,
				Status:       "error",
				ErrorMessage: err.Error(),
			}
		} else {
			result = p.queryVT(sha1Hash, infoHash)
		}
	}

	p.resultMu.Lock()
	p.scanResults[infoHash] = result
	p.resultMu.Unlock()

	if result.Status == "clean" {
		p.context.LogInfo(p.name, fmt.Sprintf("Scan complete for %s: %s", infoHash, result.DetectionRate))
	} else if result.Status == "malicious" {
		p.context.LogWarn(p.name, fmt.Sprintf("THREAT DETECTED for %s: %s", infoHash, result.DetectionRate))
	} else {
		p.context.LogInfo(p.name, fmt.Sprintf("Scan complete for %s: %s", infoHash, result.Status))
	}
}

func (p *VirusTotalPlugin) computeFileHash(infoHash string) (string, error) {
	files, err := p.context.GetTorrentFiles(infoHash)
	if err != nil {
		return "", fmt.Errorf("failed to get torrent files: %w", err)
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no files for torrent %s", infoHash)
	}

	filePath := files[0]
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	hash := sha1.Sum(data)
	return fmt.Sprintf("%x", hash), nil
}

func (p *VirusTotalPlugin) queryVT(sha1Hash, infoHash string) ScanResult {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("https://www.virustotal.com/api/v3/files/%s", sha1Hash), nil)
	if err != nil {
		return ScanResult{InfoHash: infoHash, Status: "error", ErrorMessage: err.Error()}
	}
	req.Header.Set("x-apikey", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return ScanResult{InfoHash: infoHash, Status: "error", ErrorMessage: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ScanResult{
			InfoHash: infoHash,
			SHA1:     sha1Hash,
			Status:   "not_found",
		}
	}

	if resp.StatusCode != http.StatusOK {
		return ScanResult{
			InfoHash:     infoHash,
			Status:       "error",
			ErrorMessage: fmt.Sprintf("VT API returned %d", resp.StatusCode),
		}
	}

	var result struct {
		Data struct {
			Attributes struct {
				LastAnalysisStats struct {
					Malicious  int `json:"malicious"`
					Suspicious int `json:"suspicious"`
					Undetected int `json:"undetected"`
					Harmless   int `json:"harmless"`
				} `json:"last_analysis_stats"`
				LastAnalysisDate int64  `json:"last_analysis_date"`
				SHA256           string `json:"sha256"`
			} `json:"attributes"`
			Links struct {
				Self string `json:"self"`
			} `json:"links"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ScanResult{InfoHash: infoHash, Status: "error", ErrorMessage: err.Error()}
	}

	total := result.Data.Attributes.LastAnalysisStats.Malicious +
		result.Data.Attributes.LastAnalysisStats.Suspicious +
		result.Data.Attributes.LastAnalysisStats.Undetected +
		result.Data.Attributes.LastAnalysisStats.Harmless

	detections := result.Data.Attributes.LastAnalysisStats.Malicious +
		result.Data.Attributes.LastAnalysisStats.Suspicious

	return ScanResult{
		InfoHash:      infoHash,
		SHA1:          sha1Hash,
		SHA256:        result.Data.Attributes.SHA256,
		DetectionRate: fmt.Sprintf("%d/%d", detections, total),
		TotalEngines:  total,
		Detections:    detections,
		ScanDate:      time.Unix(result.Data.Attributes.LastAnalysisDate, 0).Format(time.RFC3339),
		Permalink:     result.Data.Links.Self,
		Status:        classifyResult(detections, total),
	}
}

func (p *VirusTotalPlugin) mockScan(infoHash string) ScanResult {
	return ScanResult{
		InfoHash:      infoHash,
		FileName:      "mock-file.bin",
		SHA1:          "a" + infoHash[:39],
		SHA256:        "b" + infoHash[:63],
		DetectionRate: "0/70",
		TotalEngines:  70,
		Detections:    0,
		ScanDate:      time.Now().Format(time.RFC3339),
		Status:        "clean",
	}
}

// GetScanResult returns the scan result for a specific torrent
func (p *VirusTotalPlugin) GetScanResult(infoHash string) (ScanResult, bool) {
	p.resultMu.RLock()
	defer p.resultMu.RUnlock()
	result, ok := p.scanResults[infoHash]
	return result, ok
}

// GetAllScanResults returns all scan results
func (p *VirusTotalPlugin) GetAllScanResults() []ScanResult {
	p.resultMu.RLock()
	defer p.resultMu.RUnlock()

	results := make([]ScanResult, 0, len(p.scanResults))
	for _, r := range p.scanResults {
		results = append(results, r)
	}
	return results
}

func classifyResult(detections, total int) string {
	if detections == 0 {
		return "clean"
	}
	if detections <= 3 {
		return "suspicious"
	}
	return "malicious"
}
