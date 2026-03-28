"use client";

import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
  type ChartOptions,
} from "chart.js";
import { Line } from "react-chartjs-2";
import SourceBadge from "@/components/ui/SourceBadge";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler);

type SpeedPoint = {
  ts: number;
  downloadBps: number;
  uploadBps: number;
};

type NetworkChartProps = {
  data: SpeedPoint[];
};

function toDisplayUnit(bytesPerSecond: number, divisor: number): number {
  return bytesPerSecond / divisor;
}

export default function NetworkChart({ data }: NetworkChartProps) {
  const labels = data.map((_, idx) => {
    const secondsAgo = data.length - 1 - idx;
    return secondsAgo === 0 ? "now" : `${secondsAgo}s ago`;
  });
  const peakBps = data.reduce((max, point) => Math.max(max, point.downloadBps, point.uploadBps), 0);
  const useMbps = peakBps >= 1024 * 1024;
  const divisor = useMbps ? 1024 * 1024 : 1024;
  const unitLabel = useMbps ? "MB/s" : "KB/s";

  const chartData = {
    labels,
    datasets: [
      {
        label: "Download Speed",
        data: data.map((point) => Number(toDisplayUnit(point.downloadBps, divisor).toFixed(2))),
        borderColor: "rgba(0, 212, 180, 1)",
        backgroundColor: (context: { chart: ChartJS<"line"> }) => {
          const { chart } = context;
          const { ctx, chartArea } = chart;
          if (!chartArea) {
            return "rgba(0, 212, 180, 0.28)";
          }
          const gradient = ctx.createLinearGradient(0, chartArea.top, 0, chartArea.bottom);
          gradient.addColorStop(0, "rgba(0, 212, 180, 0.55)");
          gradient.addColorStop(1, "rgba(0, 212, 180, 0.03)");
          return gradient;
        },
        pointRadius: 0,
        borderWidth: 2.4,
        tension: 0.4,
        fill: true,
      },
      {
        label: "Upload Speed",
        data: data.map((point) => Number(toDisplayUnit(point.uploadBps, divisor).toFixed(2))),
        borderColor: "rgba(186, 230, 253, 0.95)",
        backgroundColor: (context: { chart: ChartJS<"line"> }) => {
          const { chart } = context;
          const { ctx, chartArea } = chart;
          if (!chartArea) {
            return "rgba(186, 230, 253, 0.14)";
          }
          const gradient = ctx.createLinearGradient(0, chartArea.top, 0, chartArea.bottom);
          gradient.addColorStop(0, "rgba(186, 230, 253, 0.26)");
          gradient.addColorStop(1, "rgba(186, 230, 253, 0.02)");
          return gradient;
        },
        pointRadius: 0,
        borderWidth: 2,
        borderDash: [6, 5],
        tension: 0.34,
        fill: true,
      },
    ],
  };

  const options: ChartOptions<"line"> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: {
      duration: 820,
      easing: "easeOutQuart",
      delay: (ctx) => {
        const stage = ctx as { type?: string; datasetIndex?: number; dataIndex?: number };
        if (stage.type !== "data") {
          return 0;
        }
        return (stage.datasetIndex ?? 0) * 140 + (stage.dataIndex ?? 0) * 8;
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
      y: {
        title: {
          display: true,
          text: unitLabel,
          color: "rgba(141, 160, 166, 0.85)",
        },
        suggestedMin: 0,
        suggestedMax: peakBps > 0 ? Number((toDisplayUnit(peakBps, divisor) * 1.2).toFixed(2)) : 1,
        grid: {
          color: "rgba(255, 255, 255, 0.08)",
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
        <h3 className="text-[1.65rem] font-semibold text-zinc-100">Network Activity ({unitLabel})</h3>
        <div className="flex gap-1.5">
          <SourceBadge source="WS" />
          <SourceBadge source="RT" critical />
        </div>
      </div>
      <div className="chart-stage h-[85%]">
        <Line data={chartData} options={options} />
      </div>
    </article>
  );
}
