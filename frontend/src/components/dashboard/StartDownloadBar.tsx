"use client";

import { useRef, useState } from "react";
import { callGoBackend } from "@/lib/runtime";

type CommandResult = {
  status?: string;
  message?: string;
};

type StartPayload = {
  source?: string;
  torrent_base64?: string;
  filename?: string;
};

async function sendStartCommand(payload: StartPayload) {
  return callGoBackend<CommandResult>("/api/start", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

function isTorrentFilename(name: string): boolean {
  return name.toLowerCase().endsWith(".torrent");
}

async function fileToBase64(file: File): Promise<string> {
  const bytes = await file.arrayBuffer();
  let binary = "";
  const chunkSize = 0x8000;
  const view = new Uint8Array(bytes);
  for (let index = 0; index < view.length; index += chunkSize) {
    const chunk = view.subarray(index, index + chunkSize);
    binary += String.fromCharCode(...chunk);
  }
  return btoa(binary);
}

export default function StartDownloadBar() {
  const [startInput, setStartInput] = useState("");
  const [starting, setStarting] = useState(false);
  const [result, setResult] = useState("");
  const [hasError, setHasError] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  return (
    <section className="panel reveal-up rounded-2xl p-4">
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <input
          type="text"
          placeholder="Paste magnet link or .torrent path/url, press Enter"
          value={startInput}
          onChange={(event) => setStartInput(event.target.value)}
          onKeyDown={async (event) => {
            if (event.key !== "Enter" || event.shiftKey) {
              return;
            }
            event.preventDefault();

            const source = startInput.trim();
            if (!source || starting) {
              return;
            }

            setStarting(true);
            setResult("");
            setHasError(false);
            try {
              const response = await sendStartCommand({ source });
              setResult(response.message ?? "Download process started.");
              setStartInput("");
            } catch (error) {
              setHasError(true);
              setResult(`Failed to start: ${String(error)}`);
            } finally {
              setStarting(false);
            }
          }}
          className="min-w-[18rem] flex-1 rounded-xl border border-white/12 bg-black/20 px-4 py-2 text-sm text-zinc-100 outline-none transition focus:border-[var(--bd-accent)]"
        />
        <button
          type="button"
          onClick={() => fileInputRef.current?.click()}
          disabled={starting}
          className="rounded-xl border border-white/12 bg-black/25 px-3 py-2 text-sm text-zinc-200 transition hover:border-[var(--bd-accent)] hover:text-cyan-100 disabled:opacity-50"
        >
          Choose .torrent
        </button>
        <input
          ref={fileInputRef}
          type="file"
          accept=".torrent,application/x-bittorrent"
          className="hidden"
          onChange={async (event) => {
            const file = event.target.files?.[0];
            if (!file || starting) {
              return;
            }

            setStarting(true);
            setResult("");
            setHasError(false);

            try {
              if (!isTorrentFilename(file.name)) {
                throw new Error("Please choose a .torrent file");
              }
              const torrentBase64 = await fileToBase64(file);
              const response = await sendStartCommand({
                torrent_base64: torrentBase64,
                filename: file.name,
              });
              setResult(response.message ?? `Started ${file.name}`);
            } catch (error) {
              setHasError(true);
              setResult(`Failed to start torrent file: ${String(error)}`);
            } finally {
              event.target.value = "";
              setStarting(false);
            }
          }}
        />
      </div>

      <p className="text-xs text-zinc-400">Supports magnet links, local .torrent upload, and .torrent path/url input.</p>
      {result && <p className={`mt-2 text-xs ${hasError ? "text-rose-300" : "text-cyan-200"}`}>{result}</p>}
    </section>
  );
}
