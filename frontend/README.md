# Stratum Frontend

Next.js + Tauri dashboard for the Stratum torrent backend.

## Features Implemented

- Dashboard shell with collapsible sidebar and topbar actions.
- Live websocket wiring to `hub.go` TorrentState feed.
- WSRT overview cards (downloaded/uploaded/active torrents/peers).
- Rolling 60-second charts (network activity + utilization).
- Active torrent cards with speed, progress, ETA, and swarm metrics.
- Notification dropdown fed from websocket lifecycle and torrent state changes.
- Tauri desktop wrapper scaffold (`src-tauri`) with npm scripts.

## Environment

Create `.env.local` from `.env.example`:

```bash
cp .env.example .env.local
```

Set `NEXT_PUBLIC_TORRENT_API_TOKEN` to the token printed by backend startup.

## Run Web Dashboard

```bash
npm install
npm run dev
```

Open `http://localhost:3000`.

## Run Desktop App (Tauri)

Prerequisites:

- Rust toolchain (`rustup`, `cargo`)
- Tauri system dependencies for Linux (webkit2gtk, etc.)

Run:

```bash
npm run tauri:dev
```

Build desktop bundle:

```bash
npm run tauri:build
```

## Backend Notes

- Backend websocket endpoint defaults to `ws://127.0.0.1:8080/ws`.
- `server.go` currently allows `http://localhost:3000` origin; include your Tauri dev origin if needed.
