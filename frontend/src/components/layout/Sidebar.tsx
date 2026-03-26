"use client";

import type { LucideIcon } from "lucide-react";
import { ChevronLeft, ChevronRight } from "lucide-react";

type NavItem = {
  id: string;
  label: string;
  icon: LucideIcon;
  source: "WS" | "GO" | "UI" | "FS" | "RT";
};

type SidebarProps = {
  collapsed: boolean;
  activeTab: string;
  items: NavItem[];
  onCollapseToggle: () => void;
  onSelect: (id: string) => void;
};

export default function Sidebar({
  collapsed,
  activeTab,
  items,
  onCollapseToggle,
  onSelect,
}: SidebarProps) {
  return (
    <aside
      className={[
        "relative flex h-screen shrink-0 flex-col border-r border-cyan-900/30 bg-[#03080e]/95",
        "transition-all duration-300 ease-out",
        collapsed ? "w-12" : "w-56",
      ].join(" ")}
    >
      <div className="flex h-16 items-center gap-3 border-b border-cyan-950/50 px-3.5">
        <div className="grid h-7 w-7 place-items-center rounded-md bg-cyan-500/20 text-cyan-300">
          <span className="text-xs font-bold">T</span>
        </div>
        {!collapsed && (
          <div>
            <p className="text-xl font-semibold tracking-tight text-cyan-300">Torrent</p>
          </div>
        )}
      </div>

      <nav className="flex-1 space-y-1 px-2 py-3.5">
        {items.map((item) => {
          const selected = item.id === activeTab;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => onSelect(item.id)}
              className={[
                "group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm",
                "transition-colors duration-200",
                selected
                  ? "bg-cyan-500/20 text-cyan-200 ring-1 ring-cyan-400/40"
                  : "text-zinc-400 hover:bg-cyan-950/45 hover:text-cyan-100",
              ].join(" ")}
            >
              {selected && <span className="absolute inset-y-1 left-0 w-0.5 rounded-full bg-cyan-300" />}
              <item.icon className="h-[15px] w-[15px] shrink-0" />
              {!collapsed && (
                <>
                  <span className="flex-1 text-[13px]">{item.label}</span>
                  <span className="rounded bg-zinc-900 px-1.5 py-0.5 text-[10px] font-semibold text-cyan-400/90">
                    {item.source}
                  </span>
                </>
              )}
            </button>
          );
        })}
      </nav>

      <div className="border-t border-cyan-950/50 p-1.5">
        <button
          type="button"
          onClick={onCollapseToggle}
          className="flex w-full items-center justify-center gap-2 rounded-lg px-3 py-2 text-xs text-zinc-400 transition hover:bg-cyan-950/50 hover:text-cyan-100"
        >
          {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
          {!collapsed && <span>Collapse</span>}
        </button>
      </div>
    </aside>
  );
}
