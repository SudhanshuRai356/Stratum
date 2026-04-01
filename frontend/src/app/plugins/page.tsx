"use client";

import { useEffect, useState, useCallback } from "react";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";

interface PluginStatus {
  name: string;
  version: string;
  state: string;
  healthy: boolean;
  last_error?: string;
  capabilities?: string[];
}

interface PluginData {
  [key: string]: PluginStatus;
}

export default function PluginsPage() {
  const { notifications } = useDashboardData();
  const [plugins, setPlugins] = useState<PluginData>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [apiToken, setApiToken] = useState<string | null>(null);

  // Fetch API token on mount
  useEffect(() => {
    fetch("http://127.0.0.1:8080/api/ws-auth")
      .then((res) => res.json())
      .then((data) => {
        if (data.token) {
          setApiToken(data.token);
        }
      })
      .catch(() => {
        // Token might not be available
      });
  }, []);

  const fetchPlugins = useCallback(async () => {
    if (!apiToken) return;

    try {
      const response = await fetch(`http://127.0.0.1:8080/api/plugins?token=${apiToken}`);
      if (!response.ok) {
        throw new Error(`Failed to fetch plugins: ${response.statusText}`);
      }
      const data = await response.json();
      setPlugins(data.plugins || {});
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch plugins");
    } finally {
      setLoading(false);
    }
  }, [apiToken]);

  useEffect(() => {
    if (apiToken) {
      fetchPlugins();
      const interval = setInterval(fetchPlugins, 5000); // Refresh every 5 seconds
      return () => clearInterval(interval);
    }
  }, [apiToken, fetchPlugins]);

  const handleStartPlugin = async (pluginName: string) => {
    try {
      const response = await fetch(
        `http://127.0.0.1:8080/api/plugins/${pluginName}/start?token=${apiToken}`,
        {
          method: "POST",
        }
      );
      if (!response.ok) {
        throw new Error(`Failed to start plugin: ${response.statusText}`);
      }
      await fetchPlugins(); // Refresh plugin list
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to start plugin");
    }
  };

  const handleStopPlugin = async (pluginName: string) => {
    try {
      const response = await fetch(
        `http://127.0.0.1:8080/api/plugins/${pluginName}/stop?token=${apiToken}`,
        {
          method: "POST",
        }
      );
      if (!response.ok) {
        throw new Error(`Failed to stop plugin: ${response.statusText}`);
      }
      await fetchPlugins(); // Refresh plugin list
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to stop plugin");
    }
  };

  const handleRestartPlugin = async (pluginName: string) => {
    try {
      const response = await fetch(
        `http://127.0.0.1:8080/api/plugins/${pluginName}/restart?token=${apiToken}`,
        {
          method: "POST",
        }
      );
      if (!response.ok) {
        throw new Error(`Failed to restart plugin: ${response.statusText}`);
      }
      await fetchPlugins(); // Refresh plugin list
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to restart plugin");
    }
  };

  const isRunning = (state: string) => state === "running";
  const isStopped = (state: string) => state === "stopped" || state === "registered";

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Plugins</h2>
            <p className="mt-1 text-sm text-zinc-400">Manage installed plugins and extensions.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="GO" />
            <button
              type="button"
              onClick={fetchPlugins}
              disabled={loading}
              className="rounded-lg border border-white/12 bg-black/25 px-3 py-1.5 text-xs text-zinc-200 disabled:opacity-50"
            >
              {loading ? "Loading..." : "Refresh"}
            </button>
          </div>
        </header>

        {error && (
          <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4">
            <p className="text-sm text-red-400">{error}</p>
          </div>
        )}

        {loading ? (
          <div className="rounded-lg border border-white/10 bg-black/25 p-8 text-center">
            <p className="text-sm text-zinc-400">Loading plugins...</p>
          </div>
        ) : Object.keys(plugins).length === 0 ? (
          <div className="rounded-lg border border-white/10 bg-black/25 p-8 text-center">
            <p className="text-sm text-zinc-400">No plugins available</p>
          </div>
        ) : (
          <section className="grid gap-4 md:grid-cols-2">
            {Object.entries(plugins).map(([pluginName, plugin]) => (
              <article key={pluginName} className="panel reveal-up rounded-2xl p-4 space-y-4">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <h3 className="text-lg font-semibold text-zinc-100">{plugin.name}</h3>
                    <p className="mt-1 text-xs text-zinc-500">v{plugin.version}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <span
                      className={[
                        "rounded-full px-2 py-1 text-xs font-semibold whitespace-nowrap",
                        plugin.healthy && isRunning(plugin.state)
                          ? "bg-emerald-500/20 text-emerald-300"
                          : plugin.healthy && isStopped(plugin.state)
                            ? "bg-yellow-500/20 text-yellow-300"
                            : "bg-red-500/20 text-red-300",
                      ].join(" ")}
                    >
                      {plugin.state}
                    </span>
                    {!plugin.healthy && <span className="text-xs text-red-400">⚠️</span>}
                  </div>
                </div>

                {plugin.capabilities && plugin.capabilities.length > 0 && (
                  <div className="flex flex-wrap gap-2">
                    {plugin.capabilities.map((cap) => (
                      <span
                        key={cap}
                        className="rounded-full bg-cyan-500/10 px-2 py-1 text-xs text-cyan-300 border border-cyan-500/20"
                      >
                        {cap}
                      </span>
                    ))}
                  </div>
                )}

                {plugin.last_error && (
                  <div className="rounded-lg bg-red-500/10 border border-red-500/20 p-2">
                    <p className="text-xs text-red-400">{plugin.last_error}</p>
                  </div>
                )}

                <div className="flex items-center gap-2">
                  {isRunning(plugin.state) ? (
                    <>
                      <button
                        type="button"
                        onClick={() => handleStopPlugin(pluginName)}
                        className="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-1.5 text-xs font-semibold text-red-300 hover:bg-red-500/20"
                      >
                        Stop
                      </button>
                      <button
                        type="button"
                        onClick={() => handleRestartPlugin(pluginName)}
                        className="rounded-lg border border-blue-500/30 bg-blue-500/10 px-3 py-1.5 text-xs font-semibold text-blue-300 hover:bg-blue-500/20"
                      >
                        Restart
                      </button>
                    </>
                  ) : (
                    <button
                      type="button"
                      onClick={() => handleStartPlugin(pluginName)}
                      className="rounded-lg border border-[var(--bd-accent)] bg-[var(--acc-dim)] px-3 py-1.5 text-xs font-semibold text-cyan-100 hover:bg-cyan-500/20"
                    >
                      Start
                    </button>
                  )}
                </div>
              </article>
            ))}
          </section>
        )}

        <div className="mt-8 rounded-lg border border-white/10 bg-black/25 p-6">
          <h3 className="text-lg font-semibold text-zinc-100 mb-4">Built-in Plugins</h3>
          <div className="space-y-4 text-sm text-zinc-400">
            <div>
              <p className="font-semibold text-zinc-300">Streaming Server</p>
              <p>Stream video and audio files while downloading. Supports HTTP range requests for seeking.</p>
            </div>
            <div>
              <p className="font-semibold text-zinc-300">Search</p>
              <p>Search for torrents across multiple providers (The Pirate Bay, Nyaa, and custom providers).</p>
            </div>
          </div>
        </div>
      </section>
    </AppShell>
  );
}
