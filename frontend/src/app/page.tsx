import AppShell from "@/components/layout/AppShell";

export default function Home() {
  return (
    <AppShell>
      <section className="space-y-6">
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <div className="h-28 rounded-2xl border border-cyan-900/40 bg-[#04101a]/70 p-4" />
          <div className="h-28 rounded-2xl border border-cyan-900/40 bg-[#04101a]/70 p-4" />
          <div className="h-28 rounded-2xl border border-cyan-900/40 bg-[#04101a]/70 p-4" />
          <div className="h-28 rounded-2xl border border-cyan-900/40 bg-[#04101a]/70 p-4" />
        </div>

        <div className="grid gap-4 xl:grid-cols-2">
          <div className="h-80 rounded-2xl border border-cyan-900/40 bg-[#04101a]/60 p-4" />
          <div className="h-80 rounded-2xl border border-cyan-900/40 bg-[#04101a]/60 p-4" />
        </div>

        <div className="h-80 rounded-2xl border border-cyan-900/40 bg-[#04101a]/60 p-4" />
      </section>
    </AppShell>
  );
}
