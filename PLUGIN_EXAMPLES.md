# Stratum Plugin System Examples

## Using the Streaming Server Plugin

The Streaming Server plugin allows you to watch or listen to downloaded files in real-time while they're still being downloaded.

### Starting a Stream

```bash
curl -X POST http://127.0.0.1:8080/api/plugins/stream \
  -H "Content-Type: application/json" \
  -d '{
    "info_hash": "abc123...",
    "file_path": "/path/to/downloads/movie.mp4"
  }'
```

**Response**:
```json
{
  "stream_url": "http://127.0.0.1:8081/stream/abc123..._1709251200000",
  "info_hash": "abc123...",
  "file_path": "/path/to/downloads/movie.mp4"
}
```

### Watching the Stream

Once you have the stream URL, you can:
- Play it in VLC: `vlc "http://127.0.0.1:8081/stream/abc123..._1709251200000"`
- Play it in a web browser
- Stream it to another device using the URL

### Supported Features

- **HTTP Range Requests**: Seek to any position in the file
- **MIME Type Detection**: Automatically detects video, audio, and document types
- **Multiple Streams**: Can stream multiple files concurrently (default: 10)
- **Automatic Cleanup**: Inactive streams are cleaned up after 30 minutes

### Configuration

```bash
curl -X PUT http://127.0.0.1:8080/api/plugins/streaming-server/config \
  -H "Content-Type: application/json" \
  -d '{
    "port": 8081,
    "bind_addr": "0.0.0.0",
    "max_streams": 20
  }'
```

## Using the Search Plugin

The Search plugin enables searching for torrents across multiple providers.

### Searching for Torrents

```bash
curl -X POST http://127.0.0.1:8080/api/plugins/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "linux ubuntu",
    "category": "all"
  }'
```

**Response**:
```json
{
  "results": [
    {
      "name": "Ubuntu 24.04 LTS",
      "info_hash": "da39a3ee5e6b4b0d3255bfef95601890afd80709",
      "magnet_link": "magnet:?xt=urn:btih:da39a3ee5e6b4b0d3255bfef95601890afd80709",
      "size": 4000000000,
      "seeders": 150,
      "leechers": 50,
      "category": "linux",
      "provider": "The Pirate Bay",
      "upload_date": "2024-01-15"
    }
  ],
  "query": "linux ubuntu",
  "category": "all"
}
```

### Available Categories (Provider Default)

**The Pirate Bay**:
- `all`, `video`, `audio`, `apps`, `games`, `other`

**Nyaa** (Anime):
- `all`, `anime`, `audio`, `literature`, `software`

**1337x**:
- `all`, `movies`, `tv`, `games`, `music`, `apps`

### Configuring Search Providers

```bash
curl -X PUT http://127.0.0.1:8080/api/plugins/search/config \
  -H "Content-Type: application/json" \
  -d '{
    "timeout": 30,
    "max_results": 50,
    "enabled_providers": ["the-pirate-bay", "nyaa"],
    "providers": {
      "custom-tracker": {
        "base_url": "https://custom.tracker.com",
        "search_path": "/search",
        "categories": {
          "movies": "movie",
          "tv": "tv"
        }
      }
    }
  }'
```

### Adding a Magnet Link from Search Results

Once you have a `magnet_link` from the search results, you can start a download:

```bash
curl -X POST http://127.0.0.1:8080/api/start \
  -H "Content-Type: application/json" \
  -d '{
    "source": "magnet:?xt=urn:btih:da39a3ee5e6b4b0d3255bfef95601890afd80709"
  }'
```

## Managing Plugins via the Web UI

### Plugins Page
Navigate to `http://localhost:3000/plugins` to see the Plugins dashboard.

Features:
- View all installed plugins and their status
- See plugin version and capabilities
- Start/stop/restart plugins
- View plugin health status
- Check for any error messages

### Real-time Status Updates
The plugins page auto-refreshes every 5 seconds to show the latest plugin status.

### Status Indicators
- **green (running)**: Plugin is active and healthy
- **yellow (stopped)**: Plugin is registered but not running
- **red (error)**: Plugin encountered an error

## Streaming a Downloaded Movie with VLC

### Step 1: Start a Torrent Download

```bash
./stratum "magnet:?xt=urn:btih:..." -o ~/Downloads
```

### Step 2: Check Available Streams

```bash
curl http://127.0.0.1:8080/api/plugins/stream \
  -H "Accept: application/json"
```

### Step 3: Start a Stream for Your Movie

```bash
curl -X POST http://127.0.0.1:8080/api/plugins/stream \
  -H "Content-Type: application/json" \
  -d '{
    "info_hash": "<the-torrent-info-hash>",
    "file_path": "/home/user/Downloads/Downloads/movie.mp4"
  }'
```

### Step 4: Open in VLC

Copy the `stream_url` from the response and open it in VLC:

```bash
vlc "http://127.0.0.1:8081/stream/..."
```

You can now watch the movie while it's being downloaded!

## Searching and Downloading in Workflow

### Complete Workflow Example

```bash
# 1. Search for a movie
SEARCH_RESULT=$(curl -s -X POST http://127.0.0.1:8080/api/plugins/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "dune part two",
    "category": "movies"
  }')

# 2. Extract the magnet link from the first result
MAGNET=$(echo $SEARCH_RESULT | jq -r '.results[0].magnet_link')

# 3. Start the download
curl -X POST http://127.0.0.1:8080/api/start \
  -H "Content-Type: application/json" \
  -d "{\"source\": \"$MAGNET\"}"

# 4. Get the torrent info hash from the torrent state
HASH=$(curl -s http://127.0.0.1:8080/api/state | jq -r '.torrents[0].info_hash')

# 5. Start streaming the file (once it has downloaded enough)
sleep 10  # Wait for some chunks to download

STREAM=$(curl -s -X POST http://127.0.0.1:8080/api/plugins/stream \
  -H "Content-Type: application/json" \
  -d "{
    \"info_hash\": \"$HASH\",
    \"file_path\": \"/path/to/dune.mp4\"
  }")

# 6. Get the stream URL and open in player
STREAM_URL=$(echo $STREAM | jq -r '.stream_url')
echo "Opening: $STREAM_URL"
vlc "$STREAM_URL"
```

## Client Libraries

### Python Example

```python
import requests
import json

BASE_URL = "http://127.0.0.1:8080"

def search_torrents(query, category="all"):
    """Search for torrents"""
    response = requests.post(
        f"{BASE_URL}/api/plugins/search",
        json={"query": query, "category": category}
    )
    return response.json()

def start_stream(info_hash, file_path):
    """Start streaming a file"""
    response = requests.post(
        f"{BASE_URL}/api/plugins/stream",
        json={"info_hash": info_hash, "file_path": file_path}
    )
    return response.json()

def get_plugin_status(plugin_name):
    """Get plugin status"""
    response = requests.get(f"{BASE_URL}/api/plugins/{plugin_name}/status")
    return response.json()

# Example usage
if __name__ == "__main__":
    # Search for torrents
    results = search_torrents("ubuntu 24.04", "all")
    print("Found torrents:", results)
    
    # Get plugin status
    status = get_plugin_status("streaming-server")
    print("Streaming plugin status:", status)
```

### JavaScript/Node.js Example

```javascript
const BASE_URL = "http://127.0.0.1:8080";

async function searchTorrents(query, category = "all") {
  const response = await fetch(`${BASE_URL}/api/plugins/search`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, category })
  });
  return response.json();
}

async function startStream(infoHash, filePath) {
  const response = await fetch(`${BASE_URL}/api/plugins/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ info_hash: infoHash, file_path: filePath })
  });
  return response.json();
}

async function getPluginStatus(pluginName) {
  const response = await fetch(`${BASE_URL}/api/plugins/${pluginName}/status`);
  return response.json();
}

// Example usage
(async () => {
  const results = await searchTorrents("ubuntu 24.04");
  console.log("Search results:", results);
  
  const status = await getPluginStatus("streaming-server");
  console.log("Streaming plugin status:", status);
})();
```

## Troubleshooting

### "Stream not found" error
- Ensure the file path is correct and accessible
- Check that the file has downloaded enough content for streaming

### Search returns no results
- Check that search providers are enabled
- Verify internet connectivity
- Try with different search terms or categories

### Streaming fails with "Connection refused"
- Check that the streaming server plugin is running
- Verify the port (default 8081) is not in use
- Check firewall settings
