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

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, Tooltip, Legend);

type UtilPoint = {
  ts: number;
  peers: number;
  cpu: number;
  disk: number;
};

type UtilizationChartProps = {
  data: UtilPoint[];
};

export default function UtilizationChart({ data }: UtilizationChartProps) {
  const labels = data.map((_, idx) => `${idx - data.length + 1}s`);

  const chartData: ChartData<"bar" | "line", number[], string> = {
    labels,
    datasets: [
      {
        type: "bar" as const,
        label: "Active Peers",
        data: data.map((point) => point.peers),
        yAxisID: "yPeers",
        backgroundColor: "rgba(22, 78, 99, 0.45)",
        borderRadius: 8,
      },
      {
        type: "line" as const,
        label: "CPU Usage",
        data: data.map((point) => Number(point.cpu.toFixed(1))),
        yAxisID: "yPercent",
        borderColor: "rgba(45, 212, 191, 1)",
        pointRadius: 0,
        borderWidth: 2,
        tension: 0.3,
      },
      {
        type: "line" as const,
        label: "Disk I/O",
        data: data.map((point) => Number(point.disk.toFixed(1))),
        yAxisID: "yPercent",
        borderColor: "rgba(59, 130, 246, 1)",
        pointRadius: 0,
        borderWidth: 2,
        tension: 0.3,
      },
    ],
  };

  const options: ChartOptions<"bar" | "line"> = {
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
      yPeers: {
        position: "left",
        title: {
          display: true,
          text: "Peers",
          color: "rgba(113, 113, 122, 0.88)",
        },
        grid: {
          color: "rgba(39, 39, 42, 0.32)",
        },
        ticks: {
          color: "rgba(113, 113, 122, 0.88)",
        },
      },
      yPercent: {
        position: "right",
        min: 0,
        max: 100,
        title: {
          display: true,
          text: "%",
          color: "rgba(113, 113, 122, 0.88)",
        },
        grid: {
          drawOnChartArea: false,
        },
        ticks: {
          color: "rgba(113, 113, 122, 0.88)",
        },
      },
    },
  };

  return (
    <article className="panel reveal-up h-[21.5rem] rounded-xl p-4">
      <h3 className="mb-2 text-[1.65rem] font-semibold text-zinc-100">Peer &amp; System Utilization</h3>
      <div className="h-[85%]">
        <Chart type="bar" data={chartData} options={options} />
      </div>
    </article>
  );
}
