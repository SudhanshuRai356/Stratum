# Stratum

A BitTorrent client. Written in Go. It works.

---

## What Is It

Stratum is a fully concurrent BitTorrent client built from scratch in Go. No libtorrent. No shortcuts. Every wire protocol message, every piece hash verification, every byte offset — all done by hand. It supports both `.torrent` files and magnet links, handles multi-file torrents, finds peers on its own when trackers are useless, and will pick up where it left off if it crashes. Which it might. But less than it used to.

---

## The Flex

Here is what it actually does, since apparently that needs to be spelled out:

- **Full-duplex downloading and seeding** — it does both at the same time, concurrently, because that is how the protocol is supposed to work and most clients get lazy about it.

- **Multi-file torrent support** — maps global piece offsets to the correct files in nested directory structures. This sounds trivial until you are three hours deep into byte-boundary bugs that only appear at file edges. It is not trivial.

- **Kademlia DHT** — when the tracker is dead, rate-limiting you, or just lying, the client falls back to DHT to find peers on its own. It maintains a routing table, issues find_node and get_peers queries, and generally does what it needs to do without hand-holding.

- **Persistent state resumption** — progress is written to disk. If the process dies, the next run reads the state back and continues. You do not lose everything because of a crash or a stray Ctrl-C.


---

## Usage

```
go run ./cmd/stratum <your .torrent file or magnet link>
```

## Local Dev (Backend + Frontend)

Run both services together:

```
./scripts/dev.sh
```

This starts:

- Go backend API on `127.0.0.1:8080`
- Next.js frontend in `frontend/`

Press `Ctrl+C` once to stop both.

Both formats are supported. Hand it a `.torrent` file or a `magnet:` URI (wrapped in quotes :3) and it will figure it out. For magnet links, it fetches the metadata from peers before starting the download, which takes a moment. The output directory structure is built automatically based on the torrent metadata, so you do not need to create anything beforehand.

---

## Monitoring

Open a second terminal and run this:

```
watch -n 1 du -h <output folder>
```

That will show you the storage engine assembling the files from pieces in real time. It is the closest thing to a progress bar you are getting right now. The numbers will climb as verified pieces get written to disk. If they stop climbing for a long time, the peers are probably garbage.