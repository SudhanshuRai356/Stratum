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

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler);

type SpeedPoint = {
  ts: number;
  downloadBps: number;
  uploadBps: number;
};

type NetworkChartProps = {
  data: SpeedPoint[];
};

function toMbPerSec(value: number): number {
  return value / (1024 * 1024);
}

export default function NetworkChart({ data }: NetworkChartProps) {
  const labels = data.map((_, idx) => `${idx - data.length + 1}s`);

  const chartData = {
    labels,
    datasets: [
      {
        label: "Download Speed",
        data: data.map((point) => Number(toMbPerSec(point.downloadBps).toFixed(2))),
        borderColor: "rgba(34, 211, 238, 1)",
        backgroundColor: "rgba(34, 211, 238, 0.22)",
        pointRadius: 0,
        borderWidth: 2,
        tension: 0.32,
        fill: true,
      },
      {
        label: "Upload Speed",
        data: data.map((point) => Number(toMbPerSec(point.uploadBps).toFixed(2))),
        borderColor: "rgba(20, 65, 90, 1)",
        backgroundColor: "rgba(20, 65, 90, 0.08)",
        pointRadius: 0,
        borderWidth: 2,
        tension: 0.32,
      },
    ],
  };

  const options: ChartOptions<"line"> = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      mode: "index",
      intersect: false,
    },
    plugins: {
      legend: {
        labels: {
          color: "rgba(161, 161, 170, 0.92)",
          boxWidth: 14,
        },
      },
    },
    scales: {
      x: {
        grid: {
          color: "rgba(39, 39, 42, 0.32)",
        },
        ticks: {
          color: "rgba(113, 113, 122, 0.88)",
          maxTicksLimit: 7,
        },
      },
      y: {
        title: {
          display: true,
          text: "MB/s",
          color: "rgba(113, 113, 122, 0.88)",
        },
        grid: {
          color: "rgba(39, 39, 42, 0.32)",
        },
        ticks: {
          color: "rgba(113, 113, 122, 0.88)",
        },
      },
    },
  };

  return (
    <article className="panel reveal-up h-[21.5rem] rounded-xl p-4">
      <h3 className="mb-2 text-[1.65rem] font-semibold text-zinc-100">Network Activity (MB/s)</h3>
      <div className="h-[85%]">
        <Line data={chartData} options={options} />
      </div>
    </article>
  );
}
