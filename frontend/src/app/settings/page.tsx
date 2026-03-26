"use client";

import { useState } from "react";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";
import { callGoBackend, isTauriRuntime } from "@/lib/runtime";

const STORAGE_KEY = "stratum.settings.v1";

type SettingsState = {
  wsUrl: string;
  goApiUrl: string;
  allowedOrigin: string;
  compactSidebar: boolean;
};

const DEFAULTS: SettingsState = {
  wsUrl: "ws://127.0.0.1:8080/ws",
  goApiUrl: "http://127.0.0.1:8080",
  allowedOrigin: "tauri://localhost, http://localhost:1420",
  compactSidebar: false,
};

export default function SettingsPage() {
  const { notifications } = useDashboardData();
  const [settings, setSettings] = useState<SettingsState>(() => {
    if (typeof window === "undefined") {
      return DEFAULTS;
    }
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) {
        return DEFAULTS;
      }
      const parsed = JSON.parse(raw) as Partial<SettingsState>;
      return { ...DEFAULTS, ...parsed };
    } catch {
      return DEFAULTS;
    }
  });
  const [status, setStatus] = useState<string>("");

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[980px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Settings</h2>
            <p className="mt-1 text-sm text-zinc-400">Configure websocket, backend URL, and desktop runtime integration values.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="GO" />
            <SourceBadge source="UI" />
            <SourceBadge source="FS" />
          </div>
        </header>

        <section className="panel reveal-up rounded-2xl p-5">
          <div className="space-y-4">
            <label className="block">
              <span className="mb-1 block text-sm text-zinc-300">WebSocket URL</span>
              <input
                type="text"
                value={settings.wsUrl}
                onChange={(event) => setSettings((prev) => ({ ...prev, wsUrl: event.target.value }))}
                className="w-full rounded-xl border border-white/12 bg-black/20 px-4 py-2 text-sm text-zinc-100 outline-none transition focus:border-[var(--bd-accent)]"
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-sm text-zinc-300">Go API URL</span>
              <input
                type="text"
                value={settings.goApiUrl}
                onChange={(event) => setSettings((prev) => ({ ...prev, goApiUrl: event.target.value }))}
                className="w-full rounded-xl border border-white/12 bg-black/20 px-4 py-2 text-sm text-zinc-100 outline-none transition focus:border-[var(--bd-accent)]"
              />
            </label>
            <label className="block">
              <span className="mb-1 block text-sm text-zinc-300">Allowed Origins</span>
              <input
                type="text"
                value={settings.allowedOrigin}
                onChange={(event) => setSettings((prev) => ({ ...prev, allowedOrigin: event.target.value }))}
                className="w-full rounded-xl border border-white/12 bg-black/20 px-4 py-2 text-sm text-zinc-100 outline-none transition focus:border-[var(--bd-accent)]"
              />
            </label>
            <label className="flex items-center justify-between rounded-xl border border-white/8 bg-black/20 px-4 py-3">
              <span className="text-sm text-zinc-200">Compact sidebar by default</span>
              <input
                type="checkbox"
                checked={settings.compactSidebar}
                onChange={(event) => setSettings((prev) => ({ ...prev, compactSidebar: event.target.checked }))}
              />
            </label>
          </div>

          <div className="mt-5 flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => {
                localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
                setStatus("Settings saved locally.");
              }}
              className="rounded-xl border border-[var(--bd-accent)] bg-[var(--acc-dim)] px-4 py-2 text-sm font-semibold text-cyan-100"
            >
              Save
            </button>
            <button
              type="button"
              onClick={async () => {
                setStatus("Testing backend connection...");
                try {
                  await callGoBackend<{ ok?: boolean }>("/health");
                  setStatus("Go backend reachable.");
                } catch (error) {
                  setStatus(`Backend call failed: ${String(error)}`);
                }
              }}
              className="rounded-xl border border-white/12 bg-black/25 px-4 py-2 text-sm text-zinc-200"
            >
              Test Go backend
            </button>
            <button
              type="button"
              disabled={!isTauriRuntime()}
              onClick={() => setStatus("Tauri runtime active. You can now map these values to Rust commands.")}
              className="rounded-xl border border-white/12 bg-black/25 px-4 py-2 text-sm text-zinc-200 disabled:opacity-50"
            >
              Check Tauri runtime
            </button>
          </div>
          {status && <p className="mt-3 text-xs text-cyan-200">{status}</p>}
        </section>
      </section>
    </AppShell>
  );
}
