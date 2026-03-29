"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";
import Link from "next/link";
import type { NavItem } from "@/lib/navigation";

type SidebarProps = {
  collapsed: boolean;
  pathname: string;
  items: NavItem[];
  onCollapseToggle: () => void;
};

export default function Sidebar({ collapsed, pathname, items, onCollapseToggle }: SidebarProps) {
  return (
    <aside
      className={[
        "relative flex h-screen shrink-0 flex-col border-r border-white/10 bg-[var(--bg-surface)]/95",
        "transition-all duration-300 ease-out",
        collapsed ? "w-12" : "w-56",
      ].join(" ")}
    >
      <div className="flex h-16 items-center gap-3 border-b border-white/8 px-3.5">
        <div className="grid h-7 w-7 place-items-center rounded-md bg-[var(--acc-dim)] text-[var(--acc)]">
          <span className="text-xs font-bold">T</span>
        </div>
        {!collapsed && (
          <div>
            <p className="text-xl font-semibold tracking-tight text-cyan-100">Stratum</p>
          </div>
        )}
      </div>

      <nav className="flex-1 space-y-1 px-2 py-3.5">
        {items.map((item) => {
          const selected = pathname === item.href || pathname.startsWith(`${item.href}/`);
          return (
            <Link
              key={item.id}
              href={item.href}
              className={[
                "group relative flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left",
                "transition-all duration-200",
                selected
                  ? "bg-[var(--acc-dim)] text-cyan-100 ring-1 ring-[var(--bd-accent)]"
                  : "text-zinc-400 hover:bg-[var(--bg-overlay)] hover:text-cyan-100",
              ].join(" ")}
            >
              <span
                className={[
                  "absolute left-0 top-1/2 h-[72%] w-0.5 -translate-y-1/2 rounded-full transition-opacity",
                  selected ? "bg-[var(--acc)] opacity-100" : "opacity-0",
                ].join(" ")}
              />
              <item.icon className="h-[15px] w-[15px] shrink-0" />
              {!collapsed && (
                <span className="flex-1 text-[15px] font-medium">{item.label}</span>
              )}
            </Link>
          );
        })}
      </nav>

      <div className="border-t border-white/8 p-1.5">
        <button
          type="button"
          onClick={onCollapseToggle}
          className="flex w-full items-center justify-center gap-2 rounded-lg px-3 py-2 text-xs text-zinc-400 transition hover:bg-[var(--bg-overlay)] hover:text-cyan-100"
        >
          {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
          {!collapsed && <span>Collapse</span>}
        </button>
      </div>
    </aside>
  );
}
