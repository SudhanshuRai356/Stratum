"use client";

import { useMemo, useState } from "react";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";
import type { NotificationLevel } from "@/lib/types";

const LEVELS: NotificationLevel[] = ["info", "warn", "error", "success"];

export default function LogsPage() {
  const { notifications, history, torrents } = useDashboardData();
  const [level, setLevel] = useState<NotificationLevel | "all">("all");

  const filteredNotifications = useMemo(() => {
    if (level === "all") {
      return notifications;
    }
    return notifications.filter((item) => item.level === level);
  }, [notifications, level]);

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Logs &amp; Events</h2>
            <p className="mt-1 text-sm text-zinc-400">Event stream from websocket notifications and sampled runtime data.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="WS" />
            <SourceBadge source="RT" critical />
          </div>
        </header>

        <section className="panel reveal-up rounded-2xl p-4">
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => setLevel("all")}
              className={[
                "rounded-lg border px-3 py-1 text-xs transition",
                level === "all" ? "border-[var(--bd-accent)] bg-[var(--acc-dim)] text-cyan-100" : "border-white/12 text-zinc-300",
              ].join(" ")}
            >
              all
            </button>
            {LEVELS.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => setLevel(item)}
                className={[
                  "rounded-lg border px-3 py-1 text-xs transition",
                  level === item ? "border-[var(--bd-accent)] bg-[var(--acc-dim)] text-cyan-100" : "border-white/12 text-zinc-300",
                ].join(" ")}
              >
                {item}
              </button>
            ))}
            <span className="ml-auto text-xs text-zinc-500">{filteredNotifications.length} entries</span>
          </div>
        </section>

        <section className="grid gap-4 lg:grid-cols-2">
          <article className="panel reveal-up rounded-2xl p-4">
            <h3 className="text-lg font-semibold text-zinc-100">Notification Feed</h3>
            <ul className="mt-3 max-h-[28rem] space-y-2 overflow-y-auto pr-1">
              {filteredNotifications.length === 0 && <li className="text-sm text-zinc-400">No notifications for this filter.</li>}
              {filteredNotifications.map((note) => (
                <li key={note.id} className="rounded-lg border border-white/8 bg-black/25 px-3 py-2">
                  <p className="text-sm text-zinc-200">{note.message}</p>
                  <p className="mt-1 text-xs text-zinc-500">{note.level.toUpperCase()}</p>
                </li>
              ))}
            </ul>
          </article>
          <article className="panel reveal-up rounded-2xl p-4">
            <h3 className="text-lg font-semibold text-zinc-100">Runtime Sample Window</h3>
            <div className="mt-3 max-h-[28rem] overflow-y-auto rounded-lg border border-white/8 bg-black/30 p-3 font-mono text-xs text-zinc-300">
              {history.length === 0 && <p className="text-zinc-500">No runtime samples captured yet.</p>}
              {history.slice(-120).map((point) => (
                <p key={point.ts}>
                  t={point.ts} dl={point.downloadBps} ul={point.uploadBps} peers={point.peers} cpu={point.cpuPercent.toFixed(1)} disk=
                  {point.diskPercent.toFixed(1)}
                </p>
              ))}
            </div>
          </article>
        </section>

        <section className="panel reveal-up rounded-2xl p-4">
          <h3 className="text-lg font-semibold text-zinc-100">Live Transfer Metrics</h3>
          <div className="mt-3 overflow-x-auto rounded-lg border border-white/8">
            <table className="w-full min-w-[880px] text-left text-sm">
              <thead className="bg-[var(--bg-surface)] text-xs uppercase tracking-[0.08em] text-zinc-400">
                <tr>
                  <th className="px-3 py-2">Torrent</th>
                  <th className="px-3 py-2">State</th>
                  <th className="px-3 py-2">Peers</th>
                  <th className="px-3 py-2">Seeders</th>
                  <th className="px-3 py-2">Leechers</th>
                  <th className="px-3 py-2">Pieces</th>
                  <th className="px-3 py-2">Round</th>
                  <th className="px-3 py-2">Down</th>
                  <th className="px-3 py-2">Up</th>
                </tr>
              </thead>
              <tbody>
                {torrents.length === 0 && (
                  <tr>
                    <td colSpan={9} className="px-3 py-5 text-center text-zinc-500">
                      No active runtime torrent state yet.
                    </td>
                  </tr>
                )}
                {torrents.map((torrent) => (
                  <tr key={torrent.info_hash} className="border-t border-white/8 text-zinc-200">
                    <td className="max-w-[22rem] truncate px-3 py-2">{torrent.name}</td>
                    <td className="px-3 py-2">{torrent.state}</td>
                    <td className="px-3 py-2">{torrent.connected_peers ?? 0}</td>
                    <td className="px-3 py-2">{torrent.seeders ?? 0}</td>
                    <td className="px-3 py-2">{torrent.leechers ?? 0}</td>
                    <td className="px-3 py-2">
                      {typeof torrent.completed_pieces === "number" && typeof torrent.total_pieces === "number" && torrent.total_pieces > 0
                        ? `${torrent.completed_pieces}/${torrent.total_pieces}`
                        : "-"}
                    </td>
                    <td className="px-3 py-2">{typeof torrent.round === "number" ? torrent.round : "-"}</td>
                    <td className="px-3 py-2">{Math.round(torrent.download_speed || 0)}</td>
                    <td className="px-3 py-2">{Math.round(torrent.upload_speed || 0)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      </section>
    </AppShell>
  );
}
