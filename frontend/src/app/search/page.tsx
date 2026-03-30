"use client";

import { useState, useCallback, useEffect } from "react";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";
import { Search as SearchIcon, Download, Link as LinkIcon, PlayCircle } from "lucide-react";

type SearchResult = {
  name: string;
  info_hash: string;
  magnet_link: string;
  torrent_url?: string;
  size: number;
  seeders: number;
  leechers: number;
  category: string;
  provider: string;
  upload_date?: string;
  description?: string;
};

const CATEGORIES = [
  { value: "all", label: "All" },
  { value: "movies", label: "Movies" },
  { value: "tv", label: "TV Shows" },
  { value: "anime", label: "Anime" },
  { value: "music", label: "Music" },
  { value: "games", label: "Games" },
  { value: "apps", label: "Apps/Software" },
];

function formatBytes(bytes: number): string {
  if (bytes === 0) return "Unknown";
  const k = 1024;
  const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + " " + sizes[i];
}

export default function SearchPage() {
  const { notifications } = useDashboardData();
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("all");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [apiToken, setApiToken] = useState<string | null>(null);
  const [downloadingHashes, setDownloadingHashes] = useState<Set<string>>(new Set());

  // Fetch API token
  useEffect(() => {
    fetch("http://127.0.0.1:8080/api/ws-auth")
      .then((res) => res.json())
      .then((data) => {
        if (data.token) {
          setApiToken(data.token);
        }
      })
      .catch(() => {
        // Token not available
      });
  }, []);

  const handleSearch = useCallback(async () => {
    if (!query.trim() || !apiToken) return;

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `http://127.0.0.1:8080/api/plugins/search?token=${apiToken}`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            query: query.trim(),
            category,
          }),
        }
      );

      if (!response.ok) {
        throw new Error(`Search failed: ${response.statusText}`);
      }

      const data = await response.json();
      setResults(data.results || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Search failed");
      setResults([]);
    } finally {
      setLoading(false);
    }
  }, [query, category, apiToken]);

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      handleSearch();
    }
  };

  const copyMagnetLink = (magnetLink: string) => {
    navigator.clipboard.writeText(magnetLink);
    // You could add a toast notification here
  };

  const addToDownloads = async (magnetLink: string, infoHash: string) => {
    if (!apiToken) return;

    try {
      setDownloadingHashes(prev => new Set(prev).add(infoHash));

      const response = await fetch(`http://127.0.0.1:8080/api/start?token=${apiToken}`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          source: magnetLink,
        }),
      });

      if (!response.ok) {
        throw new Error("Failed to start download");
      }

      // Success - you could show a notification here
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add download");
    } finally {
      setDownloadingHashes(prev => {
        const newSet = new Set(prev);
        newSet.delete(infoHash);
        return newSet;
      });
    }
  };

  const downloadTorrentFile = async (torrentURL: string, torrentName: string) => {
    try {
      // Fetch the .torrent file from our proxy endpoint
      const response = await fetch(`http://127.0.0.1:8080/api/download-torrent?url=${encodeURIComponent(torrentURL)}&token=${apiToken}`, {
        method: "GET",
      });

      if (!response.ok) {
        throw new Error("Failed to download .torrent file");
      }

      // Create blob from response and download
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${torrentName}.torrent`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to download .torrent file");
    }
  };

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up">
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Torrent Search</h2>
              <p className="mt-1 text-sm text-zinc-400">
                Search across multiple providers: The Pirate Bay, YTS, EZTV, Nyaa, and more
              </p>
            </div>
            <SourceBadge source="GO" />
          </div>
        </header>

        {/* Search Bar */}
        <div className="reveal-up rounded-2xl border border-white/10 bg-black/25 p-6">
          <div className="flex gap-4">
            <div className="flex-1">
              <div className="relative">
                <SearchIcon className="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-zinc-400" />
                <input
                  type="text"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  onKeyPress={handleKeyPress}
                  placeholder="Search for torrents..."
                  className="w-full rounded-lg border border-white/12 bg-black/40 py-3 pl-11 pr-4 text-zinc-100 placeholder-zinc-500 focus:border-cyan-500/50 focus:outline-none focus:ring-1 focus:ring-cyan-500/50"
                />
              </div>
            </div>

            <select
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              className="rounded-lg border border-white/12 bg-black/40 px-4 py-3 text-zinc-100 focus:border-cyan-500/50 focus:outline-none focus:ring-1 focus:ring-cyan-500/50"
            >
              {CATEGORIES.map((cat) => (
                <option key={cat.value} value={cat.value}>
                  {cat.label}
                </option>
              ))}
            </select>

            <button
              onClick={handleSearch}
              disabled={loading || !query.trim()}
              className="rounded-lg border border-[var(--bd-accent)] bg-[var(--acc-dim)] px-6 py-3 font-semibold text-cyan-100 hover:bg-cyan-500/20 disabled:opacity-50"
            >
              {loading ? "Searching..." : "Search"}
            </button>
          </div>
        </div>

        {/* Error Message */}
        {error && (
          <div className="reveal-up rounded-lg border border-red-500/30 bg-red-500/10 p-4">
            <p className="text-sm text-red-300">{error}</p>
          </div>
        )}

        {/* Results */}
        {results.length > 0 && (
          <div className="reveal-up">
            <div className="mb-4 flex items-center justify-between">
              <p className="text-sm text-zinc-400">
                Found {results.length} result{results.length !== 1 ? "s" : ""}
              </p>
            </div>

            <div className="space-y-3">
              {results.map((result, index) => (
                <article
                  key={`${result.info_hash}-${index}`}
                  className="rounded-xl border border-white/10 bg-black/25 p-4 transition-all hover:border-white/20 hover:bg-black/30"
                >
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex-1 space-y-2">
                      <h3 className="text-base font-semibold text-zinc-100">{result.name}</h3>

                      <div className="flex flex-wrap items-center gap-3 text-xs text-zinc-400">
                        <span className="rounded-full bg-cyan-500/10 px-2 py-1 text-cyan-300">
                          {result.provider}
                        </span>
                        {result.category && (
                          <span className="rounded-full bg-purple-500/10 px-2 py-1 text-purple-300">
                            {result.category}
                          </span>
                        )}
                        <span className="flex items-center gap-1">
                          <span className="text-emerald-400">↑ {result.seeders || 0}</span>
                          <span>/</span>
                          <span className="text-red-400">↓ {result.leechers || 0}</span>
                        </span>
                        {result.size > 0 && (
                          <span>{formatBytes(result.size)}</span>
                        )}
                      </div>

                      {result.description && (
                        <p className="line-clamp-2 text-sm text-zinc-500">{result.description}</p>
                      )}
                    </div>

                    <div className="flex flex-col gap-2">
                      {result.magnet_link && (
                        <button
                          onClick={() => addToDownloads(result.magnet_link, result.info_hash)}
                          disabled={downloadingHashes.has(result.info_hash)}
                          className="flex items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-4 py-2 text-sm font-semibold text-emerald-300 hover:bg-emerald-500/20 disabled:opacity-50"
                        >
                          {downloadingHashes.has(result.info_hash) ? (
                            <>
                              <span className="animate-spin">⏳</span>
                              <span>Adding...</span>
                            </>
                          ) : (
                            <>
                              <Download size={16} />
                              <span>Download</span>
                            </>
                          )}
                        </button>
                      )}

                      {result.torrent_url && (
                        <button
                          onClick={() => downloadTorrentFile(result.torrent_url!, result.name)}
                          className="flex items-center gap-2 rounded-lg border border-orange-500/30 bg-orange-500/10 px-4 py-2 text-sm font-semibold text-orange-300 hover:bg-orange-500/20"
                        >
                          <Download size={16} />
                          <span>.torrent File</span>
                        </button>
                      )}

                      <button
                        onClick={() => copyMagnetLink(result.magnet_link)}
                        className="flex items-center gap-2 rounded-lg border border-blue-500/30 bg-blue-500/10 px-4 py-2 text-sm font-semibold text-blue-300 hover:bg-blue-500/20"
                      >
                        <LinkIcon size={16} />
                        <span>Copy Magnet</span>
                      </button>
                    </div>
                  </div>
                </article>
              ))}
            </div>
          </div>
        )}

        {/* No Results */}
        {!loading && results.length === 0 && query && !error && (
          <div className="reveal-up rounded-lg border border-white/10 bg-black/25 p-12 text-center">
            <p className="text-zinc-400">No results found. Try a different search term.</p>
          </div>
        )}

        {/* Initial State */}
        {!loading && results.length === 0 && !query && (
          <div className="reveal-up rounded-lg border border-white/10 bg-black/25 p-12 text-center">
            <SearchIcon className="mx-auto mb-4 h-16 w-16 text-zinc-600" />
            <p className="text-lg text-zinc-400">Enter a search term to find torrents</p>
            <p className="mt-2 text-sm text-zinc-500">
              Searching across The Pirate Bay, YTS, EZTV, Nyaa, and more
            </p>
          </div>
        )}
      </section>
    </AppShell>
  );
}
