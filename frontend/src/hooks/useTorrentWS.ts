"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type {
  DashboardNotification,
  DashboardTotals,
  SpeedHistoryPoint,
  TorrentState,
  WebSocketConnectionStatus,
} from "@/lib/types";

type UseTorrentWSOptions = {
  url?: string;
  reconnectDelayMs?: number;
};

type UseTorrentWSResult = {
  status: WebSocketConnectionStatus;
  latest: TorrentState | null;
  torrents: TorrentState[];
  history: SpeedHistoryPoint[];
  totals: DashboardTotals;
  notifications: DashboardNotification[];
  lastMessageAt: number | null;
  error: string | null;
  attempts: number;
};

const DEFAULT_RECONNECT_DELAY_MS = 1500;
const HISTORY_WINDOW = 60;
const TOTALS_STORAGE_KEY = "stratum.dashboard.totals";
const MAX_NOTIFICATIONS = 25;
const TRANSIENT_ZERO_GRACE_MS = 3500;

function isActiveState(state: string | undefined): boolean {
  return state === "Downloading" || state === "Seeding";
}

function stabilizeOptionalNumber(nextValue: number | undefined, prevValue: number | undefined): number | undefined {
  if (typeof nextValue === "number") {
    return nextValue;
  }
  return prevValue;
}

function stabilizeTransientZero(
  nextValue: number | undefined,
  prevValue: number | undefined,
  active: boolean,
  now: number,
  previousSeenAt: number | undefined,
): number | undefined {
  if (typeof nextValue !== "number") {
    return prevValue;
  }
  if (!active) {
    return nextValue;
  }
  if (nextValue > 0) {
    return nextValue;
  }
  if (typeof prevValue === "number" && prevValue > 0 && typeof previousSeenAt === "number") {
    if (now - previousSeenAt <= TRANSIENT_ZERO_GRACE_MS) {
      return prevValue;
    }
  }
  return nextValue;
}

function toDefaultWSURL(apiBase: string): string {
  try {
    const parsed = new URL(apiBase);
    parsed.protocol = parsed.protocol === "https:" ? "wss:" : "ws:";
    parsed.pathname = "/ws";
    parsed.search = "";
    parsed.hash = "";
    return parsed.toString();
  } catch {
    return "ws://127.0.0.1:8080/ws";
  }
}

function safeParseState(raw: string): TorrentState | null {
  try {
    const data = JSON.parse(raw) as TorrentState;
    if (typeof data !== "object" || data === null) {
      return null;
    }
    if (typeof data.info_hash !== "string") {
      return null;
    }
    return data;
  } catch {
    return null;
  }
}

function makeNotification(message: string, level: DashboardNotification["level"]): DashboardNotification {
  return {
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    message,
    level,
    createdAt: Date.now(),
  };
}

export function useTorrentWS(options?: UseTorrentWSOptions): UseTorrentWSResult {
  const reconnectDelayMs = options?.reconnectDelayMs ?? DEFAULT_RECONNECT_DELAY_MS;
  const apiBase = process.env.NEXT_PUBLIC_GO_API_URL ?? "http://127.0.0.1:8080";
  const defaultWsUrl = options?.url ?? process.env.NEXT_PUBLIC_TORRENT_WS_URL ?? toDefaultWSURL(apiBase);

  const envToken = process.env.NEXT_PUBLIC_TORRENT_API_TOKEN;
  const [resolvedWsUrl, setResolvedWsUrl] = useState(defaultWsUrl);
  const [resolvedToken, setResolvedToken] = useState<string | null>(envToken ?? null);

  useEffect(() => {
    if (envToken) {
      return;
    }

    let cancelled = false;
    const controller = new AbortController();

    const discoverAuth = async () => {
      try {
        const res = await fetch(`${apiBase.replace(/\/$/, "")}/api/ws-auth`, {
          method: "GET",
          signal: controller.signal,
        });
        if (!res.ok) {
          return;
        }
        const payload = (await res.json()) as { token?: string; ws_url?: string };
        if (cancelled) {
          return;
        }
        if (payload.ws_url) {
          setResolvedWsUrl(payload.ws_url);
        } else {
          setResolvedWsUrl(defaultWsUrl);
        }
        if (payload.token) {
          setResolvedToken(payload.token);
        }
      } catch {
        if (!cancelled) {
          setResolvedWsUrl(defaultWsUrl);
        }
      }
    };

    discoverAuth();
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [apiBase, defaultWsUrl, envToken]);

  const effectiveUrl = useMemo(() => {
    if (!resolvedToken || resolvedWsUrl.includes("token=")) {
      return resolvedWsUrl;
    }
    const separator = resolvedWsUrl.includes("?") ? "&" : "?";
    return `${resolvedWsUrl}${separator}token=${encodeURIComponent(resolvedToken)}`;
  }, [resolvedToken, resolvedWsUrl]);

  const reconnectTimerRef = useRef<number | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const latestRef = useRef<TorrentState | null>(null);
  const torrentsRef = useRef<Record<string, TorrentState>>({});
  const seenAtRef = useRef<Map<string, number>>(new Map());
  const previousStateRef = useRef<Map<string, string>>(new Map());

  const [status, setStatus] = useState<WebSocketConnectionStatus>("idle");
  const [latest, setLatest] = useState<TorrentState | null>(null);
  const [torrentsMap, setTorrentsMap] = useState<Record<string, TorrentState>>({});
  const [history, setHistory] = useState<SpeedHistoryPoint[]>([]);
  const [totals, setTotals] = useState<DashboardTotals>({ downloadedBytes: 0, uploadedBytes: 0 });
  const [notifications, setNotifications] = useState<DashboardNotification[]>([]);
  const [lastMessageAt, setLastMessageAt] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempts, setAttempts] = useState(0);

  const pushNotification = (message: string, level: DashboardNotification["level"] = "info") => {
    const next = makeNotification(message, level);
    setNotifications((prev) => [next, ...prev].slice(0, MAX_NOTIFICATIONS));
  };

  useEffect(() => {
    let rafId = 0;
    try {
      const raw = localStorage.getItem(TOTALS_STORAGE_KEY);
      if (!raw) {
        return;
      }
      const parsed = JSON.parse(raw) as DashboardTotals;
      const nextTotals = {
        downloadedBytes: Number.isFinite(parsed.downloadedBytes) ? parsed.downloadedBytes : 0,
        uploadedBytes: Number.isFinite(parsed.uploadedBytes) ? parsed.uploadedBytes : 0,
      };
      rafId = window.requestAnimationFrame(() => {
        setTotals(nextTotals);
      });
    } catch {
      // Ignore persistence errors in private or restricted contexts.
    }

    return () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
    };
  }, []);

  useEffect(() => {
    try {
      localStorage.setItem(TOTALS_STORAGE_KEY, JSON.stringify(totals));
    } catch {
      // Ignore persistence errors in private or restricted contexts.
    }
  }, [totals]);

  useEffect(() => {
    torrentsRef.current = torrentsMap;
  }, [torrentsMap]);

  useEffect(() => {
    const tick = window.setInterval(() => {
      const activeTorrents = Object.values(torrentsRef.current).filter((torrent) => isActiveState(torrent.state));

      const peers = activeTorrents.reduce((sum, torrent) => sum + Math.max(0, torrent.connected_peers || 0), 0);
      const seeders = activeTorrents.reduce((sum, torrent) => {
        if (typeof torrent.seeders === "number") {
          return sum + Math.max(0, torrent.seeders);
        }
        return sum + Math.max(0, Math.floor((torrent.connected_peers || 0) * 0.65));
      }, 0);
      const leechers = activeTorrents.reduce((sum, torrent) => {
        if (typeof torrent.leechers === "number") {
          return sum + Math.max(0, torrent.leechers);
        }
        const peerCount = Math.max(0, torrent.connected_peers || 0);
        const estimatedSeeders = Math.max(0, Math.floor(peerCount * 0.65));
        return sum + Math.max(0, peerCount - estimatedSeeders);
      }, 0);
      const downloadBps = activeTorrents.reduce((sum, torrent) => sum + Math.max(0, torrent.download_speed || 0), 0);
      const uploadBps = activeTorrents.reduce((sum, torrent) => {
        const upload = torrent.upload_speed;
        if (typeof upload === "number") {
          return sum + Math.max(0, upload);
        }
        return sum + Math.max(0, (torrent.download_speed || 0) * 0.12);
      }, 0);

      // Estimate utilization until backend exposes direct runtime metrics.
      const cpuPercent = Math.min(100, 10 + (downloadBps / (1024 * 1024)) * 12 + peers * 0.6);
      const diskPercent = Math.min(100, 8 + (downloadBps / (1024 * 1024)) * 9 + peers * 0.45);

      setHistory((prev) => {
        const nextPoint: SpeedHistoryPoint = {
          ts: Date.now(),
          downloadBps,
          uploadBps,
          peers,
          seeders,
          leechers,
          cpuPercent,
          diskPercent,
        };
        return [...prev, nextPoint].slice(-HISTORY_WINDOW);
      });

      setTotals((prev) => ({
        downloadedBytes: prev.downloadedBytes + downloadBps,
        uploadedBytes: prev.uploadedBytes + uploadBps,
      }));
    }, 1000);

    return () => {
      window.clearInterval(tick);
    };
  }, []);

  useEffect(() => {
    let isCancelled = false;

    const clearReconnectTimer = () => {
      if (reconnectTimerRef.current !== null) {
        window.clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
    };

    const connect = () => {
      if (isCancelled) {
        return;
      }

      clearReconnectTimer();
      setStatus("connecting");
      setAttempts((prev) => prev + 1);

      const ws = new WebSocket(effectiveUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        if (isCancelled) {
          return;
        }
        setStatus("open");
        setError(null);
        pushNotification("WebSocket connected", "success");
      };

      ws.onmessage = (event) => {
        if (isCancelled) {
          return;
        }

        const parsed = safeParseState(String(event.data));
        if (!parsed) {
          return;
        }

        const now = Date.now();
        const previouslySeenAt = seenAtRef.current.get(parsed.info_hash);

        latestRef.current = parsed;
        setLatest(parsed);
        setTorrentsMap((prev) => {
          const previousTorrent = prev[parsed.info_hash];
          const active = isActiveState(parsed.state);

          const stabilized: TorrentState = {
            ...previousTorrent,
            ...parsed,
            total_downloaded: stabilizeOptionalNumber(parsed.total_downloaded, previousTorrent?.total_downloaded),
            total_uploaded: stabilizeOptionalNumber(parsed.total_uploaded, previousTorrent?.total_uploaded),
            size_bytes: stabilizeOptionalNumber(parsed.size_bytes, previousTorrent?.size_bytes),
            connected_peers: stabilizeTransientZero(
              parsed.connected_peers,
              previousTorrent?.connected_peers,
              active,
              now,
              previouslySeenAt,
            ) ?? 0,
            download_speed: stabilizeTransientZero(
              parsed.download_speed,
              previousTorrent?.download_speed,
              active,
              now,
              previouslySeenAt,
            ) ?? 0,
            upload_speed: stabilizeTransientZero(
              parsed.upload_speed,
              previousTorrent?.upload_speed,
              active,
              now,
              previouslySeenAt,
            ),
            seeders: stabilizeTransientZero(
              parsed.seeders,
              previousTorrent?.seeders,
              active,
              now,
              previouslySeenAt,
            ),
            leechers: stabilizeTransientZero(
              parsed.leechers,
              previousTorrent?.leechers,
              active,
              now,
              previouslySeenAt,
            ),
          };

          return { ...prev, [parsed.info_hash]: stabilized };
        });

        const prevState = previousStateRef.current.get(parsed.info_hash);
        if (prevState && prevState !== parsed.state) {
          pushNotification(`${parsed.name}: ${prevState} -> ${parsed.state}`, "info");
        }
        if (parsed.state === "Seeding" && prevState !== "Seeding") {
          pushNotification(`${parsed.name} completed and entered seeding`, "success");
        }
        previousStateRef.current.set(parsed.info_hash, parsed.state);

        seenAtRef.current.set(parsed.info_hash, now);
        setLastMessageAt(now);
      };

      ws.onerror = () => {
        if (isCancelled) {
          return;
        }
        setStatus("error");
        setError("WebSocket error");
        pushNotification("WebSocket error", "error");
      };

      ws.onclose = () => {
        if (isCancelled) {
          return;
        }
        setStatus("closed");
        pushNotification("WebSocket disconnected. Reconnecting...", "warn");
        reconnectTimerRef.current = window.setTimeout(connect, reconnectDelayMs);
      };
    };

    connect();

    return () => {
      isCancelled = true;
      clearReconnectTimer();
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [effectiveUrl, reconnectDelayMs]);

  const torrents = useMemo(
    () => Object.values(torrentsMap).sort((a, b) => a.name.localeCompare(b.name)),
    [torrentsMap],
  );

  return {
    status,
    latest,
    torrents,
    history,
    totals,
    notifications,
    lastMessageAt,
    error,
    attempts,
  };
}
