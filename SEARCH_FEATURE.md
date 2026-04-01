# Stratum Torrent Search - Complete Guide

## Overview

Stratum now includes a **powerful multi-provider torrent search** that searches across all major torrent sites simultaneously and presents results in a beautiful, easy-to-use interface.

## Supported Search Providers

### ✅ Fully Implemented (Working)

1. **The Pirate Bay** - General torrents (movies, TV, software, games, etc.)
2. **YTS** - High-quality movie torrents (720p, 1080p, 4K)
3. **EZTV** - TV show episodes and series
4. **Nyaa** - Anime, manga, and Japanese media

### 🔧 Configurable

5. **TorrentGalaxy** - Can be enabled via configuration
6. **1337x** - Available but needs HTML parsing (disabled by default)

## Features

### 🔍 Search Capabilities
- **Multi-provider search** - Searches all enabled providers simultaneously
- **Category filtering** - Filter by: All, Movies, TV Shows, Anime, Music, Games, Apps
- **Results aggregation** - Combines and deduplicates results from all providers
- **Real-time results** - Shows results as they come in

### 📊 Result Information
Each search result shows:
- ✅ Torrent name and description
- 📦 File size
- 🌱 Seeders count (in green)
- 📥 Leechers count (in red)
- 🏷️ Provider name
- 🎯 Category badge

### ⚡ One-Click Actions
- **Download Button** - Instantly add torrent to your downloads
- **Copy Magnet Link** - One-click magnet link copy to clipboard
- Beautiful loading states while adding downloads

## How to Use

### Via Web UI (Easiest)

1. Start Stratum: `./scripts/dev.sh`
2. Open browser: `http://localhost:3000`
3. Click **"Search"** in the sidebar (second item)
4. Enter your search query
5. Select category (optional)
6. Click "Search" or press Enter
7. Click **"Download"** on any result to add it instantly!

### Via API

```bash
# Search for torrents
curl -X POST http://127.0.0.1:8080/api/plugins/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "ubuntu 24.04",
    "category": "apps"
  }'
```

**Response Example**:
```json
{
  "results": [
    {
      "name": "Ubuntu 24.04 LTS Desktop ISO",
      "info_hash": "abc123...",
      "magnet_link": "magnet:?xt=urn:btih:abc123...",
      "size": 4500000000,
      "seeders": 250,
      "leechers": 45,
      "category": "apps",
      "provider": "The Pirate Bay"
    }
  ],
  "query": "ubuntu 24.04",
  "category": "apps"
}
```

## Search Examples

### Finding Movies
```
Query: "dune part two"
Category: Movies
Providers: YTS, The Pirate Bay
```

### Finding TV Shows
```
Query: "breaking bad s01"
Category: TV Shows
Providers: EZTV, The Pirate Bay
```

### Finding Anime
```
Query: "demon slayer"
Category: Anime
Providers: Nyaa
```

### Finding Software
```
Query: "ubuntu"
Category: Apps/Software
Providers: The Pirate Bay
```

## Provider Details

### The Pirate Bay (TPB)
- **Type**: Public API
- **Categories**: All
- **Rate Limit**: 1 request/second
- **Best For**: General purpose, all content types

### YTS
- **Type**: JSON API
- **Categories**: Movies only
- **Rate Limit**: 1 request/second
- **Best For**: High-quality movie encodes
- **Special**: Shows movie year, rating, quality (720p/1080p/4K)

### EZTV
- **Type**: JSON API
- **Categories**: TV shows only
- **Rate Limit**: 1 request/second
- **Best For**: TV show episodes, latest releases

### Nyaa
- **Type**: RSS Feed
- **Categories**: Anime, music, literature, software
- **Rate Limit**: 1 request/second
- **Best For**: Anime and Japanese media

## Configuration

### Enable/Disable Providers

Edit plugin configuration via API:

```bash
curl -X PUT http://127.0.0.1:8080/api/plugins/search/config \
  -H "Content-Type: application/json" \
  -d '{
    "enabled_providers": ["the-pirate-bay", "yts", "eztv", "nyaa"],
    "max_results": 100,
    "timeout": 30
  }'
```

### Add Custom Provider

```bash
curl -X PUT http://127.0.0.1:8080/api/plugins/search/config \
  -H "Content-Type: application/json" \
  -d '{
    "providers": {
      "my-custom-tracker": {
        "base_url": "https://mytracker.com",
        "search_path": "/api/search",
        "categories": {
          "movies": "1",
          "tv": "2"
        },
        "enabled": true
      }
    }
  }'
```

## UI Features

### Responsive Design
- ✨ Modern, clean interface
- 📱 Mobile-friendly
- 🌙 Dark theme optimized
- ⚡ Smooth animations

### Search Bar
- 🔍 Auto-focus on load
- ⌨️ Enter key support
- 📋 Category dropdown
- 🚀 Loading state indicator

### Results Display
- 📊 Card-based layout
- 🎨 Provider-specific colors
- ✅ Seeder/leecher visualization
- 📦 Human-readable file sizes
- 🏷️ Category badges

### Action Buttons
- 💚 **Download**: Emerald green, adds to downloads
- 💙 **Copy Magnet**: Blue, copies to clipboard
- ⏳ Loading spinners during operations

## Technical Implementation

### Backend
- **Language**: Go
- **Location**: `internal/plugins/search.go`
- **Architecture**: Plugin-based
- **Concurrency**: Searches all providers in parallel
- **Deduplication**: Automatic by info hash

### Frontend
- **Framework**: Next.js 16 + React 19
- **Location**: `frontend/src/app/search/page.tsx`
- **Styling**: Tailwind CSS
- **Icons**: Lucide React

## Troubleshooting

### No Results
- Check that search plugin is running: Visit `/plugins` page
- Verify internet connectivity
- Try different search terms
- Try "all" category instead of specific category

### Slow Searches
- Provider may be rate-limiting
- Network connectivity issues
- Reduce `max_results` in configuration

### Provider Not Working
- Check provider status in plugin configuration
- Some providers may be geo-restricted
- Provider API may have changed (check logs)

## Future Enhancements

Planned features:
- [ ] Filter by minimum seeders
- [ ] Sort by seeders, size, date
- [ ] Advanced filters (file type, quality)
- [ ] Search history
- [ ] Favorite searches
- [ ] More providers (RARBG alternatives, LimeTorrents, etc.)
- [ ] Torrent previews/screenshots
- [ ] IMDb/TMDB integration for movies

## API Endpoints

### Search
`POST /api/plugins/search`
```json
{
  "query": "search term",
  "category": "all|movies|tv|anime|music|games|apps"
}
```

### Get Plugin Status
`GET /api/plugins/search/status`

### Configure Search
`PUT /api/plugins/search/config`

## Performance

- **Average search time**: 2-5 seconds
- **Providers searched**: 4-6 simultaneously
- **Typical results**: 50-200 torrents
- **Deduplication**: Automatic
- **Caching**: Currently none (real-time)

## Safety & Privacy

- ⚠️ No tracking or logging of searches
- 🔒 Searches go directly to public APIs
- 🛡️ No torrent content is hosted
- 📡 Only metadata is retrieved
- 🔐 Magnet links only (no direct file hosting)

## Keyboard Shortcuts

- `Enter` - Execute search
- `Cmd/Ctrl + K` - Focus search bar (planned)

---

**Enjoy fast, easy torrent searching with Stratum!** 🚀
