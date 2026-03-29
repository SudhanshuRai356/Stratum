type StatCardProps = {
  title: string;
  value: string;
  subtitle: string;
};
export default function StatCard({ title, value, subtitle }: StatCardProps) {
  return (
    <article className="panel reveal-up rounded-xl p-4">
      <p className="text-[11px] uppercase tracking-[0.11em] text-zinc-400">{title}</p>
      <p
        suppressHydrationWarning
        className="mt-3 text-[2rem] font-semibold leading-none text-zinc-100 [text-shadow:0_0_24px_rgba(0,212,180,0.2)]"
      >
        {value}
      </p>
      <p className="mt-3 text-xs text-zinc-400">{subtitle}</p>
    </article>
  );
}
