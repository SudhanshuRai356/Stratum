import type { DataSourceTag } from "@/lib/navigation";
import SourceBadge from "@/components/ui/SourceBadge";

type StatCardProps = {
  title: string;
  value: string;
  subtitle: string;
  trend: string;
  trendTone: "cyan" | "green" | "red";
  source?: DataSourceTag;
  critical?: boolean;
};

const trendToneClass: Record<StatCardProps["trendTone"], string> = {
  cyan: "text-cyan-300",
  green: "text-emerald-300",
  red: "text-rose-300",
};

export default function StatCard({
  title,
  value,
  subtitle,
  trend,
  trendTone,
  source = "WS",
  critical = false,
}: StatCardProps) {
  return (
    <article className="panel reveal-up rounded-xl p-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-[11px] uppercase tracking-[0.11em] text-zinc-400">{title}</p>
        <SourceBadge source={source} critical={critical} />
      </div>
      <p
        suppressHydrationWarning
        className="mt-3 text-[2rem] font-semibold leading-none text-zinc-100 [text-shadow:0_0_24px_rgba(0,212,180,0.2)]"
      >
        {value}
      </p>
      <div className="mt-3 flex items-center justify-between gap-3">
        <p className="text-xs text-zinc-400">{subtitle}</p>
        <p className={`text-xs font-semibold ${trendToneClass[trendTone]}`}>{trend}</p>
      </div>
    </article>
  );
}
