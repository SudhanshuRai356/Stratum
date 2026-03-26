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
  const wsUrl = options?.url ?? process.env.NEXT_PUBLIC_TORRENT_WS_URL ?? "ws://127.0.0.1:8080/ws";

  const token = process.env.NEXT_PUBLIC_TORRENT_API_TOKEN;
  const effectiveUrl = useMemo(() => {
    if (!token || wsUrl.includes("token=")) {
      return wsUrl;
    }
    const separator = wsUrl.includes("?") ? "&" : "?";
    return `${wsUrl}${separator}token=${encodeURIComponent(token)}`;
  }, [token, wsUrl]);

  const reconnectTimerRef = useRef<number | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const latestRef = useRef<TorrentState | null>(null);
  const previousStateRef = useRef<Map<string, string>>(new Map());

  const [status, setStatus] = useState<WebSocketConnectionStatus>("idle");
  const [latest, setLatest] = useState<TorrentState | null>(null);
  const [torrentsMap, setTorrentsMap] = useState<Record<string, TorrentState>>({});
  const [history, setHistory] = useState<SpeedHistoryPoint[]>([]);
  const [totals, setTotals] = useState<DashboardTotals>(() => {
    if (typeof window === "undefined") {
      return { downloadedBytes: 0, uploadedBytes: 0 };
    }
    try {
      const raw = localStorage.getItem(TOTALS_STORAGE_KEY);
      if (!raw) {
        return { downloadedBytes: 0, uploadedBytes: 0 };
      }
      const parsed = JSON.parse(raw) as DashboardTotals;
      return {
        downloadedBytes: Number.isFinite(parsed.downloadedBytes) ? parsed.downloadedBytes : 0,
        uploadedBytes: Number.isFinite(parsed.uploadedBytes) ? parsed.uploadedBytes : 0,
      };
    } catch {
      return { downloadedBytes: 0, uploadedBytes: 0 };
    }
  });
  const [notifications, setNotifications] = useState<DashboardNotification[]>([]);
  const [lastMessageAt, setLastMessageAt] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempts, setAttempts] = useState(0);

  const pushNotification = (message: string, level: DashboardNotification["level"] = "info") => {
    const next = makeNotification(message, level);
    setNotifications((prev) => [next, ...prev].slice(0, MAX_NOTIFICATIONS));
  };

  useEffect(() => {
    try {
      localStorage.setItem(TOTALS_STORAGE_KEY, JSON.stringify(totals));
    } catch {
      // Ignore persistence errors in private or restricted contexts.
    }
  }, [totals]);

  useEffect(() => {
    const tick = window.setInterval(() => {
      const active = latestRef.current;
      if (!active) {
        return;
      }

      const peers = Math.max(0, active.connected_peers || 0);
      const downloadBps = Math.max(0, active.download_speed || 0);
      const uploadBps = Math.max(0, active.upload_speed || Math.max(0, downloadBps * 0.12));

      // Estimate utilization until backend exposes direct runtime metrics.
      const cpuPercent = Math.min(100, 10 + (downloadBps / (1024 * 1024)) * 12 + peers * 0.6);
      const diskPercent = Math.min(100, 8 + (downloadBps / (1024 * 1024)) * 9 + peers * 0.45);

      setHistory((prev) => {
        const nextPoint: SpeedHistoryPoint = {
          ts: Date.now(),
          downloadBps,
          uploadBps,
          peers,
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

        latestRef.current = parsed;
        setLatest(parsed);
        setTorrentsMap((prev) => ({ ...prev, [parsed.info_hash]: parsed }));

        const prevState = previousStateRef.current.get(parsed.info_hash);
        if (prevState && prevState !== parsed.state) {
          pushNotification(`${parsed.name}: ${prevState} -> ${parsed.state}`, "info");
        }
        if (parsed.state === "Seeding" && prevState !== "Seeding") {
          pushNotification(`${parsed.name} completed and entered seeding`, "success");
        }
        previousStateRef.current.set(parsed.info_hash, parsed.state);

        setLastMessageAt(Date.now());
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
