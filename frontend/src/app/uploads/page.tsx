"use client";

import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";
import { formatPercent, formatSpeed } from "@/lib/format";

export default function UploadsPage() {
  const { seedingTorrents, notifications } = useDashboardData();

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Seeding / Uploads</h2>
            <p className="mt-1 text-sm text-zinc-400">Track ratio, upload speed, and swarm health for seeding torrents.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="WS" />
            <SourceBadge source="RT" critical />
          </div>
        </header>

        <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <article className="panel reveal-up rounded-2xl p-4">
            <p className="text-xs uppercase tracking-[0.08em] text-zinc-400">Seeding torrents</p>
            <p className="mt-2 text-3xl font-semibold text-zinc-100">{seedingTorrents.length}</p>
          </article>
          <article className="panel reveal-up rounded-2xl p-4">
            <p className="text-xs uppercase tracking-[0.08em] text-zinc-400">Aggregate upload</p>
            <p className="mt-2 font-mono text-3xl text-cyan-200">
              {formatSpeed(seedingTorrents.reduce((sum, torrent) => sum + (torrent.upload_speed || 0), 0))}
            </p>
          </article>
          <article className="panel reveal-up rounded-2xl p-4">
            <p className="text-xs uppercase tracking-[0.08em] text-zinc-400">Aggregate peers</p>
            <p className="mt-2 text-3xl font-semibold text-zinc-100">
              {seedingTorrents.reduce((sum, torrent) => sum + (torrent.connected_peers || 0), 0)}
            </p>
          </article>
          <article className="panel reveal-up rounded-2xl p-4">
            <p className="text-xs uppercase tracking-[0.08em] text-zinc-400">Avg progress</p>
            <p className="mt-2 text-3xl font-semibold text-zinc-100">
              {formatPercent(
                seedingTorrents.length > 0
                  ? seedingTorrents.reduce((sum, torrent) => sum + torrent.progress, 0) / seedingTorrents.length
                  : 0,
                1,
              )}
            </p>
          </article>
        </section>

        <section className="panel reveal-up overflow-hidden rounded-2xl">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[860px] text-left text-sm">
              <thead className="bg-[var(--bg-surface)] text-xs uppercase tracking-[0.08em] text-zinc-400">
                <tr>
                  <th className="px-4 py-3">Torrent</th>
                  <th className="px-4 py-3">Upload speed</th>
                  <th className="px-4 py-3">Download speed</th>
                  <th className="px-4 py-3">Peers</th>
                  <th className="px-4 py-3">Ratio</th>
                  <th className="px-4 py-3">Health</th>
                </tr>
              </thead>
              <tbody>
                {seedingTorrents.length === 0 && (
                  <tr>
                    <td colSpan={6} className="px-4 py-8 text-center text-zinc-400">
                      No seeding torrents yet.
                    </td>
                  </tr>
                )}
                {seedingTorrents.map((torrent) => {
                  const downloaded = torrent.total_downloaded ?? 1;
                  const uploaded = torrent.total_uploaded ?? 0;
                  const ratio = uploaded / Math.max(1, downloaded);
                  const health = Math.min(100, 30 + (torrent.seeders ?? 0) * 2 + (torrent.connected_peers ?? 0));

                  return (
                    <tr key={torrent.info_hash} className="border-t border-white/8 hover:bg-[var(--bg-overlay)]/70">
                      <td className="px-4 py-3">
                        <p className="font-medium text-zinc-100">{torrent.name}</p>
                        <p className="text-xs text-zinc-500">{torrent.state}</p>
                      </td>
                      <td className="px-4 py-3 font-mono text-cyan-200">{formatSpeed(torrent.upload_speed || 0)}</td>
                      <td className="px-4 py-3 font-mono text-zinc-300">{formatSpeed(torrent.download_speed || 0)}</td>
                      <td className="px-4 py-3 text-zinc-200">{torrent.connected_peers || 0}</td>
                      <td className="px-4 py-3 text-zinc-200">{ratio.toFixed(2)}</td>
                      <td className="px-4 py-3 text-zinc-200">{formatPercent(health, 0)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      </section>
    </AppShell>
  );
}
