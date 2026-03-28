export type TorrentLifecycleState = "Downloading" | "Seeding" | string;

export type TorrentState = {
  info_hash: string;
  name: string;
  progress: number;
  download_speed: number;
  connected_peers: number;
  state: TorrentLifecycleState;
  // Future-compatible fields expected by upcoming dashboard cards.
  total_downloaded?: number;
  total_uploaded?: number;
  upload_speed?: number;
  seeders?: number;
  leechers?: number;
  size_bytes?: number;
  category?: string;
};

export type WebSocketConnectionStatus =
  | "idle"
  | "connecting"
  | "open"
  | "closed"
  | "error";

export type SpeedHistoryPoint = {
  ts: number;
  downloadBps: number;
  uploadBps: number;
  peers: number;
  seeders: number;
  leechers: number;
  cpuPercent: number;
  diskPercent: number;
};

export type NotificationLevel = "info" | "warn" | "error" | "success";

export type DashboardNotification = {
  id: string;
  message: string;
  level: NotificationLevel;
  createdAt: number;
};

export type DashboardTotals = {
  downloadedBytes: number;
  uploadedBytes: number;
};
