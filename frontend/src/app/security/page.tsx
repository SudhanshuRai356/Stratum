"use client";

import { useMemo, useState } from "react";
import AppShell from "@/components/layout/AppShell";
import { useDashboardData } from "@/components/providers/DashboardDataProvider";
import SourceBadge from "@/components/ui/SourceBadge";

type SecurityRule = {
  id: string;
  label: string;
  enabled: boolean;
  source: "UI" | "RT";
};

export default function SecurityPage() {
  const { notifications } = useDashboardData();
  const [rules, setRules] = useState<SecurityRule[]>([
    { id: "ban-malformed", label: "Auto-ban malformed handshake peers", enabled: true, source: "RT" },
    { id: "strict-origin", label: "Enforce strict WS origin policy", enabled: true, source: "UI" },
    { id: "rate-limit", label: "Rate-limit reconnect storms", enabled: true, source: "RT" },
    { id: "tracker-sandbox", label: "Sandbox unknown trackers", enabled: false, source: "UI" },
  ]);

  const criticalAlerts = useMemo(
    () => notifications.filter((note) => note.level === "error" || note.level === "warn").slice(0, 8),
    [notifications],
  );

  return (
    <AppShell notifications={notifications}>
      <section className="mx-auto w-full max-w-[1280px] space-y-6">
        <header className="reveal-up flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-3xl font-semibold tracking-tight text-zinc-100">Security Center</h2>
            <p className="mt-1 text-sm text-zinc-400">Real-time safety posture and peer hardening controls.</p>
          </div>
          <div className="flex items-center gap-2">
            <SourceBadge source="RT" critical />
            <SourceBadge source="UI" />
          </div>
        </header>

        <section className="grid gap-4 lg:grid-cols-2">
          <article className="panel reveal-up rounded-2xl p-4">
            <h3 className="text-lg font-semibold text-zinc-100">Policy Rules</h3>
            <div className="mt-3 space-y-2">
              {rules.map((rule) => (
                <label
                  key={rule.id}
                  className="flex items-center justify-between rounded-lg border border-white/8 bg-black/20 px-3 py-2"
                >
                  <div>
                    <p className="text-sm text-zinc-200">{rule.label}</p>
                    <p className="mt-0.5 text-xs text-zinc-500">Source: {rule.source}</p>
                  </div>
                  <button
                    type="button"
                    onClick={() =>
                      setRules((prev) => prev.map((item) => (item.id === rule.id ? { ...item, enabled: !item.enabled } : item)))
                    }
                    className={[
                      "rounded-full border px-3 py-1 text-xs font-semibold transition",
                      rule.enabled
                        ? "border-emerald-400/40 bg-emerald-500/18 text-emerald-300"
                        : "border-zinc-500/40 bg-zinc-700/20 text-zinc-300",
                    ].join(" ")}
                  >
                    {rule.enabled ? "Enabled" : "Disabled"}
                  </button>
                </label>
              ))}
            </div>
          </article>

          <article className="panel reveal-up rounded-2xl p-4">
            <h3 className="text-lg font-semibold text-zinc-100">Realtime Alerts</h3>
            <ul className="mt-3 space-y-2">
              {criticalAlerts.length === 0 && <li className="text-sm text-zinc-400">No active warnings.</li>}
              {criticalAlerts.map((note) => (
                <li key={note.id} className="rounded-lg border border-white/8 bg-black/25 px-3 py-2 text-sm text-zinc-200">
                  <p>{note.message}</p>
                  <p className="mt-1 text-xs text-zinc-500">{note.level.toUpperCase()}</p>
                </li>
              ))}
            </ul>
          </article>
        </section>
      </section>
    </AppShell>
  );
}
