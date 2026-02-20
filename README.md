# clr-gmessages-bridge

Google Messages (SMS/RCS) bridge with Supabase sync. Fork of [MaxGhenis/openmessage](https://github.com/MaxGhenis/openmessage) with added Supabase integration for centralized message storage.

Built on [mautrix/gmessages](https://github.com/mautrix/gmessages) (libgm) for the Google Messages protocol.

## What we added (vs upstream openmessage)

- **Supabase sync** — Messages, conversations, and contacts sync to Supabase via PostgREST RPC (alongside existing SQLite)
- **Supabase Storage** — Media uploads to `gmessages-media` bucket with public URLs
- **Auto-migration** — Schema applied on startup via `SUPABASE_DB_URL` (optional)
- **`/api/download` endpoint** — Downloads media from Google Messages, uploads to Supabase Storage, returns public URL
- **Dockerfile** — Multi-stage build for `ghcr.io/clearminds/clr-gmessages-bridge`
- **CI/CD** — GitHub Actions for tests + Docker builds

## Architecture

```
Phone (Google Messages app)
    ↕ QR code pairing (libgm)
clr-gmessages-bridge (this repo)
    ↕ PostgREST RPC + Storage API
Supabase (PostgreSQL + Storage)
    ↕ supabase-py (read-only)
clr-gmessages-mcp (Python, FastMCP)
    ↕ MCP tools
Claude
```

## Quick start

### Prerequisites

- **Go 1.24+** ([install](https://go.dev/dl/))
- **Google Messages** on your Android phone
- **Supabase project** (optional — works without it, just uses local SQLite)

### 1. Build

```bash
go build -o gmessages-bridge .
```

### 2. Pair with your phone

```bash
./gmessages-bridge pair
```

Scan the QR code in Google Messages > Settings > Device pairing.

### 3. Start the server

```bash
# Without Supabase (local SQLite only)
./gmessages-bridge serve

# With Supabase sync
export SUPABASE_URL="https://your-project.supabase.co"
export SUPABASE_KEY="your-service-role-key"
export SUPABASE_DB_URL="postgresql://..."  # optional, for auto-migration
./gmessages-bridge serve
```

### 4. Docker

```bash
docker run -d \
  -p 7007:7007 \
  -v gmessages-data:/data \
  -e SUPABASE_URL="..." \
  -e SUPABASE_KEY="..." \
  ghcr.io/clearminds/clr-gmessages-bridge:latest
```

## Environment variables

| Var | Default | Purpose |
|-----|---------|---------|
| `SUPABASE_URL` | *(none)* | Supabase project URL (enables sync) |
| `SUPABASE_KEY` | *(none)* | Supabase service role key |
| `SUPABASE_DB_URL` | *(none)* | PostgreSQL URL for auto-migration |
| `OPENMESSAGES_DATA_DIR` | `~/.local/share/openmessage` | Data directory (DB + session) |
| `OPENMESSAGES_PORT` | `7007` | Web UI / API port |
| `OPENMESSAGES_LOG_LEVEL` | `info` | Log level (debug/info/warn/error) |

## REST API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/conversations` | GET | List conversations |
| `/api/conversations/{id}/messages` | GET | Messages in a conversation |
| `/api/search?q=...` | GET | Full-text search |
| `/api/send` | POST | Send a message |
| `/api/download` | POST | Download media → Supabase Storage |
| `/api/status` | GET | Connection status |
| `/api/media/{msg_id}` | GET | Stream media from Google Messages |

## Development

```bash
go test ./...        # Run all tests (7 suites)
go build .           # Build binary
go vet ./...         # Lint
```

## License

AGPL-3.0 (required by libgm dependency)
