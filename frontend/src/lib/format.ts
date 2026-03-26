export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }

  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let scaled = value;
  let unit = 0;

  while (scaled >= 1024 && unit < units.length - 1) {
    scaled /= 1024;
    unit += 1;
  }

  const precision = scaled >= 100 ? 0 : scaled >= 10 ? 1 : 2;
  return `${scaled.toFixed(precision)} ${units[unit]}`;
}

export function formatSpeed(bytesPerSecond: number): string {
  return `${formatBytes(bytesPerSecond)}/s`;
}

export function formatCompactStorage(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 GB";
  }
  const gb = bytes / 1024 ** 3;
  if (gb >= 1024) {
    return `${(gb / 1024).toFixed(2)} TB`;
  }
  return `${gb.toFixed(1)} GB`;
}

export function formatEta(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return "∞";
  }
  if (seconds <= 1) {
    return "Done";
  }

  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);

  if (hours <= 0) {
    return `${mins}m`;
  }
  return `${hours}h ${mins}m`;
}

export function formatPercent(value: number, digits = 1): string {
  if (!Number.isFinite(value)) {
    return "0%";
  }
  return `${value.toFixed(digits)}%`;
}
