"use client";

import { useState } from "react";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";

type Plugin = {
  id: string;
  name: string;
  description: string;
  installed: boolean;
  enabled: boolean;
};

const DEFAULT_PLUGINS: Plugin[] = [
  {
    id: "tracker-intel",
    name: "Tracker Intel",
    description: "Correlates tracker latency and failure heatmaps across swarms.",
    installed: true,
    enabled: true,
  },
  {
    id: "peer-geo",
    name: "Peer Geo Lens",
    description: "Estimates regional peer distribution from observed endpoints.",
    installed: false,
    enabled: false,
  },
  {
    id: "auto-ratio",
    name: "Auto Ratio Guard",
    description: "Stops seeding when per-torrent ratio target is reached.",
    installed: true,
    enabled: false,
  },
  {
    id: "disk-probe",
    name: "Disk Probe",
    description: "Tracks write pressure spikes and predicts cache flush intervals.",
    installed: false,
    enabled: false,
  },
];

export default function PluginsPage() {
  const { notifications } = useDashboardData();
  const [plugins, setPlugins] = useState<Plugin[]>(DEFAULT_PLUGINS);

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Plugins</h2>
            <p className="mt-1 text-sm text-zinc-400">Frontend-managed extension surface. Safe to stub and expand later.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="UI" />
            <SourceBadge source="FS" />
          </div>
        </header>

        <section className="grid gap-4 md:grid-cols-2">
          {plugins.map((plugin) => (
            <article key={plugin.id} className="panel reveal-up rounded-2xl p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <h3 className="text-lg font-semibold text-zinc-100">{plugin.name}</h3>
                  <p className="mt-1 text-sm text-zinc-400">{plugin.description}</p>
                </div>
                <span
                  className={[
                    "rounded-full px-2 py-1 text-xs font-semibold",
                    plugin.enabled ? "bg-emerald-500/20 text-emerald-300" : "bg-zinc-700/30 text-zinc-300",
                  ].join(" ")}
                >
                  {plugin.enabled ? "enabled" : "disabled"}
                </span>
              </div>

              <div className="mt-4 flex items-center gap-2">
                <button
                  type="button"
                  onClick={() =>
                    setPlugins((prev) =>
                      prev.map((item) => (item.id === plugin.id ? { ...item, installed: true } : item)),
                    )
                  }
                  disabled={plugin.installed}
                  className="rounded-lg border border-white/12 bg-black/25 px-3 py-1.5 text-xs text-zinc-200 disabled:opacity-50"
                >
                  {plugin.installed ? "Installed" : "Install"}
                </button>
                <button
                  type="button"
                  onClick={() =>
                    setPlugins((prev) =>
                      prev.map((item) =>
                        item.id === plugin.id ? { ...item, enabled: item.installed ? !item.enabled : false } : item,
                      ),
                    )
                  }
                  disabled={!plugin.installed}
                  className="rounded-lg border border-[var(--bd-accent)] bg-[var(--acc-dim)] px-3 py-1.5 text-xs font-semibold text-cyan-100 disabled:opacity-50"
                >
                  {plugin.enabled ? "Disable" : "Enable"}
                </button>
              </div>
            </article>
          ))}
        </section>
      </section>
    </AppShell>
  );
}
