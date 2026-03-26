"use client";

import { Bell, Search, Settings } from "lucide-react";
import { useMemo, useState } from "react";

type TopbarProps = {
  title: string;
};

export default function Topbar({ title }: TopbarProps) {
  const [open, setOpen] = useState(false);

  const notifications = useMemo(
    () => [
      { id: "n1", type: "completed", message: "Torrent OS v3.2 finished", time: "2m" },
      { id: "n2", type: "warn", message: "Peer banned for corrupt piece", time: "6m" },
      { id: "n3", type: "error", message: "Tracker timeout on udp://...", time: "9m" },
    ],
    [],
  );

  return (
    <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-cyan-950/40 bg-[#02070d]/90 px-6 backdrop-blur">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight text-zinc-100">{title}</h1>
      </div>

      <div className="flex items-center gap-2">
        <button
          type="button"
          className="grid h-9 w-9 place-items-center rounded-full text-zinc-400 transition hover:bg-cyan-950/60 hover:text-cyan-200"
          aria-label="Search"
        >
          <Search className="h-4 w-4" />
        </button>

        <div className="relative">
          <button
            type="button"
            onClick={() => setOpen((prev) => !prev)}
            className="relative grid h-9 w-9 place-items-center rounded-full text-zinc-400 transition hover:bg-cyan-950/60 hover:text-cyan-200"
            aria-label="Notifications"
          >
            <Bell className="h-4 w-4" />
            <span className="absolute -right-0.5 -top-0.5 grid h-4 min-w-4 place-items-center rounded-full bg-cyan-500 px-1 text-[10px] font-semibold text-black">
              3
            </span>
          </button>

          {open && (
            <div className="absolute right-0 mt-2 w-80 overflow-hidden rounded-xl border border-cyan-900/50 bg-[#05111b] shadow-2xl shadow-cyan-950/60">
              <div className="border-b border-cyan-900/40 px-4 py-3 text-sm font-semibold text-cyan-200">
                Recent Events
              </div>
              <ul className="max-h-72 divide-y divide-cyan-950/50 overflow-y-auto">
                {notifications.map((note) => (
                  <li key={note.id} className="px-4 py-3">
                    <p className="text-sm text-zinc-200">{note.message}</p>
                    <p className="mt-1 text-xs text-zinc-500">
                      {note.type} • {note.time}
                    </p>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>

        <button
          type="button"
          className="grid h-9 w-9 place-items-center rounded-full text-zinc-400 transition hover:bg-cyan-950/60 hover:text-cyan-200"
          aria-label="Settings"
        >
          <Settings className="h-4 w-4" />
        </button>

        <button
          type="button"
          className="ml-1 grid h-9 w-9 place-items-center rounded-full border border-cyan-700/40 bg-cyan-500/20 text-sm font-semibold text-cyan-100"
          aria-label="User profile"
        >
          U
        </button>
      </div>
    </header>
  );
}
