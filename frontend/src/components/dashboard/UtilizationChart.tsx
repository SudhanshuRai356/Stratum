"use client";

import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  type ChartData,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
  type ChartOptions,
} from "chart.js";
import { Chart } from "react-chartjs-2";
import SourceBadge from "@/components/ui/SourceBadge";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, Tooltip, Legend);

type UtilPoint = {
  ts: number;
  peers: number;
  seeders: number;
  leechers: number;
  cpu: number;
  disk: number;
};

type UtilizationChartProps = {
  data: UtilPoint[];
};

export default function UtilizationChart({ data }: UtilizationChartProps) {
  const labels = data.map((_, idx) => {
    const secondsAgo = data.length - 1 - idx;
    return secondsAgo === 0 ? "now" : `${secondsAgo}s ago`;
  });
  const peakPeers = data.reduce((max, point) => Math.max(max, point.peers), 0);

  const chartData: ChartData<"bar" | "line", number[], string> = {
    labels,
    datasets: [
      {
        type: "bar" as const,
        label: "Active Peers",
        data: data.map((point) => point.peers),
        yAxisID: "yPeers",
        backgroundColor: (context: { chart: ChartJS<"bar" | "line"> }) => {
          const { chart } = context;
          const { ctx, chartArea } = chart;
          if (!chartArea) {
            return "rgba(0, 212, 180, 0.22)";
          }
          const gradient = ctx.createLinearGradient(0, chartArea.top, 0, chartArea.bottom);
          gradient.addColorStop(0, "rgba(0, 212, 180, 0.45)");
          gradient.addColorStop(1, "rgba(0, 212, 180, 0.08)");
          return gradient;
        },
        borderRadius: 8,
      },
      {
        type: "line" as const,
        label: "Seeders",
        data: data.map((point) => point.seeders),
        yAxisID: "yPeers",
        borderColor: "rgba(16, 185, 129, 1)",
        pointRadius: 0,
        borderWidth: 2.2,
        tension: 0.34,
        fill: false,
      },
      {
        type: "line" as const,
        label: "Leechers",
        data: data.map((point) => point.leechers),
        yAxisID: "yPeers",
        borderColor: "rgba(251, 191, 36, 0.95)",
        pointRadius: 0,
        borderWidth: 2,
        borderDash: [7, 5],
        tension: 0.3,
        fill: false,
      },
      {
        type: "line" as const,
        label: "CPU %",
        data: data.map((point) => Number(point.cpu.toFixed(1))),
        yAxisID: "yPercent",
        borderColor: "rgba(34, 197, 94, 1)",
        pointRadius: 0,
        borderWidth: 1.8,
        tension: 0.3,
        fill: false,
      },
      {
        type: "line" as const,
        label: "Disk %",
        data: data.map((point) => Number(point.disk.toFixed(1))),
        yAxisID: "yPercent",
        borderColor: "rgba(244, 114, 182, 1)",
        pointRadius: 0,
        borderWidth: 1.8,
        borderDash: [5, 4],
        tension: 0.3,
        fill: false,
      },
    ],
  };

  const options: ChartOptions<"bar" | "line"> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: {
      duration: 880,
      easing: "easeOutQuart",
      delay: (ctx) => {
        const stage = ctx as { type?: string; datasetIndex?: number; dataIndex?: number };
        if (stage.type !== "data") {
          return 0;
        }
        return (stage.datasetIndex ?? 0) * 110 + (stage.dataIndex ?? 0) * 5;
      },
    },
    interaction: {
      mode: "index",
      intersect: false,
    },
    plugins: {
      legend: {
        labels: {
          color: "rgba(203, 213, 225, 0.9)",
          boxWidth: 14,
        },
      },
    },
    scales: {
      x: {
        grid: {
          color: "rgba(255, 255, 255, 0.08)",
        },
        ticks: {
          color: "rgba(141, 160, 166, 0.85)",
          maxTicksLimit: 6,
          callback: (_, index) => {
            const step = Math.max(1, Math.floor(data.length / 6));
            return index % step === 0 || index === data.length - 1 ? labels[index] : "";
          },
        },
      },
      yPeers: {
        position: "left",
        min: 0,
        suggestedMax: peakPeers > 0 ? Math.ceil(peakPeers * 1.25) : 5,
        title: {
          display: true,
          text: "Peers / Swarm",
          color: "rgba(141, 160, 166, 0.85)",
        },
        grid: {
          color: "rgba(255, 255, 255, 0.08)",
        },
        ticks: {
          color: "rgba(141, 160, 166, 0.85)",
        },
      },
      yPercent: {
        position: "right",
        min: 0,
        max: 100,
        title: {
          display: true,
          text: "%",
          color: "rgba(141, 160, 166, 0.85)",
        },
        grid: {
          drawOnChartArea: false,
        },
        ticks: {
          color: "rgba(141, 160, 166, 0.85)",
        },
      },
    },
  };

  return (
    <article className="panel chart-panel-glow scan-lines reveal-up h-[21.5rem] rounded-xl p-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="text-[1.65rem] font-semibold text-zinc-100">Swarm Pattern &amp; Utilization</h3>
        <div className="flex gap-1.5">
          <SourceBadge source="WS" />
          <SourceBadge source="GO" />
        </div>
      </div>
      <div className="chart-stage chart-stage-delayed h-[85%]">
        <Chart type="bar" data={chartData} options={options} />
      </div>
    </article>
  );
}
