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
import Sidebar from "./Sidebar";
import Topbar from "./Topbar";

type AppShellProps = {
  children: React.ReactNode;
};

const SIDEBAR_STORAGE_KEY = "stratum.sidebar.collapsed";

export default function AppShell({ children }: AppShellProps) {
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
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
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_15%_15%,rgba(14,165,233,0.18),transparent_38%),radial-gradient(circle_at_80%_0%,rgba(6,182,212,0.08),transparent_40%),linear-gradient(180deg,#02050a_0%,#03070e_45%,#010409_100%)]" />

      <div className="relative flex min-h-screen">
        <Sidebar
          collapsed={collapsed}
          activeTab={activeTab}
          items={navItems}
          onCollapseToggle={toggleCollapsed}
          onSelect={setActiveTab}
        />

        <div className="flex min-h-screen flex-1 flex-col">
          <Topbar title="Overview" />
          <main className="flex-1 overflow-y-auto px-6 py-6 md:px-8">{children}</main>
        </div>
      </div>
    </div>
  );
}
