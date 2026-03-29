"use client";

import NetworkChart from "@/components/dashboard/NetworkChart";
import StartDownloadBar from "@/components/dashboard/StartDownloadBar";
import StatCard from "@/components/dashboard/StatCard";
import TorrentCard from "@/components/dashboard/TorrentCard";
import UtilizationChart from "@/components/dashboard/UtilizationChart";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";
import { formatCompactStorage } from "@/lib/format";

export default function DashboardPage() {
  const { status, history, totals, notifications, activeTorrents, totalPeersAcrossActive } = useDashboardData();

  const chartPoints =
    history.length > 0
      ? history
      : Array.from({ length: 60 }, (_, idx) => ({
          ts: idx,
          downloadBps: 0,
          uploadBps: 0,
          peers: 0,
          seeders: 0,
          leechers: 0,
          cpuPercent: 0,
          diskPercent: 0,
        }));

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Global Overview</h2>
            <p className="mt-1 text-sm text-zinc-400">Live TorrentState stream from hub.go rendered at 1s intervals.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="WS" />
            <SourceBadge source="RT" critical />
            <span className="rounded-full border border-white/10 bg-black/30 px-3 py-1 text-xs text-zinc-300">
              WS status: {status}
            </span>
          </div>
        </header>

        <StartDownloadBar />

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <StatCard
            title="Total Downloaded"
            value={formatCompactStorage(totals.downloadedBytes)}
            subtitle="Cumulative bytes downloaded"
          />
          <StatCard
            title="Total Uploaded"
            value={formatCompactStorage(totals.uploadedBytes)}
            subtitle="Persisted between sessions"
          />
          <StatCard
            title="Active Torrents"
            value={String(activeTorrents.length)}
            subtitle="Downloading + Seeding"
          />
          <StatCard
            title="Peers Connected"
            value={String(totalPeersAcrossActive)}
            subtitle="Across active torrents"
          />
        </div>

        <section className="space-y-3">
          <h3 className="text-2xl font-semibold text-zinc-100">Advanced Metrics</h3>
          <div className="grid gap-4 xl:grid-cols-2">
            <NetworkChart
              data={chartPoints.map((point) => ({
                ts: point.ts,
                downloadBps: point.downloadBps,
                uploadBps: point.uploadBps,
              }))}
            />
            <UtilizationChart
              data={chartPoints.map((point) => ({
                ts: point.ts,
                peers: point.peers,
                seeders: point.seeders,
                leechers: point.leechers,
                cpu: point.cpuPercent,
                disk: point.diskPercent,
              }))}
            />
          </div>
        </section>

        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-2xl font-semibold text-zinc-100">Active Downloads</h3>
          </div>

          <div className="grid gap-4 xl:grid-cols-3">
            {activeTorrents.length === 0 && (
              <div className="panel rounded-2xl p-6 text-sm text-zinc-400 xl:col-span-3">
                No active torrents yet. Start a backend transfer and this view will update in real time.
              </div>
            )}
            {activeTorrents.map((torrent) => (
              <TorrentCard key={torrent.info_hash} torrent={torrent} />
            ))}
          </div>
        </section>
      </section>
    </AppShell>
  );
}
