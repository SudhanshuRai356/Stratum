"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
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

function normalizeSourceInput(value: string): string {
  let next = value.trim();
  next = next.replace(/^["'`]+|["'`]+$/g, "");
  if (next.startsWith("<") && next.endsWith(">")) {
    next = next.slice(1, -1).trim();
  }
  return next;
}

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
  const router = useRouter();
  const [startInput, setStartInput] = useState("");
  const [starting, setStarting] = useState(false);
  const [result, setResult] = useState("");
  const [hasError, setHasError] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const submitSource = async () => {
    const source = normalizeSourceInput(startInput);
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
      router.push("/dashboard");
    } catch (error) {
      setHasError(true);
      setResult(`Failed to start: ${String(error)}`);
    } finally {
      setStarting(false);
    }
  };

  return (
    <section className="panel reveal-up rounded-2xl p-4">
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <input
          type="text"
          placeholder="Paste magnet link or .torrent path/url, press Enter"
          value={startInput}
          onChange={(event) => setStartInput(event.target.value)}
          onPaste={(event) => {
            const pasted = event.clipboardData.getData("text");
            if (!pasted) {
              return;
            }
            event.preventDefault();
            setStartInput(normalizeSourceInput(pasted));
          }}
          onKeyDown={async (event) => {
            if (event.key !== "Enter" || event.shiftKey) {
              return;
            }
            event.preventDefault();
            await submitSource();
          }}
          className="min-w-[18rem] flex-1 rounded-xl border border-white/12 bg-black/20 px-4 py-2 text-sm text-zinc-100 outline-none transition focus:border-(--bd-accent)"
        />
        <button
          type="button"
          onClick={submitSource}
          disabled={starting}
          className="stratum-button rounded-xl px-3 py-2 text-sm font-semibold tracking-wide"
        >
          Start
        </button>
        <button
          type="button"
          onClick={() => fileInputRef.current?.click()}
          disabled={starting}
          className="stratum-button-secondary rounded-xl px-3 py-2 text-sm font-semibold tracking-wide disabled:opacity-50"
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
              router.push("/dashboard");
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
