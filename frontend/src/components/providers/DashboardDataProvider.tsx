"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";
import { useTorrentWS } from "@/hooks/useTorrentWS";
import type { TorrentState } from "@/lib/types";

type DashboardDataValue = ReturnType<typeof useTorrentWS> & {
  activeTorrents: TorrentState[];
  completedTorrents: TorrentState[];
  seedingTorrents: TorrentState[];
  totalPeersAcrossActive: number;
};

const DashboardDataContext = createContext<DashboardDataValue | null>(null);
const STOPPED_GRACE_MS = 90_000;

function sortByName(torrents: TorrentState[]): TorrentState[] {
  return [...torrents].sort((a, b) => a.name.localeCompare(b.name));
}

export function DashboardDataProvider({ children }: { children: ReactNode }) {
  const ws = useTorrentWS();

  const value = useMemo<DashboardDataValue>(() => {
    const referenceNow = ws.lastMessageAt ?? 0;
    const activeTorrents = sortByName(
      ws.torrents.filter(
        (torrent) => {
          if (torrent.state === "Starting" || torrent.state === "Downloading" || torrent.state === "Seeding") {
            return true;
          }

          if (torrent.state === "Stopped" && torrent.progress < 100 && typeof torrent.client_seen_at === "number") {
            return referenceNow > 0 && referenceNow - torrent.client_seen_at <= STOPPED_GRACE_MS;
          }

          return false;
        },
      ),
    );
    const completedTorrents = sortByName(ws.torrents.filter((torrent) => torrent.progress >= 100));
    const seedingTorrents = sortByName(ws.torrents.filter((torrent) => torrent.state === "Seeding"));
    const totalPeersAcrossActive = activeTorrents.reduce(
      (sum, torrent) => sum + Math.max(0, torrent.connected_peers || 0),
      0,
    );

    return {
      ...ws,
      activeTorrents,
      completedTorrents,
      seedingTorrents,
      totalPeersAcrossActive,
    };
  }, [ws]);

  return <DashboardDataContext.Provider value={value}>{children}</DashboardDataContext.Provider>;
}

export function useDashboardData() {
  const ctx = useContext(DashboardDataContext);
  if (!ctx) {
    throw new Error("useDashboardData must be used inside DashboardDataProvider");
  }
  return ctx;
}
