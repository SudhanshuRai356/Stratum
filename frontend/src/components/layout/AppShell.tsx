"use client";

import {
  Activity,
  ArrowDownToLine,
  ArrowUpToLine,
  CheckCircle2,
  FileCog,
  LayoutDashboard,
  Plug,
  ShieldCheck,
} from "lucide-react";
import { useMemo, useState } from "react";
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
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }
    if (window.innerWidth < 1024) {
      return true;
    }
    return localStorage.getItem(SIDEBAR_STORAGE_KEY) === "1";
  });
  const [activeTab, setActiveTab] = useState("dashboard");

  const toggleCollapsed = () => {
    setCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem(SIDEBAR_STORAGE_KEY, next ? "1" : "0");
      return next;
    });
  };

  const navItems = useMemo(
    () => [
      { id: "dashboard", label: "Dashboard", icon: LayoutDashboard, source: "WS" as const },
      { id: "downloads", label: "Active Downloads", icon: ArrowDownToLine, source: "WS" as const },
      { id: "uploads", label: "Uploads", icon: ArrowUpToLine, source: "WS" as const },
      { id: "completed", label: "Completed", icon: CheckCircle2, source: "FS" as const },
      { id: "plugins", label: "Plugins", icon: Plug, source: "UI" as const },
      { id: "security", label: "Security Center", icon: ShieldCheck, source: "RT" as const },
      { id: "logs", label: "Logs", icon: Activity, source: "WS" as const },
      { id: "settings", label: "Settings", icon: FileCog, source: "GO" as const },
    ],
    [],
  );

  return (
    <div className="relative min-h-screen overflow-hidden bg-[#010409] text-zinc-100">
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(180deg,#010409_0%,#03070e_52%,#02060c_100%)]" />

      <div className="relative flex min-h-screen">
        <Sidebar
          collapsed={collapsed}
          activeTab={activeTab}
          items={navItems}
          onCollapseToggle={toggleCollapsed}
          onSelect={setActiveTab}
        />

        <div className="flex min-h-screen flex-1 flex-col">
          <Topbar title={title} notifications={notifications} />
          <main className="flex-1 overflow-y-auto px-4 py-4 md:px-7 md:py-6">{children}</main>
        </div>
      </div>
    </div>
  );
}
