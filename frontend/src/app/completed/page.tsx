"use client";

import { useState } from "react";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";
import { formatBytes } from "@/lib/format";
import { isTauriRuntime } from "@/lib/runtime";

export default function CompletedPage() {
  const { completedTorrents, notifications } = useDashboardData();
  const [status, setStatus] = useState<string>("");

  const exportAsJson = () => {
    const blob = new Blob([JSON.stringify(completedTorrents, null, 2)], { type: "application/json" });
    const href = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = href;
    anchor.download = `completed-torrents-${Date.now()}.json`;
    anchor.style.display = "none";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    window.setTimeout(() => URL.revokeObjectURL(href), 1000);
    setStatus("Exported completed torrents as JSON.");
  };

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Completed</h2>
            <p className="mt-1 text-sm text-zinc-400">Filesystem-oriented view for completed downloads and export actions.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="FS" />
            <SourceBadge source="UI" />
          </div>
        </header>

        <section className="panel reveal-up rounded-2xl p-4">
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={exportAsJson}
              className="rounded-xl border border-white/12 bg-black/25 px-4 py-2 text-sm text-zinc-100 transition hover:border-[var(--bd-accent)]"
            >
              Export JSON
            </button>
            <button
              type="button"
              disabled={!isTauriRuntime()}
              onClick={() => setStatus("Tauri runtime detected. Native file actions can be wired to Rust commands.")}
              className="rounded-xl border border-white/12 bg-black/25 px-4 py-2 text-sm text-zinc-100 transition hover:border-[var(--bd-accent)] disabled:opacity-50"
            >
              Open Completed Folder (Tauri)
            </button>
          </div>
          {status && <p className="mt-3 text-xs text-cyan-200">{status}</p>}
        </section>

        <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {completedTorrents.length === 0 && (
            <div className="panel rounded-xl p-6 text-sm text-zinc-400 md:col-span-2 xl:col-span-3">
              Completed torrents will appear here once progress reaches 100%.
            </div>
          )}
          {completedTorrents.map((torrent) => (
            <article key={torrent.info_hash} className="panel reveal-up rounded-xl p-4">
              <p className="line-clamp-2 text-base font-semibold text-zinc-100">{torrent.name}</p>
              <p className="mt-2 text-sm text-zinc-400">State: {torrent.state}</p>
              <p className="mt-1 text-sm text-zinc-300">Size: {formatBytes(torrent.size_bytes || 0)}</p>
              <p className="mt-1 text-sm text-zinc-300">Uploaded: {formatBytes(torrent.total_uploaded || 0)}</p>
            </article>
          ))}
        </section>
      </section>
    </AppShell>
  );
}
