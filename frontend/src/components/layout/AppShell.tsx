"use client";

import { usePathname } from "next/navigation";
import { useState } from "react";
import { NAV_ITEMS, titleForPathname } from "@/lib/navigation";
import type { DashboardNotification } from "@/lib/types";
import Sidebar from "./Sidebar";
import Topbar from "./Topbar";

type AppShellProps = {
  children: React.ReactNode;
  title?: string;
  notifications?: DashboardNotification[];
};

const SIDEBAR_STORAGE_KEY = "stratum.sidebar.collapsed";

export default function AppShell({ children, title = "Overview", notifications = [] }: AppShellProps) {
  const pathname = usePathname();
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }
    if (window.innerWidth < 1024) {
      return true;
    }
    return localStorage.getItem(SIDEBAR_STORAGE_KEY) === "1";
  });
  const toggleCollapsed = () => {
    setCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem(SIDEBAR_STORAGE_KEY, next ? "1" : "0");
      return next;
    });
  };
  const resolvedTitle = title === "Overview" ? titleForPathname(pathname) : title;

  return (
    <div className="relative min-h-screen overflow-hidden bg-[var(--bg-base)] text-zinc-100">
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(180deg,var(--bg-void)_0%,var(--bg-base)_46%,#0a0d12_100%)]" />

      <div className="relative flex min-h-screen">
        <Sidebar collapsed={collapsed} pathname={pathname} items={NAV_ITEMS} onCollapseToggle={toggleCollapsed} />

        <div className="flex min-h-screen flex-1 flex-col">
          <Topbar title={resolvedTitle} notifications={notifications} />
          <main className="flex-1 overflow-y-auto px-4 py-4 md:px-7 md:py-6">{children}</main>
        </div>
      </div>
    </div>
  );
}
