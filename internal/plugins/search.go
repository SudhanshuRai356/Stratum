package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SearchPlugin provides torrent search functionality across multiple providers
type SearchPlugin struct {
	name        string
	version     string
	description string
	author      string

	// Plugin state
	running   bool
	healthy   bool
	mu        sync.RWMutex
	lastError string

	// Configuration
	enabledProviders []string
	maxResults       int
	timeout          time.Duration

	// Search providers
	providers map[string]SearchProviderConfig

	// Plugin context
	context PluginContext

	// HTTP client
	client *http.Client
}

// SearchProviderConfig contains configuration for a search provider
type SearchProviderConfig struct {
	Name           string            `json:"name"`
	BaseURL        string            `json:"base_url"`
	FallbackURLs   []string          `json:"fallback_urls"` // Alternative domains
	SearchPath     string            `json:"search_path"`
	Categories     map[string]string `json:"categories"`
	Enabled        bool              `json:"enabled"`
	RateLimit      time.Duration     `json:"rate_limit"`
	Headers        map[string]string `json:"headers"`
	ResultSelector string            `json:"result_selector"`
	FieldMappings  map[string]string `json:"field_mappings"`
}

// NewSearchPlugin creates a new search plugin
func NewSearchPlugin() Plugin {
	return &SearchPlugin{
		name:        "search",
		version:     "1.0.0",
		description: "Multi-provider torrent search engine",
		author:      "Stratum Team",
		providers:   make(map[string]SearchProviderConfig),
		timeout:     30 * time.Second,
		maxResults:  100,
	}
}

// Plugin interface implementation

func (p *SearchPlugin) Name() string {
	return p.name
}

func (p *SearchPlugin) Version() string {
	return p.version
}

func (p *SearchPlugin) Description() string {
	return p.description
}

func (p *SearchPlugin) Author() string {
	return p.author
}

func (p *SearchPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Create HTTP client with cookie jar for Cloudflare sites
	jar, _ := cookiejar.New(nil)
	p.client = &http.Client{
		Timeout: p.timeout,
		Jar:     jar, // Enable cookie handling for Cloudflare sessions
		Transport: &http.Transport{
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	// Parse configuration
	if timeoutVal, ok := config["timeout"]; ok {
		if timeoutSec, ok := timeoutVal.(float64); ok {
			p.timeout = time.Duration(timeoutSec) * time.Second
			p.client.Timeout = p.timeout
		}
	}

	if maxVal, ok := config["max_results"]; ok {
		if max, ok := maxVal.(float64); ok {
			p.maxResults = int(max)
		}
	}

	// Initialize default providers
	p.initializeDefaultProviders()

	// Load custom providers from config
	if providersVal, ok := config["providers"]; ok {
		if providersMap, ok := providersVal.(map[string]interface{}); ok {
			p.loadProvidersFromConfig(providersMap)
		}
	}

	if enabledVal, ok := config["enabled_providers"]; ok {
		switch enabled := enabledVal.(type) {
		case []interface{}:
			providers := make([]string, 0, len(enabled))
			for _, provider := range enabled {
				if providerName, ok := provider.(string); ok {
					providers = append(providers, providerName)
				}
			}
			p.enabledProviders = providers
		case []string:
			p.enabledProviders = append([]string(nil), enabled...)
		}
	}

	// Default to Pirate Bay only.
	if len(p.enabledProviders) == 0 {
		p.enabledProviders = []string{"the-pirate-bay"}
	}

	p.healthy = true
	return nil
}

func (p *SearchPlugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("search plugin is already running")
	}

	p.running = true

	if p.context != nil {
		p.context.LogInfo(p.name, "Search plugin started")
	}

	return nil
}

func (p *SearchPlugin) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.running = false

	if p.context != nil {
		p.context.LogInfo(p.name, "Search plugin stopped")
	}

	return nil
}

func (p *SearchPlugin) Cleanup() error {
	return nil
}

func (p *SearchPlugin) HandleEvent(event Event) error {
	// Search plugin doesn't need to handle system events
	return nil
}

func (p *SearchPlugin) GetCapabilities() []Capability {
	return []Capability{CapabilitySearch}
}

func (p *SearchPlugin) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.healthy
}

// SetContext injects the plugin context
func (p *SearchPlugin) SetContext(ctx PluginContext) {
	p.context = ctx
}

// SearchProvider interface implementation

func (p *SearchPlugin) Search(query string, category string) ([]SearchResult, error) {
	if !p.running {
		return nil, fmt.Errorf("search plugin is not running")
	}

	var allResults []SearchResult
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	// Search across all enabled providers concurrently
	for _, providerName := range p.enabledProviders {
		provider, exists := p.providers[providerName]
		if !exists || !provider.Enabled {
			continue
		}

		wg.Add(1)
		go func(prov SearchProviderConfig) {
			defer wg.Done()

			results, err := p.searchProvider(prov, query, category)
			if err != nil {
				p.context.LogWarn(p.name, fmt.Sprintf("Search failed for provider %s: %v", prov.Name, err))
				return
			}

			resultsMu.Lock()
			allResults = append(allResults, results...)
			resultsMu.Unlock()
		}(provider)
	}

	wg.Wait()

	// Deduplicate and limit results
	uniqueResults := p.deduplicateResults(allResults)
	if len(uniqueResults) > p.maxResults {
		uniqueResults = uniqueResults[:p.maxResults]
	}

	return uniqueResults, nil
}

func (p *SearchPlugin) GetSupportedCategories() []string {
	categories := make(map[string]bool)

	for _, providerName := range p.enabledProviders {
		if provider, exists := p.providers[providerName]; exists && provider.Enabled {
			for category := range provider.Categories {
				categories[category] = true
			}
		}
	}

	result := make([]string, 0, len(categories))
	for category := range categories {
		result = append(result, category)
	}

	return result
}

func (p *SearchPlugin) GetProviderName() string {
	return "multi-provider"
}

// Private methods

func (p *SearchPlugin) initializeDefaultProviders() {
	// The Pirate Bay - JSON API (working)
	p.providers["the-pirate-bay"] = SearchProviderConfig{
		Name:       "The Pirate Bay",
		BaseURL:    "https://apibay.org",
		SearchPath: "/q.php",
		Categories: map[string]string{
			"all":   "",
			"video": "200",
			"audio": "100",
			"apps":  "300",
			"games": "400",
			"other": "600",
		},
		Enabled:   true,
		RateLimit: 1 * time.Second,
	}

	// YTS (Movies) - JSON API with fallback domains
	p.providers["yts"] = SearchProviderConfig{
		Name:    "YTS",
		BaseURL: "https://yts.lt",
		FallbackURLs: []string{
			"https://yts.am",
			"https://yts.ag",
			"https://yts.mx",
		},
		SearchPath: "/api/v2/list_movies.json",
		Categories: map[string]string{
			"all":    "",
			"movies": "",
		},
		Enabled:   false, // DISABLED for testing
		RateLimit: 1 * time.Second,
	}

	// Nyaa (Anime) - RSS feed with mirrors
	p.providers["nyaa"] = SearchProviderConfig{
		Name:    "Nyaa",
		BaseURL: "https://nyaa.si",
		FallbackURLs: []string{
			"https://nyaa.land",
			"https://nyaa.iss.one",
		},
		SearchPath: "/?page=rss&q=",
		Categories: map[string]string{
			"all":        "",
			"anime":      "&c=1_2",
			"audio":      "&c=2_0",
			"literature": "&c=3_0",
			"software":   "&c=6_0",
		},
		Enabled:   false, // DISABLED for testing
		RateLimit: 1 * time.Second,
	}

	// TorrentGalaxy with fallback domains
	p.providers["torrentgalaxy"] = SearchProviderConfig{
		Name:    "TorrentGalaxy",
		BaseURL: "https://torrentgalaxy.to",
		FallbackURLs: []string{
			"https://torrentgalaxy.mx",
			"https://torrentgalaxy.org",
		},
		SearchPath: "/torrents.php",
		Categories: map[string]string{
			"all":    "",
			"movies": "&c3=1",
			"tv":     "&c41=1",
			"games":  "&c4=1",
			"apps":   "&c1=1",
		},
		Enabled:   false, // DISABLED for testing
		RateLimit: 2 * time.Second,
	}

	// EZTV (TV Shows) - Has API
	p.providers["eztv"] = SearchProviderConfig{
		Name:       "EZTV",
		BaseURL:    "https://eztv.re",
		SearchPath: "/api/get-torrents",
		Categories: map[string]string{
			"all": "",
			"tv":  "",
		},
		Enabled:   false, // DISABLED for testing
		RateLimit: 1 * time.Second,
	}

	// 1337x - Popular general tracker with fallback domains
	p.providers["1337x"] = SearchProviderConfig{
		Name:    "1337x",
		BaseURL: "https://1337x.to",
		FallbackURLs: []string{
			"https://1337x.st",
			"https://x1337x.ws",
			"https://1337x.gd",
		},
		SearchPath: "/search/",
		Categories: map[string]string{
			"all":    "",
			"movies": "Movies",
			"tv":     "TV",
			"games":  "Games",
			"music":  "Music",
			"apps":   "Apps",
		},
		Enabled:   false,           // DISABLED - Cloudflare protection blocks all requests
		RateLimit: 5 * time.Second, // More respectful rate limit
	}

	// LimeTorrents - General tracker with multiple domains
	p.providers["limetorrents"] = SearchProviderConfig{
		Name:    "LimeTorrents",
		BaseURL: "https://www.limetorrents.fun", // UPDATED working domain
		FallbackURLs: []string{
			"https://www.limetorrents.lol",
			"https://www.limetorrents.zone",
			"https://limetorrents.pro",
			"https://www.limetorrents.info", // Original domain as fallback
		},
		SearchPath: "/search/all/",
		Categories: map[string]string{
			"all":    "all",
			"movies": "movies",
			"tv":     "tv",
			"music":  "music",
			"games":  "games",
			"apps":   "applications",
		},
		Enabled:   false,
		RateLimit: 5 * time.Second, // More respectful rate limit
	}
}

func (p *SearchPlugin) loadProvidersFromConfig(config map[string]interface{}) {
	for name, providerConfig := range config {
		if configMap, ok := providerConfig.(map[string]interface{}); ok {
			provider := SearchProviderConfig{
				Name:       name,
				Categories: make(map[string]string),
				Headers:    make(map[string]string),
				Enabled:    true,
				RateLimit:  1 * time.Second,
			}

			if baseURL, ok := configMap["base_url"].(string); ok {
				provider.BaseURL = baseURL
			}

			if searchPath, ok := configMap["search_path"].(string); ok {
				provider.SearchPath = searchPath
			}

			if enabled, ok := configMap["enabled"].(bool); ok {
				provider.Enabled = enabled
			}

			// Parse categories
			if categories, ok := configMap["categories"].(map[string]interface{}); ok {
				for cat, val := range categories {
					if catVal, ok := val.(string); ok {
						provider.Categories[cat] = catVal
					}
				}
			}

			p.providers[name] = provider
		}
	}
}

// tryProviderWithFallback attempts to make a request with fallback URLs
func (p *SearchPlugin) tryProviderWithFallback(provider SearchProviderConfig, searchURL string) (*http.Response, error) {
	// Try primary URL first with detailed logging
	resp, err := p.makeProviderRequest(provider, searchURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		return resp, nil
	}

	// Log the primary URL failure details
	if err != nil {
		if p.context != nil {
			p.context.LogWarn(p.name, fmt.Sprintf("Primary URL failed for %s (%s): %v", provider.Name, provider.BaseURL, err))
		}
	} else if resp != nil {
		if p.context != nil {
			p.context.LogWarn(p.name, fmt.Sprintf("Primary URL returned HTTP %d for %s (%s)", resp.StatusCode, provider.Name, provider.BaseURL))
		}
		resp.Body.Close()
	}

	// Try fallback URLs
	for _, fallbackURL := range provider.FallbackURLs {
		// Replace base URL in search URL
		fallbackSearchURL := strings.Replace(searchURL, provider.BaseURL, fallbackURL, 1)

		if p.context != nil {
			p.context.LogInfo(p.name, fmt.Sprintf("Trying fallback URL for %s: %s", provider.Name, fallbackURL))
		}

		// Add delay to be respectful to Cloudflare
		time.Sleep(2 * time.Second)

		resp, err = p.makeProviderRequest(provider, fallbackSearchURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		// Log fallback failure details
		if err != nil {
			if p.context != nil {
				p.context.LogWarn(p.name, fmt.Sprintf("Fallback URL failed for %s (%s): %v", provider.Name, fallbackURL, err))
			}
		} else if resp != nil {
			if p.context != nil {
				p.context.LogWarn(p.name, fmt.Sprintf("Fallback URL returned HTTP %d for %s (%s)", resp.StatusCode, provider.Name, fallbackURL))
			}
			resp.Body.Close()
		}
	}

	return nil, fmt.Errorf("all URLs failed for provider %s (tried %d URLs)", provider.Name, 1+len(provider.FallbackURLs))
}

// makeProviderRequest makes a single HTTP request to a provider
func (p *SearchPlugin) makeProviderRequest(provider SearchProviderConfig, searchURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	if provider.Name == "1337x" || provider.Name == "LimeTorrents" {
		// Enhanced browser-like headers for sites with Cloudflare protection
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("Cache-Control", "max-age=0")
		req.Header.Set("DNT", "1")
		req.Header.Set("Pragma", "no-cache")

		// Modern Chrome client hints that Cloudflare expects
		req.Header.Set("Sec-CH-UA", "\"Not_A Brand\";v=\"8\", \"Chromium\";v=\"120\", \"Google Chrome\";v=\"120\"")
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
		req.Header.Set("Sec-CH-UA-Platform", "\"Windows\"")
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-User", "?1")

		// Add referer from the base domain
		if provider.BaseURL != "" {
			req.Header.Set("Referer", provider.BaseURL+"/")
		}
	} else {
		req.Header.Set("User-Agent", "Stratum/1.0")
	}
	for key, value := range provider.Headers {
		req.Header.Set(key, value)
	}

	return p.client.Do(req)
}

func (p *SearchPlugin) searchProvider(provider SearchProviderConfig, query string, category string) ([]SearchResult, error) {
	// Build search URL based on provider
	var searchURL string

	switch provider.Name {
	case "The Pirate Bay":
		searchURL = fmt.Sprintf("%s%s?q=%s", provider.BaseURL, provider.SearchPath, url.QueryEscape(query))

	case "YTS":
		searchURL = fmt.Sprintf("%s%s?query_term=%s&limit=50", provider.BaseURL, provider.SearchPath, url.QueryEscape(query))

	case "Nyaa":
		categoryParam := ""
		if category != "" && category != "all" {
			if catValue, exists := provider.Categories[category]; exists {
				categoryParam = catValue
			}
		}
		searchURL = fmt.Sprintf("%s%s%s%s", provider.BaseURL, provider.SearchPath, url.QueryEscape(query), categoryParam)

	case "EZTV":
		searchURL = fmt.Sprintf("%s%s?limit=100", provider.BaseURL, provider.SearchPath)
		if query != "" {
			searchURL += fmt.Sprintf("&search_string=%s", url.QueryEscape(query))
		}

	case "1337x":
		// Format: https://1337x.to/search/QUERY/1/
		searchURL = fmt.Sprintf("%s%s%s/1/", provider.BaseURL, provider.SearchPath, url.QueryEscape(query))

	case "LimeTorrents":
		// Format: https://www.limetorrents.lol/search/all/QUERY/
		categoryPath := "all"
		if category != "" && category != "all" {
			if catValue, exists := provider.Categories[category]; exists {
				categoryPath = catValue
			}
		}
		searchURL = fmt.Sprintf("%s/search/%s/%s/", provider.BaseURL, categoryPath, url.QueryEscape(query))

	default:
		// Generic URL building
		params := url.Values{}
		params.Add("q", query)

		if category != "" && category != "all" {
			if catValue, exists := provider.Categories[category]; exists && catValue != "" {
				params.Add("cat", catValue)
			}
		}
		searchURL = provider.BaseURL + provider.SearchPath + "?" + params.Encode()
	}

	// Make HTTP request with fallback support
	resp, err := p.tryProviderWithFallback(provider, searchURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Parse response based on provider
	switch provider.Name {
	case "The Pirate Bay":
		return p.parsePirateBayResponse(resp, provider.Name)
	case "YTS":
		return p.parseYTSResponse(resp, provider.Name)
	case "EZTV":
		return p.parseEZTVResponse(resp, provider.Name)
	case "Nyaa":
		// Nyaa uses RSS
		return p.parseNyaaRSSResponse(resp, provider.Name)
	case "1337x":
		return p.parse1337xResponse(resp, provider.Name)
	case "LimeTorrents":
		return p.parseLimeTorrentsResponse(resp, provider.Name)
	default:
		return p.parseGenericResponse(resp, provider.Name)
	}
}

func (p *SearchPlugin) parsePirateBayResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	var apiResults []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&apiResults); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	var results []SearchResult
	for _, item := range apiResults {
		result := SearchResult{
			Provider: providerName,
		}

		if name, ok := item["name"].(string); ok {
			result.Name = name
		}

		if infoHash, ok := item["info_hash"].(string); ok {
			result.InfoHash = infoHash
			result.MagnetLink = fmt.Sprintf("magnet:?xt=urn:btih:%s", infoHash)
		}

		if size, ok := item["size"].(string); ok {
			if sizeInt, err := strconv.ParseInt(size, 10, 64); err == nil {
				result.Size = sizeInt
			}
		}

		if seeders, ok := item["seeders"].(string); ok {
			if seedersInt, err := strconv.Atoi(seeders); err == nil {
				result.Seeders = seedersInt
			}
		}

		if leechers, ok := item["leechers"].(string); ok {
			if leechersInt, err := strconv.Atoi(leechers); err == nil {
				result.Leechers = leechersInt
			}
		}

		if added, ok := item["added"].(string); ok {
			result.UploadDate = added
		}

		// Only add results with valid data
		if result.Name != "" && result.InfoHash != "" {
			results = append(results, result)
		}
	}

	return results, nil
}

func (p *SearchPlugin) parseGenericResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	// Try to detect content type
	contentType := resp.Header.Get("Content-Type")

	// If it's RSS/XML, try parsing as RSS feed
	if strings.Contains(contentType, "xml") || strings.Contains(contentType, "rss") {
		return p.parseRSSResponse(resp, providerName)
	}

	// If it's JSON, try parsing as JSON
	if strings.Contains(contentType, "json") {
		return p.parseJSONResponse(resp, providerName)
	}

	// For HTML or unknown types, return empty for now
	// In a production implementation, you'd want to use a proper HTML parser
	if p.context != nil {
		p.context.LogWarn(p.name, fmt.Sprintf("Unsupported content type for provider %s: %s", providerName, contentType))
	}
	return []SearchResult{}, nil
}

// parseRSSResponse parses RSS feed responses (used by many torrent sites)
func (p *SearchPlugin) parseRSSResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	// This is a simplified RSS parser
	// In production, you'd use encoding/xml with proper struct definitions
	var results []SearchResult

	// For now, return empty results with a log message
	if p.context != nil {
		p.context.LogInfo(p.name, fmt.Sprintf("RSS parsing not fully implemented for %s", providerName))
	}
	return results, nil
}

// parseJSONResponse parses JSON API responses
func (p *SearchPlugin) parseJSONResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	var apiResults []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&apiResults); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	var results []SearchResult
	for _, item := range apiResults {
		result := SearchResult{
			Provider: providerName,
		}

		// Try to extract common fields
		if name, ok := item["name"].(string); ok {
			result.Name = name
		} else if title, ok := item["title"].(string); ok {
			result.Name = title
		}

		if infoHash, ok := item["info_hash"].(string); ok {
			result.InfoHash = infoHash
			result.MagnetLink = fmt.Sprintf("magnet:?xt=urn:btih:%s", infoHash)
		} else if magnet, ok := item["magnet"].(string); ok {
			result.MagnetLink = magnet
		}

		if size, ok := item["size"].(float64); ok {
			result.Size = int64(size)
		}

		if seeders, ok := item["seeders"].(float64); ok {
			result.Seeders = int(seeders)
		}

		if leechers, ok := item["leechers"].(float64); ok {
			result.Leechers = int(leechers)
		}

		if category, ok := item["category"].(string); ok {
			result.Category = category
		}

		if date, ok := item["upload_date"].(string); ok {
			result.UploadDate = date
		} else if added, ok := item["added"].(string); ok {
			result.UploadDate = added
		}

		// Only add results with valid data
		if result.Name != "" && (result.InfoHash != "" || result.MagnetLink != "") {
			results = append(results, result)
		}
	}

	return results, nil
}

// parseYTSResponse parses YTS movie API responses
func (p *SearchPlugin) parseYTSResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	var apiResponse struct {
		Status string `json:"status"`
		Data   struct {
			Movies []struct {
				Title    string  `json:"title"`
				Year     int     `json:"year"`
				Rating   float64 `json:"rating"`
				Summary  string  `json:"summary"`
				Torrents []struct {
					Hash    string `json:"hash"`
					Quality string `json:"quality"`
					Type    string `json:"type"`
					Seeds   int    `json:"seeds"`
					Peers   int    `json:"peers"`
					Size    string `json:"size"`
				} `json:"torrents"`
			} `json:"movies"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode YTS JSON: %w", err)
	}

	var results []SearchResult
	for _, movie := range apiResponse.Data.Movies {
		for _, torrent := range movie.Torrents {
			result := SearchResult{
				Name:        fmt.Sprintf("%s (%d) [%s] [%s]", movie.Title, movie.Year, torrent.Quality, torrent.Type),
				InfoHash:    torrent.Hash,
				MagnetLink:  fmt.Sprintf("magnet:?xt=urn:btih:%s", torrent.Hash),
				Seeders:     torrent.Seeds,
				Leechers:    torrent.Peers,
				Category:    "movies",
				Provider:    providerName,
				Description: movie.Summary,
			}

			// Parse size (e.g., "1.5 GB")
			sizeStr := strings.Replace(torrent.Size, " ", "", -1)
			if strings.Contains(strings.ToUpper(sizeStr), "GB") {
				sizeVal, _ := strconv.ParseFloat(strings.TrimSuffix(strings.ToUpper(sizeStr), "GB"), 64)
				result.Size = int64(sizeVal * 1024 * 1024 * 1024)
			} else if strings.Contains(strings.ToUpper(sizeStr), "MB") {
				sizeVal, _ := strconv.ParseFloat(strings.TrimSuffix(strings.ToUpper(sizeStr), "MB"), 64)
				result.Size = int64(sizeVal * 1024 * 1024)
			}

			results = append(results, result)
		}
	}

	return results, nil
}

// parseEZTVResponse parses EZTV TV show API responses
func (p *SearchPlugin) parseEZTVResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	var apiResponse struct {
		Torrents []struct {
			ID           int         `json:"id"`
			Hash         string      `json:"hash"`
			Filename     string      `json:"filename"`
			Title        string      `json:"title"`
			SizeBytes    string      `json:"size_bytes"`
			Seeds        int         `json:"seeds"`
			Peers        int         `json:"peers"`
			DateReleased interface{} `json:"date_released_unix"` // Can be string or number
		} `json:"torrents"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode EZTV JSON: %w", err)
	}

	var results []SearchResult
	for _, torrent := range apiResponse.Torrents {
		if torrent.Hash == "" {
			continue
		}

		result := SearchResult{
			Name:       torrent.Title,
			InfoHash:   torrent.Hash,
			MagnetLink: fmt.Sprintf("magnet:?xt=urn:btih:%s", torrent.Hash),
			Seeders:    torrent.Seeds,
			Leechers:   torrent.Peers,
			Category:   "tv",
			Provider:   providerName,
		}

		// Parse size
		if sizeBytes, err := strconv.ParseInt(torrent.SizeBytes, 10, 64); err == nil {
			result.Size = sizeBytes
		}

		results = append(results, result)
	}

	return results, nil
}

// parseNyaaRSSResponse parses Nyaa RSS feed responses
func (p *SearchPlugin) parseNyaaRSSResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	// Simple RSS parsing - in production use encoding/xml
	// For now, do basic string parsing of the RSS feed
	var results []SearchResult

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read RSS body: %w", err)
	}

	bodyStr := string(body)

	// Split by <item> tags
	items := strings.Split(bodyStr, "<item>")
	for i, item := range items {
		if i == 0 {
			continue // Skip the header
		}

		result := SearchResult{
			Provider: providerName,
			Category: "anime",
		}

		// Extract title
		if titleStart := strings.Index(item, "<title>"); titleStart >= 0 {
			titleStart += 7 // len("<title>")
			if titleEnd := strings.Index(item[titleStart:], "</title>"); titleEnd >= 0 {
				result.Name = strings.TrimSpace(item[titleStart : titleStart+titleEnd])
			}
		}

		// Extract magnet link
		if linkStart := strings.Index(item, "magnet:?xt=urn:btih:"); linkStart >= 0 {
			if linkEnd := strings.Index(item[linkStart:], "\""); linkEnd >= 0 {
				magnetLink := item[linkStart : linkStart+linkEnd]
				result.MagnetLink = magnetLink

				// Extract hash from magnet
				if hashStart := strings.Index(magnetLink, "btih:"); hashStart >= 0 {
					hashStart += 5
					hash := magnetLink[hashStart:]
					if ampIdx := strings.Index(hash, "&"); ampIdx >= 0 {
						hash = hash[:ampIdx]
					}
					result.InfoHash = strings.ToLower(hash)
				}
			}
		}

		// Extract seeders
		if seedStart := strings.Index(item, "<nyaa:seeders>"); seedStart >= 0 {
			seedStart += 14
			if seedEnd := strings.Index(item[seedStart:], "</nyaa:seeders>"); seedEnd >= 0 {
				if seeds, err := strconv.Atoi(strings.TrimSpace(item[seedStart : seedStart+seedEnd])); err == nil {
					result.Seeders = seeds
				}
			}
		}

		// Extract leechers
		if leechStart := strings.Index(item, "<nyaa:leechers>"); leechStart >= 0 {
			leechStart += 15
			if leechEnd := strings.Index(item[leechStart:], "</nyaa:leechers>"); leechEnd >= 0 {
				if leechers, err := strconv.Atoi(strings.TrimSpace(item[leechStart : leechStart+leechEnd])); err == nil {
					result.Leechers = leechers
				}
			}
		}

		// Extract size
		if sizeStart := strings.Index(item, "<nyaa:size>"); sizeStart >= 0 {
			sizeStart += 11
			if sizeEnd := strings.Index(item[sizeStart:], "</nyaa:size>"); sizeEnd >= 0 {
				sizeStr := strings.TrimSpace(item[sizeStart : sizeStart+sizeEnd])
				// Parse size like "545.3 MiB"
				parts := strings.Fields(sizeStr)
				if len(parts) == 2 {
					if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
						unit := strings.ToUpper(parts[1])
						if strings.Contains(unit, "GIB") || strings.Contains(unit, "GB") {
							result.Size = int64(val * 1024 * 1024 * 1024)
						} else if strings.Contains(unit, "MIB") || strings.Contains(unit, "MB") {
							result.Size = int64(val * 1024 * 1024)
						}
					}
				}
			}
		}

		// Only add if we have valid data
		if result.Name != "" && result.MagnetLink != "" {
			results = append(results, result)
		}
	}

	return results, nil
}

func (p *SearchPlugin) deduplicateResults(results []SearchResult) []SearchResult {
	seen := make(map[string]bool)
	var unique []SearchResult

	for _, result := range results {
		// Use InfoHash as the key for deduplication
		key := result.InfoHash
		if key == "" {
			// If no InfoHash, use name as fallback
			key = result.Name
		}

		if !seen[key] {
			seen[key] = true
			unique = append(unique, result)
		}
	}

	return unique
}

// GetProviders returns the list of available providers
func (p *SearchPlugin) GetProviders() []SearchProviderConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()

	providers := make([]SearchProviderConfig, 0, len(p.providers))
	for _, provider := range p.providers {
		providers = append(providers, provider)
	}

	return providers
}

// EnableProvider enables a search provider
func (p *SearchPlugin) EnableProvider(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	provider, exists := p.providers[name]
	if !exists {
		return fmt.Errorf("provider %s not found", name)
	}

	provider.Enabled = true
	p.providers[name] = provider

	// Add to enabled providers list if not already there
	for _, enabled := range p.enabledProviders {
		if enabled == name {
			return nil
		}
	}

	p.enabledProviders = append(p.enabledProviders, name)
	return nil
}

// DisableProvider disables a search provider
func (p *SearchPlugin) DisableProvider(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	provider, exists := p.providers[name]
	if !exists {
		return fmt.Errorf("provider %s not found", name)
	}

	provider.Enabled = false
	p.providers[name] = provider

	// Remove from enabled providers list
	for i, enabled := range p.enabledProviders {
		if enabled == name {
			p.enabledProviders = append(p.enabledProviders[:i], p.enabledProviders[i+1:]...)
			break
		}
	}

	return nil
}

// parse1337xResponse scrapes 1337x HTML search results and detail pages
func (p *SearchPlugin) parse1337xResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	html := string(body)
	parser := NewHTMLParser(p.client)

	var results []SearchResult

	// Parse search results table
	// 1337x format: <tr> rows in search results table
	rows := parser.ExtractAllBetween(html, "<tr", "</tr>")

	for _, row := range rows {
		// Skip header row
		if strings.Contains(row, "<th") {
			continue
		}

		// Extract torrent detail page link
		detailLinkMatch := extractBetween(row, `href="/torrent/`, `"`)
		if detailLinkMatch == "" {
			detailLinkMatch = extractBetween(row, `href='/torrent/`, `'`)
		}
		if detailLinkMatch == "" {
			continue
		}

		detailURL := "https://1337x.to/torrent/" + detailLinkMatch

		// Extract title
		titleHTML := extractBetween(row, `<td class="name">`, `</td>`)
		if titleHTML == "" {
			titleHTML = extractBetween(row, `<td class="coll-1 name">`, `</td>`)
		}
		title := parser.StripHTMLTags(titleHTML)

		if title == "" {
			continue
		}

		// Extract seeders and leechers from the row
		seedersStr := extractBetween(row, `<td class="coll-2 seeds">`, `</td>`)
		if seedersStr == "" {
			seedersStr = extractBetween(row, `<td class="seeds">`, `</td>`)
		}
		seeders, _ := strconv.Atoi(strings.TrimSpace(parser.StripHTMLTags(seedersStr)))

		leechersStr := extractBetween(row, `<td class="coll-3 leeches">`, `</td>`)
		if leechersStr == "" {
			leechersStr = extractBetween(row, `<td class="leeches">`, `</td>`)
		}
		leechers, _ := strconv.Atoi(strings.TrimSpace(parser.StripHTMLTags(leechersStr)))

		// Extract size
		sizeStr := extractBetween(row, `<td class="coll-4 size">`, `</td>`)
		if sizeStr == "" {
			sizeStr = extractBetween(row, `<td class="size">`, `</td>`)
		}
		size := parser.ParseFileSize(parser.StripHTMLTags(sizeStr))

		// Now scrape the detail page for magnet and .torrent links
		result := SearchResult{
			Name:     title,
			Provider: providerName,
			Seeders:  seeders,
			Leechers: leechers,
			Size:     size,
			Category: "all",
		}

		// Fetch detail page (with error handling)
		if detailHTML, err := parser.FetchHTML(detailURL); err == nil {
			// Extract magnet link
			magnetLink := parser.ExtractMagnetLink(detailHTML)
			if magnetLink != "" {
				result.MagnetLink = magnetLink

				// Extract info hash from magnet
				if hashIdx := strings.Index(magnetLink, "btih:"); hashIdx >= 0 {
					hash := magnetLink[hashIdx+5:]
					if ampIdx := strings.Index(hash, "&"); ampIdx >= 0 {
						hash = hash[:ampIdx]
					}
					result.InfoHash = strings.ToLower(hash)
				}
			}

			// Extract .torrent download link
			torrentDownload := parser.ExtractTorrentDownloadLink(detailHTML, "https://1337x.to")
			if torrentDownload != "" {
				result.TorrentURL = torrentDownload
			}
		}

		// Only add if we have at least a magnet link or torrent URL
		if result.MagnetLink != "" || result.TorrentURL != "" {
			results = append(results, result)
		}
	}

	return results, nil
}

// parseLimeTorrentsResponse scrapes LimeTorrents HTML
func (p *SearchPlugin) parseLimeTorrentsResponse(resp *http.Response, providerName string) ([]SearchResult, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	html := string(body)
	parser := NewHTMLParser(p.client)

	var results []SearchResult

	// LimeTorrents uses table rows for results
	rows := parser.ExtractAllBetween(html, "<tr", "</tr>")

	for _, row := range rows {
		// Skip non-data rows
		if !strings.Contains(row, "tt-name") {
			continue
		}

		// Extract title and detail link
		titleSection := extractBetween(row, `<div class="tt-name">`, `</div>`)
		detailLink := extractBetween(titleSection, `href="`, `"`)
		title := parser.StripHTMLTags(titleSection)

		if title == "" || detailLink == "" {
			continue
		}

		// Build full URL using the actual provider base URL
		if !strings.HasPrefix(detailLink, "http") {
			detailLink = "https://www.limetorrents.fun" + detailLink
		}

		// Extract stats
		seedsStr := extractBetween(row, `<td class="tdseed">`, `</td>`)
		seeds, _ := strconv.Atoi(strings.TrimSpace(parser.StripHTMLTags(seedsStr)))

		leechStr := extractBetween(row, `<td class="tdleech">`, `</td>`)
		leeches, _ := strconv.Atoi(strings.TrimSpace(parser.StripHTMLTags(leechStr)))

		sizeStr := extractBetween(row, `<td class="tdnormal">`, `</td>`)
		size := parser.ParseFileSize(parser.StripHTMLTags(sizeStr))

		result := SearchResult{
			Name:     title,
			Provider: providerName,
			Seeders:  seeds,
			Leechers: leeches,
			Size:     size,
			Category: "all",
		}

		// Fetch detail page
		if detailHTML, err := parser.FetchHTML(detailLink); err == nil {
			// Extract magnet
			result.MagnetLink = parser.ExtractMagnetLink(detailHTML)

			// Extract info hash
			if result.MagnetLink != "" {
				if hashIdx := strings.Index(result.MagnetLink, "btih:"); hashIdx >= 0 {
					hash := result.MagnetLink[hashIdx+5:]
					if ampIdx := strings.Index(hash, "&"); ampIdx >= 0 {
						hash = hash[:ampIdx]
					}
					result.InfoHash = strings.ToLower(hash)
				}
			}

			// Extract .torrent link using the actual provider base URL
			result.TorrentURL = parser.ExtractTorrentDownloadLink(detailHTML, "https://www.limetorrents.fun")
		}

		if result.MagnetLink != "" || result.TorrentURL != "" {
			results = append(results, result)
		}
	}

	return results, nil
}

// Helper function to extract text between two strings
func extractBetween(text, start, end string) string {
	startIdx := strings.Index(text, start)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(start)

	endIdx := strings.Index(text[startIdx:], end)
	if endIdx == -1 {
		return ""
	}

	return text[startIdx : startIdx+endIdx]
}
