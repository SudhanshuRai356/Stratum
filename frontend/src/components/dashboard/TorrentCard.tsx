import type { TorrentState } from "@/lib/types";
import SourceBadge from "@/components/ui/SourceBadge";
import { formatBytes, formatEta, formatPercent, formatSpeed } from "@/lib/format";

type TorrentCardProps = {
  torrent: TorrentState;
};

function safeSizeBytes(torrent: TorrentState): number {
  if (torrent.size_bytes && torrent.size_bytes > 0) {
    return torrent.size_bytes;
  }

  if (torrent.total_downloaded && torrent.progress > 0) {
    return torrent.total_downloaded / Math.max(0.01, torrent.progress / 100);
  }

  return 0;
}

export default function TorrentCard({ torrent }: TorrentCardProps) {
  const sizeBytes = safeSizeBytes(torrent);
  const uploaded = torrent.total_uploaded ?? 0;
  const downloaded = torrent.total_downloaded ?? 0;
  const ratio = downloaded > 0 ? uploaded / downloaded : 0;

  const speedDown = Math.max(0, torrent.download_speed || 0);
  const speedUp = Math.max(0, torrent.upload_speed || 0);
  const remainingBytes = Math.max(0, sizeBytes * (1 - torrent.progress / 100));
  const eta = speedDown > 0 ? remainingBytes / speedDown : Number.POSITIVE_INFINITY;

  const peers = Math.max(0, torrent.connected_peers || 0);
  const seeders = torrent.seeders ?? Math.max(0, Math.floor(peers * 0.65));
  const leechers = torrent.leechers ?? Math.max(0, peers - seeders);
  const health = peers > 0 ? Math.min(100, 35 + seeders * 2) : 0;
  const category = torrent.category || "General";

  return (
    <article className="panel reveal-up rounded-xl p-4">
      <div className="flex items-start justify-between gap-3">
        <h4 className="line-clamp-2 text-lg font-semibold text-zinc-100">{torrent.name || "Unnamed Torrent"}</h4>
        <span
          className={[
            "rounded-full px-2.5 py-1 text-xs font-semibold",
            torrent.state === "Seeding"
              ? "bg-emerald-500/20 text-emerald-300"
              : "bg-cyan-500/20 text-cyan-300",
          ].join(" ")}
        >
          {torrent.state || "Downloading"}
        </span>
      </div>

      <div className="mt-2 flex items-center gap-2">
        <SourceBadge source="WS" />
        <SourceBadge source="UI" />
      </div>

      <div className="mt-3 flex items-center gap-4 text-xs text-zinc-300">
        <span>↓ {formatSpeed(speedDown)}</span>
        <span>↑ {formatSpeed(speedUp)}</span>
      </div>

      <div className="mt-3 h-2 overflow-hidden rounded-full bg-cyan-950/60">
        <div
          className="h-full rounded-full bg-gradient-to-r from-cyan-300 to-cyan-500 transition-all duration-500"
          style={{ width: `${Math.max(0, Math.min(100, torrent.progress))}%` }}
        />
      </div>

      <div className="mt-2 flex items-center justify-between text-xs text-zinc-400">
        <span>{formatPercent(torrent.progress, 0)}</span>
        <span>ETA: {formatEta(eta)}</span>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-y-1 text-xs text-zinc-300 sm:grid-cols-3">
        <p>Seeders: {seeders}</p>
        <p>Leechers: {leechers}</p>
        <p>Size: {formatBytes(sizeBytes)}</p>
        <p>Ratio: {ratio.toFixed(2)}</p>
        <p>Health: {formatPercent(health, 0)}</p>
        <p>Category: {category}</p>
      </div>
    </article>
  );
}
