import type { DataSourceTag } from "@/lib/navigation";

type SourceBadgeProps = {
  source: DataSourceTag;
  critical?: boolean;
};

function sourceClasses(source: DataSourceTag): string {
  if (source === "RT") {
    return "bg-rose-950/60 text-rose-300 border-rose-500/30";
  }
  if (source === "GO") {
    return "bg-sky-950/60 text-sky-300 border-sky-500/30";
  }
  if (source === "FS") {
    return "bg-amber-950/60 text-amber-300 border-amber-500/30";
  }
  if (source === "UI") {
    return "bg-zinc-900 text-zinc-300 border-zinc-500/30";
  }
  return "bg-emerald-950/60 text-emerald-300 border-emerald-500/30";
}

export default function SourceBadge({ source, critical = false }: SourceBadgeProps) {
  return (
    <span
      className={[
        "inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold tracking-[0.08em]",
        sourceClasses(source),
      ].join(" ")}
    >
      {source}
      {critical ? " RT" : ""}
    </span>
  );
}
