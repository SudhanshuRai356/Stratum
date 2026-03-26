"use client";

import { Bell, Search, Settings } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import type { DashboardNotification } from "@/lib/types";

type TopbarProps = {
  title: string;
  notifications?: DashboardNotification[];
};

function formatRelativeTime(createdAt: number): string {
  const deltaSeconds = Math.max(0, Math.floor((Date.now() - createdAt) / 1000));
  if (deltaSeconds < 60) {
    return `${deltaSeconds}s`;
  }
  if (deltaSeconds < 3600) {
    return `${Math.floor(deltaSeconds / 60)}m`;
  }
  return `${Math.floor(deltaSeconds / 3600)}h`;
}

const toneLabel: Record<DashboardNotification["level"], string> = {
  info: "info",
  warn: "warning",
  error: "error",
  success: "completed",
};

export default function Topbar({ title, notifications = [] }: TopbarProps) {
  const [open, setOpen] = useState(false);

  const unreadCount = notifications.length;
  const recent = useMemo(() => notifications.slice(0, 12), [notifications]);

  return (
    <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-white/8 bg-[var(--bg-surface)]/90 px-4 backdrop-blur md:px-6">
      <div className="flex items-center gap-3">
        <span className="h-2 w-2 rounded-full bg-[var(--acc)] shadow-[0_0_10px_rgba(0,212,180,0.8)]" />
        <h1 className="text-3xl font-semibold tracking-tight text-zinc-100">{title}</h1>
      </div>

      <div className="flex items-center gap-1.5">
        <button
          type="button"
          className="grid h-8 w-8 place-items-center rounded-full border border-transparent text-zinc-400 transition hover:border-[var(--bd-accent)] hover:bg-[var(--acc-dim)] hover:text-cyan-100"
          aria-label="Search"
        >
          <Search className="h-[15px] w-[15px]" />
        </button>

        <div className="relative">
          <button
            type="button"
            onClick={() => setOpen((prev) => !prev)}
            className="relative grid h-8 w-8 place-items-center rounded-full border border-transparent text-zinc-400 transition hover:border-[var(--bd-accent)] hover:bg-[var(--acc-dim)] hover:text-cyan-100"
            aria-label="Notifications"
          >
            <Bell className="h-[15px] w-[15px]" />
            <span className="absolute -right-0.5 -top-0.5 grid h-4 min-w-4 place-items-center rounded-full bg-cyan-500 px-1 text-[10px] font-semibold text-black">
              {Math.min(99, unreadCount)}
            </span>
          </button>

          {open && (
            <div className="absolute right-0 mt-2 w-80 overflow-hidden rounded-xl border border-white/10 bg-[var(--bg-overlay)] shadow-2xl shadow-black/60 reveal-up">
              <div className="border-b border-white/8 px-4 py-3 text-sm font-semibold text-cyan-100">
                Recent Events
              </div>
              <ul className="max-h-72 divide-y divide-white/6 overflow-y-auto">
                {recent.length === 0 && (
                  <li className="px-4 py-4 text-sm text-zinc-400">No events yet.</li>
                )}
                {recent.map((note) => (
                  <li key={note.id} className="px-4 py-3">
                    <p className="text-sm text-zinc-200">{note.message}</p>
                    <p className="mt-1 text-xs text-zinc-500">
                      {toneLabel[note.level]} • {formatRelativeTime(note.createdAt)}
                    </p>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>

        <Link
          href="/settings"
          className="grid h-8 w-8 place-items-center rounded-full border border-transparent text-zinc-400 transition hover:border-[var(--bd-accent)] hover:bg-[var(--acc-dim)] hover:text-cyan-100"
          aria-label="Settings"
        >
          <Settings className="h-[15px] w-[15px]" />
        </Link>

        <button
          type="button"
          className="ml-1 grid h-8 w-8 place-items-center rounded-full border border-[var(--bd-accent)] bg-[var(--acc-dim)] text-xs font-semibold text-cyan-100"
          aria-label="User profile"
        >
          U
        </button>
      </div>
    </header>
  );
}
