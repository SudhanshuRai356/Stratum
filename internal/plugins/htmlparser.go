package plugins

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// HTMLParser provides utilities for parsing HTML content
type HTMLParser struct {
	client *http.Client
}

// NewHTMLParser creates a new HTML parser
func NewHTMLParser(client *http.Client) *HTMLParser {
	return &HTMLParser{client: client}
}

// FetchHTML fetches HTML content from a URL
func (p *HTMLParser) FetchHTML(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Set realistic browser headers to avoid blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "max-age=0")
	
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// ExtractLinks extracts all href links from HTML
func (p *HTMLParser) ExtractLinks(html string) []string {
	re := regexp.MustCompile(`href="([^"]+)"`)
	matches := re.FindAllStringSubmatch(html, -1)
	
	var links []string
	for _, match := range matches {
		if len(match) > 1 {
			links = append(links, match[1])
		}
	}
	
	return links
}

// ExtractMagnetLink finds magnet link in HTML
func (p *HTMLParser) ExtractMagnetLink(html string) string {
	// Look for magnet: links
	re := regexp.MustCompile(`magnet:\?xt=urn:btih:([a-fA-F0-9]{40}|[a-zA-Z0-9]{32})([^"'\s]*)`)
	match := re.FindString(html)
	return match
}

// ExtractTorrentDownloadLink finds .torrent download link
func (p *HTMLParser) ExtractTorrentDownloadLink(html string, baseURL string) string {
	// Look for .torrent file links
	re := regexp.MustCompile(`href="([^"]*\.torrent[^"]*)"`)
	match := re.FindStringSubmatch(html)
	
	if len(match) > 1 {
		link := match[1]
		if strings.HasPrefix(link, "http") {
			return link
		}
		if strings.HasPrefix(link, "//") {
			return "https:" + link
		}
		if strings.HasPrefix(link, "/") {
			return baseURL + link
		}
		return baseURL + "/" + link
	}
	
	return ""
}

// ExtractText extracts text between HTML tags
func (p *HTMLParser) ExtractText(html, startTag, endTag string) string {
	startIdx := strings.Index(html, startTag)
	if startIdx == -1 {
		return ""
	}
	
	startIdx += len(startTag)
	endIdx := strings.Index(html[startIdx:], endTag)
	if endIdx == -1 {
		return ""
	}
	
	return strings.TrimSpace(html[startIdx : startIdx+endIdx])
}

// ExtractAllBetween extracts all text occurrences between tags
func (p *HTMLParser) ExtractAllBetween(html, startTag, endTag string) []string {
	var results []string
	searchHTML := html
	
	for {
		startIdx := strings.Index(searchHTML, startTag)
		if startIdx == -1 {
			break
		}
		
		startIdx += len(startTag)
		endIdx := strings.Index(searchHTML[startIdx:], endTag)
		if endIdx == -1 {
			break
		}
		
		text := strings.TrimSpace(searchHTML[startIdx : startIdx+endIdx])
		if text != "" {
			results = append(results, text)
		}
		
		searchHTML = searchHTML[startIdx+endIdx+len(endTag):]
	}
	
	return results
}

// ParseTableRow parses a table row into cells
func (p *HTMLParser) ParseTableRow(rowHTML string) []string {
	return p.ExtractAllBetween(rowHTML, "<td", "</td>")
}

// StripHTMLTags removes all HTML tags from text
func (p *HTMLParser) StripHTMLTags(html string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return strings.TrimSpace(re.ReplaceAllString(html, ""))
}

// ExtractNumbers extracts all numbers from text
func (p *HTMLParser) ExtractNumbers(text string) []string {
	re := regexp.MustCompile(`\d+`)
	return re.FindAllString(text, -1)
}

// ParseFileSize converts size string to bytes
func (p *HTMLParser) ParseFileSize(sizeStr string) int64 {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))
	
	re := regexp.MustCompile(`([\d.]+)\s*([KMGT]?I?B)`)
	match := re.FindStringSubmatch(sizeStr)
	
	if len(match) < 3 {
		return 0
	}
	
	var value float64
	fmt.Sscanf(match[1], "%f", &value)
	unit := match[2]
	
	multiplier := int64(1)
	switch {
	case strings.Contains(unit, "K"):
		multiplier = 1024
	case strings.Contains(unit, "M"):
		multiplier = 1024 * 1024
	case strings.Contains(unit, "G"):
		multiplier = 1024 * 1024 * 1024
	case strings.Contains(unit, "T"):
		multiplier = 1024 * 1024 * 1024 * 1024
	}
	
	return int64(value * float64(multiplier))
}
