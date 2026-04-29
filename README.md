# rawth

A key-value database built from scratch in Go. Custom B+Tree, custom binary file format, custom query language, TCP server, HTTP API, WebSocket server, and a web UI. No ORM. No Postgres. No SQLite. No `gorilla/websocket`. Zero external dependencies.

Not because that was the wise choice. Because that was the point.

---

## What It Does

rawth stores key-value pairs on disk in a B+Tree organized into 4KB pages. You can read, write, delete, and query them over TCP, HTTP, or a browser terminal. Every database file starts with the magic bytes `RAWT`. If you open one in a hex editor, that's the first thing you'll see.

The query language is called RQL. The commands are named `SHOVE`, `YOINK`, `YEET`, `PEEK`, `KEYS`, `STATS`, `NUKE`, and `HELP`. This was a deliberate choice. `SET/GET/DEL` is Redis. This is not Redis.

| Layer | What It Does |
|-------|-------------|
| **Pager** | 4KB fixed-size pages, buffer pool, free list, magic bytes `RAWT` |
| **B+Tree** | Disk-based tree with splitting, leaf linking, and range scans |
| **Engine** | Thread-safe wrapper with TTL support and lazy expiration |
| **RQL** | Hand-written lexer + recursive descent parser |
| **TCP Server** | Redis-style interface on port 6379 |
| **HTTP Server** | REST API + hand-rolled WebSocket (RFC 6455) |
| **Web UI** | Interactive terminal, live stats, architecture visualization |

---

## Building and Running

Requires Go 1.22+.

```bash
git clone https://github.com/niksingh2745/rawth.git
cd rawth
go build -o rawth ./cmd/rawth/
./rawth serve
```

Single binary. Web UI is embedded. Open `http://localhost:8080`.

---

## RQL

| Command | What It Does |
|---------|-------------|
| `SHOVE key "value"` | Store a key-value pair |
| `SHOVE key "value" TTL 60` | Store with expiration (seconds) |
| `YOINK key` | Retrieve a value |
| `YEET key` | Delete a key |
| `PEEK key` | Check existence |
| `KEYS` | List all keys |
| `STATS` | Database statistics |
| `NUKE` | Delete everything |
| `HELP` | Show command reference |

---

## TCP Interface

```bash
nc localhost 6379
```

Line-delimited RQL in, text out. Works exactly like you'd expect. Type `QUIT` to disconnect.

---

## Docker

```bash
docker build -t rawth .
docker run -p 8080:8080 -p 6379:6379 rawth
```

---

## What It Is Not

Production-ready. There's no write-ahead log, no crash recovery, no replication, no authentication. An unclean shutdown mid-write can corrupt the file. TTL expiration is lazy - expired keys stay on disk until you touch them. Node merging on delete is not implemented. The buffer pool eviction policy is "if the map is full, skip caching," which is exactly as naive as it sounds.

None of this is accidental. rawth exists to show how databases work, not to replace the one your company depends on.

---

## Project Structure

```
rawth/
├── cmd/rawth/main.go
├── internal/
│   ├── storage/
│   │   ├── pager.go       # disk I/O, buffer pool, free list
│   │   ├── btree.go       # B+Tree with split/search/scan
│   │   └── engine.go      # storage engine, TTL, stats
│   ├── rql/
│   │   ├── token.go
│   │   ├── lexer.go
│   │   ├── parser.go
│   │   └── executor.go
│   └── server/
│       ├── tcp.go
│       └── http.go        # REST API + hand-rolled WebSocket
├── web/
│   ├── embed.go
│   └── static/
│       ├── index.html
│       ├── style.css
│       └── app.js
├── go.mod
└── Dockerfile
```

---

## License

MIT - do whatever you want with it.

Built from scratch by Nikhil Singh.
