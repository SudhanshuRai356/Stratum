import {
  Activity,
  ArrowDownToLine,
  ArrowUpToLine,
  CheckCircle2,
  FileCog,
  LayoutDashboard,
  Plug,
  Search,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";

export type DataSourceTag = "WS" | "GO" | "UI" | "FS" | "RT";

export type NavItem = {
  id: string;
  label: string;
  href: string;
  icon: LucideIcon;
  source: DataSourceTag;
};

export const NAV_ITEMS: NavItem[] = [
  { id: "dashboard", label: "Dashboard", href: "/dashboard", icon: LayoutDashboard, source: "WS" },
  { id: "search", label: "Search", href: "/search", icon: Search, source: "GO" },
  { id: "downloads", label: "Downloads", href: "/downloads", icon: ArrowDownToLine, source: "WS" },
  { id: "uploads", label: "Seeding", href: "/uploads", icon: ArrowUpToLine, source: "WS" },
  { id: "completed", label: "Completed", href: "/completed", icon: CheckCircle2, source: "FS" },
  { id: "security", label: "Security", href: "/security", icon: ShieldCheck, source: "RT" },
  { id: "logs", label: "Logs", href: "/logs", icon: Activity, source: "WS" },
  { id: "settings", label: "Settings", href: "/settings", icon: FileCog, source: "GO" },
  { id: "plugins", label: "Plugins", href: "/plugins", icon: Plug, source: "UI" },
];

export function titleForPathname(pathname: string): string {
  const exact = NAV_ITEMS.find((item) => item.href === pathname);
  if (exact) {
    return exact.label;
  }
  const fallback = NAV_ITEMS.find((item) => pathname.startsWith(item.href));
  return fallback?.label ?? "Dashboard";
}
