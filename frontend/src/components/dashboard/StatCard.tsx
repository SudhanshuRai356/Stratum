type StatCardProps = {
  title: string;
  value: string;
  subtitle: string;
  trend: string;
  trendTone: "cyan" | "green" | "red";
};

const trendToneClass: Record<StatCardProps["trendTone"], string> = {
  cyan: "text-cyan-300",
  green: "text-emerald-300",
  red: "text-rose-300",
};

export default function StatCard({ title, value, subtitle, trend, trendTone }: StatCardProps) {
  return (
    <article className="panel reveal-up h-28 rounded-xl p-4">
      <p className="text-[11px] uppercase tracking-[0.11em] text-zinc-500">{title}</p>
      <p className="mt-2 text-[2rem] font-semibold leading-none text-zinc-100 [text-shadow:0_0_24px_rgba(34,211,238,0.14)]">
        {value}
      </p>
      <div className="mt-2 flex items-center justify-between">
        <p className="text-xs text-zinc-400">{subtitle}</p>
        <p className={`text-xs font-semibold ${trendToneClass[trendTone]}`}>{trend}</p>
      </div>
    </article>
  );
}
