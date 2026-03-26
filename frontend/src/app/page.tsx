"use client";

import NetworkChart from "@/components/dashboard/NetworkChart";
import StatCard from "@/components/dashboard/StatCard";
import TorrentCard from "@/components/dashboard/TorrentCard";
import UtilizationChart from "@/components/dashboard/UtilizationChart";
import AppShell from "@/components/layout/AppShell";
import { useTorrentWS } from "@/hooks/useTorrentWS";
import { formatCompactStorage } from "@/lib/format";

const FALLBACK_TREND = {
  totalDownloaded: "+12.5% last month",
  totalUploaded: "+7.8% last month",
  activeTorrents: "-1 last hour",
  peersConnected: "+3.1% daily",
};

export default function Home() {
  const { status, torrents, history, totals, notifications } = useTorrentWS();

  const activeTorrents = torrents.filter(
    (torrent) => torrent.state === "Downloading" || torrent.state === "Seeding",
  );

  const totalPeers = activeTorrents.reduce((sum, torrent) => sum + (torrent.connected_peers || 0), 0);

  const chartPoints = history.length > 0 ? history : Array.from({ length: 60 }, (_, idx) => ({
    ts: idx,
    downloadBps: 0,
    uploadBps: 0,
    peers: 0,
    cpuPercent: 0,
    diskPercent: 0,
  }));

  return (
    <AppShell title="Overview" notifications={notifications}>
      <section className="mx-auto w-full max-w-[1240px] space-y-6">
        <header>
          <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Overview</h2>
          <p className="mt-1 text-sm text-zinc-400">Live system state from TorrentState websocket feed.</p>
        </header>

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <StatCard
            title="WSRT Total Downloaded"
            value={formatCompactStorage(totals.downloadedBytes)}
            subtitle="Cumulative bytes downloaded"
            trend={FALLBACK_TREND.totalDownloaded}
            trendTone="cyan"
          />
          <StatCard
            title="WSRT Total Uploaded"
            value={formatCompactStorage(totals.uploadedBytes)}
            subtitle="Persisted between sessions"
            trend={FALLBACK_TREND.totalUploaded}
            trendTone="cyan"
          />
          <StatCard
            title="WSRT Active Torrents"
            value={String(activeTorrents.length)}
            subtitle={`WS status: ${status}`}
            trend={FALLBACK_TREND.activeTorrents}
            trendTone="red"
          />
          <StatCard
            title="WSRT Peers Connected"
            value={String(totalPeers)}
            subtitle="Total peers across active torrents"
            trend={FALLBACK_TREND.peersConnected}
            trendTone="green"
          />
        </div>

        <section className="space-y-3">
          <h3 className="text-[2rem] font-semibold text-zinc-100">Advanced Metrics</h3>
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
              cpu: point.cpuPercent,
              disk: point.diskPercent,
            }))}
          />
          </div>
        </section>

        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-[2rem] font-semibold text-zinc-100">Active Downloads</h3>
            <p className="text-sm text-zinc-400">WS per-torrent cards</p>
          </div>

          <div className="grid gap-4 xl:grid-cols-3">
            {activeTorrents.length === 0 && (
              <div className="panel rounded-2xl p-6 text-sm text-zinc-400 xl:col-span-3">
                No active torrents yet. Start backend download and the dashboard will fill in live.
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
