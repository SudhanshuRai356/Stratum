"use client";

import { useMemo, useState } from "react";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";
import { formatBytes, formatEta, formatPercent, formatSpeed } from "@/lib/format";
import { callGoBackend } from "@/lib/runtime";

type CommandResult = {
  status?: string;
  message?: string;
};

type TorrentAction = "pause" | "resume" | "reannounce" | "stop";

async function sendTorrentCommand(infoHash: string, action: TorrentAction) {
  return callGoBackend<CommandResult>("/api/control", {
    method: "POST",
    body: JSON.stringify({ info_hash: infoHash, action }),
  });
}

function actionSetForState(state: string): Array<{ action: TorrentAction; label: string }> {
  if (state === "Seeding") {
    return [
      { action: "resume", label: "start" },
      { action: "reannounce", label: "reannounce" },
      { action: "stop", label: "stop" },
    ];
  }
  if (state === "Downloading") {
    return [
      { action: "pause", label: "pause" },
      { action: "resume", label: "start" },
      { action: "stop", label: "stop" },
    ];
  }
  return [
    { action: "resume", label: "start" },
    { action: "stop", label: "stop" },
  ];
}

export default function DownloadsPage() {
  const { activeTorrents, notifications } = useDashboardData();
  const [query, setQuery] = useState("");
  const [busyHash, setBusyHash] = useState<string | null>(null);
  const [result, setResult] = useState<string>("");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) {
      return activeTorrents;
    }
    return activeTorrents.filter(
      (torrent) => torrent.name.toLowerCase().includes(q) || torrent.info_hash.toLowerCase().includes(q),
    );
  }, [activeTorrents, query]);

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Downloads</h2>
            <p className="mt-1 text-sm text-zinc-400">Primary operations page for active transfers and control actions.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="WS" />
            <SourceBadge source="GO" />
            <SourceBadge source="RT" critical />
          </div>
        </header>

        <section className="panel reveal-up rounded-2xl p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <input
              type="search"
              placeholder="Search torrent name or hash"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              className="w-full rounded-xl border border-white/12 bg-black/20 px-4 py-2 text-sm text-zinc-100 outline-none transition focus:border-[var(--bd-accent)] md:w-[25rem]"
            />
            <p className="text-sm text-zinc-400">
              {filtered.length} visible / {activeTorrents.length} active
            </p>
          </div>
          {result && <p className="mt-3 text-xs text-cyan-200">{result}</p>}
        </section>

        <section className="panel reveal-up overflow-hidden rounded-2xl">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[940px] text-left text-sm">
              <thead className="bg-[var(--bg-surface)] text-xs uppercase tracking-[0.08em] text-zinc-400">
                <tr>
                  <th className="px-4 py-3">Torrent</th>
                  <th className="px-4 py-3">State</th>
                  <th className="px-4 py-3">Progress</th>
                  <th className="px-4 py-3">Down</th>
                  <th className="px-4 py-3">Up</th>
                  <th className="px-4 py-3">ETA</th>
                  <th className="px-4 py-3">Peers</th>
                  <th className="px-4 py-3">Pieces</th>
                  <th className="px-4 py-3">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.length === 0 && (
                  <tr>
                    <td colSpan={9} className="px-4 py-8 text-center text-zinc-400">
                      No matching torrents.
                    </td>
                  </tr>
                )}
                {filtered.map((torrent) => {
                  const pendingMetadata = torrent.pending_metadata || torrent.info_hash.startsWith("pending:");
                  const remaining = Math.max(0, 100 - torrent.progress);
                  const speed = Math.max(0, torrent.download_speed || 0);
                  const etaSeconds = speed > 0 ? ((torrent.size_bytes ?? 0) * (remaining / 100)) / speed : Infinity;

                  return (
                    <tr key={torrent.info_hash} className="border-t border-white/8 hover:bg-[var(--bg-overlay)]/70">
                      <td className="px-4 py-3">
                        <p className="max-w-[22rem] truncate font-medium text-zinc-100">{torrent.name}</p>
                        <p className="font-mono text-[11px] text-zinc-500">{torrent.info_hash.slice(0, 18)}...</p>
                      </td>
                      <td className="px-4 py-3 text-zinc-300">{torrent.state}</td>
                      <td className="px-4 py-3">
                        <div className="w-36 rounded-full bg-black/35">
                          <div
                            className="h-2 rounded-full bg-gradient-to-r from-emerald-400 to-cyan-400"
                            style={{ width: `${Math.max(0, Math.min(100, torrent.progress))}%` }}
                          />
                        </div>
                        <p className="mt-1 text-xs text-zinc-400">{formatPercent(torrent.progress, 1)}</p>
                      </td>
                      <td className="px-4 py-3 font-mono text-cyan-200">{formatSpeed(torrent.download_speed || 0)}</td>
                      <td className="px-4 py-3 font-mono text-zinc-300">{formatSpeed(torrent.upload_speed || 0)}</td>
                      <td className="px-4 py-3 text-zinc-300">{formatEta(etaSeconds)}</td>
                      <td className="px-4 py-3 text-zinc-300">{torrent.connected_peers ?? 0}</td>
                      <td className="px-4 py-3 text-zinc-300">
                        {typeof torrent.completed_pieces === "number" && typeof torrent.total_pieces === "number" && torrent.total_pieces > 0
                          ? `${torrent.completed_pieces}/${torrent.total_pieces}`
                          : "-"}
                      </td>
                      <td className="px-4 py-3">
                        {pendingMetadata ? (
                          <p className="text-xs text-amber-200">fetching metadata...</p>
                        ) : (
                          <div className="flex items-center gap-2">
                            {actionSetForState(torrent.state).map(({ action, label }) => (
                              <button
                                key={`${torrent.info_hash}-${action}`}
                                type="button"
                                disabled={busyHash === torrent.info_hash}
                                onClick={async () => {
                                  setBusyHash(torrent.info_hash);
                                  setResult("");
                                  try {
                                    const response = await sendTorrentCommand(torrent.info_hash, action);
                                    setResult(response.message ?? `Command ${action} sent for ${torrent.name}`);
                                  } catch (error) {
                                    setResult(`Failed to ${action}: ${String(error)}`);
                                  } finally {
                                    setBusyHash(null);
                                  }
                                }}
                                className="rounded-lg border border-white/12 bg-black/25 px-2 py-1 text-xs text-zinc-200 transition hover:border-[var(--bd-accent)] hover:text-cyan-100 disabled:opacity-50"
                              >
                                {label}
                              </button>
                            ))}
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>

        <section className="grid gap-4 md:grid-cols-3">
          <article className="panel reveal-up rounded-2xl p-4">
            <p className="text-sm text-zinc-300">Realtime throughput</p>
            <p className="mt-2 font-mono text-2xl text-cyan-200">
              {formatSpeed(filtered.reduce((sum, torrent) => sum + (torrent.download_speed || 0), 0))}
            </p>
          </article>
          <article className="panel reveal-up rounded-2xl p-4">
            <p className="text-sm text-zinc-300">Realtime uploaded</p>
            <p className="mt-2 font-mono text-2xl text-zinc-100">
              {formatSpeed(filtered.reduce((sum, torrent) => sum + (torrent.upload_speed || 0), 0))}
            </p>
          </article>
          <article className="panel reveal-up rounded-2xl p-4">
            <p className="text-sm text-zinc-300">Visible size</p>
            <p className="mt-2 font-mono text-2xl text-zinc-100">
              {formatBytes(filtered.reduce((sum, torrent) => sum + (torrent.size_bytes || 0), 0))}
            </p>
          </article>
        </section>
      </section>
    </AppShell>
  );
}
